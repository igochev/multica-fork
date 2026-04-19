package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type projectControlOverseerReconcileResponse struct {
	ProjectID            string                           `json:"project_id"`
	Autonomy             *projectOverseerAutonomyResponse `json:"autonomy"`
	CreatedAutopilot     bool                             `json:"created_autopilot"`
	UpdatedAutopilot     bool                             `json:"updated_autopilot"`
	CreatedTrigger       bool                             `json:"created_trigger"`
	UpdatedTrigger       bool                             `json:"updated_trigger"`
	UpdatedProjectLink   bool                             `json:"updated_project_link"`
}

type projectControlOverseerReconcileHandler interface {
	ReconcileProjectControlOverseer(http.ResponseWriter, *http.Request)
}

func callReconcileProjectControlOverseer(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(projectControlOverseerReconcileHandler)
	if !ok {
		t.Fatalf("handler is missing method ReconcileProjectControlOverseer")
	}

	h.ReconcileProjectControlOverseer(w, req)
}

func TestProjectControlOverseerReconcileBackfillsAutopilotAndIsIdempotent(t *testing.T) {
	project := createPipelineTestProject(t, "Project control overseer reconcile")
	overseerAgentID := createHandlerTestAgent(t, "Project control reconcile overseer agent", nil)

	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO project_control_settings (project_id, overseer_agent_id, overseer_config)
		VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (project_id) DO UPDATE
		SET overseer_agent_id = EXCLUDED.overseer_agent_id,
		    overseer_config = EXCLUDED.overseer_config,
		    overseer_autopilot_id = NULL,
		    updated_at = now()
	`, project.ID, overseerAgentID, `{"scan_interval_hours":12,"scan_focus":["security"]}`); err != nil {
		t.Fatalf("insert overseer reconcile fixture: %v", err)
	}

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects/"+project.ID+"/control/overseer/reconcile", nil)
	req = withURLParam(req, "id", project.ID)
	callReconcileProjectControlOverseer(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ReconcileProjectControlOverseer first: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var first projectControlOverseerReconcileResponse
	if err := json.NewDecoder(w.Body).Decode(&first); err != nil {
		t.Fatalf("ReconcileProjectControlOverseer first: decode response: %v", err)
	}
	if first.ProjectID != project.ID {
		t.Fatalf("ReconcileProjectControlOverseer first: expected project_id %q, got %q", project.ID, first.ProjectID)
	}
	if !first.CreatedAutopilot {
		t.Fatal("ReconcileProjectControlOverseer first: expected created_autopilot true")
	}
	if !first.CreatedTrigger {
		t.Fatal("ReconcileProjectControlOverseer first: expected created_trigger true")
	}
	if !first.UpdatedProjectLink {
		t.Fatal("ReconcileProjectControlOverseer first: expected updated_project_link true")
	}
	if first.Autonomy == nil {
		t.Fatal("ReconcileProjectControlOverseer first: expected autonomy summary")
	}
	autonomyDetails := loadProjectLinkedOverseerAutonomyDetails(t, project.ID)
	assertProjectControlOverseerReconcileAutonomy(t, first, autonomyDetails)
	assertProjectLinkedOverseerAutopilot(t, project.ID, overseerAgentID, autonomyDetails.AutopilotID, "0 */12 * * *")

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+project.ID+"/control/overseer/reconcile", nil)
	req = withURLParam(req, "id", project.ID)
	callReconcileProjectControlOverseer(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ReconcileProjectControlOverseer second: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var second projectControlOverseerReconcileResponse
	if err := json.NewDecoder(w.Body).Decode(&second); err != nil {
		t.Fatalf("ReconcileProjectControlOverseer second: decode response: %v", err)
	}
	if second.CreatedAutopilot || second.UpdatedAutopilot || second.CreatedTrigger || second.UpdatedTrigger || second.UpdatedProjectLink {
		t.Fatalf("ReconcileProjectControlOverseer second: expected idempotent summary, got %+v", second)
	}
	assertProjectControlOverseerReconcileAutonomy(t, second, autonomyDetails)

	var triggerCount int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM autopilot_trigger t
		JOIN project_control_settings pcs ON pcs.overseer_autopilot_id = t.autopilot_id
		WHERE pcs.project_id = $1 AND t.kind = 'schedule'
	`, project.ID).Scan(&triggerCount); err != nil {
		t.Fatalf("count overseer schedule triggers: %v", err)
	}
	if triggerCount != 1 {
		t.Fatalf("expected 1 overseer schedule trigger after idempotent reconcile, got %d", triggerCount)
	}
}

func TestProjectControlOverseerReconcileRejectsInvalidScanIntervalHours(t *testing.T) {
	project := createPipelineTestProject(t, "Project control overseer reconcile invalid interval")
	overseerAgentID := createHandlerTestAgent(t, "Project control reconcile invalid overseer", nil)

	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO project_control_settings (project_id, overseer_agent_id, overseer_config)
		VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (project_id) DO UPDATE
		SET overseer_agent_id = EXCLUDED.overseer_agent_id,
		    overseer_config = EXCLUDED.overseer_config,
		    updated_at = now()
	`, project.ID, overseerAgentID, `{"scan_interval_hours":5}`); err != nil {
		t.Fatalf("insert invalid interval fixture: %v", err)
	}

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects/"+project.ID+"/control/overseer/reconcile", nil)
	req = withURLParam(req, "id", project.ID)
	callReconcileProjectControlOverseer(t, w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ReconcileProjectControlOverseer invalid interval: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got == "" || !containsJSONError(got, "scan_interval_hours") {
		t.Fatalf("ReconcileProjectControlOverseer invalid interval: expected scan_interval_hours error, got %s", got)
	}
}

func assertProjectControlOverseerReconcileAutonomy(t *testing.T, got projectControlOverseerReconcileResponse, want projectOverseerAutonomyDetails) {
	t.Helper()

	if got.Autonomy == nil {
		t.Fatal("expected autonomy summary to be populated")
	}
	if got.Autonomy.AutopilotID != want.AutopilotID {
		t.Fatalf("expected autonomy.autopilot_id %q, got %q", want.AutopilotID, got.Autonomy.AutopilotID)
	}
	if got.Autonomy.Status != want.Status {
		t.Fatalf("expected autonomy.status %q, got %q", want.Status, got.Autonomy.Status)
	}
	if stringPtrValue(got.Autonomy.TriggerID) != stringPtrValue(want.TriggerID) {
		t.Fatalf("expected autonomy.trigger_id %v, got %v", want.TriggerID, got.Autonomy.TriggerID)
	}
	if stringPtrValue(got.Autonomy.NextRunAt) != stringPtrValue(want.NextRunAt) {
		t.Fatalf("expected autonomy.next_run_at %v, got %v", want.NextRunAt, got.Autonomy.NextRunAt)
	}
}

func containsJSONError(body, substring string) bool {
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return false
	}
	message, _ := payload["error"].(string)
	return message != "" && strings.Contains(message, substring)
}
