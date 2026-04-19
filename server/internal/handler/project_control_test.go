package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type projectOverseerAutonomyResponse struct {
	AutopilotID string  `json:"autopilot_id"`
	Status      string  `json:"status"`
	TriggerID   *string `json:"trigger_id"`
	NextRunAt   *string `json:"next_run_at"`
}

type projectControlResponse struct {
	ProjectID                  string                           `json:"project_id"`
	OverseerAgentID            *string                          `json:"overseer_agent_id"`
	OverseerAutopilotID        *string                          `json:"overseer_autopilot_id"`
	OverseerAutonomyStatus     *string                          `json:"overseer_autonomy_status"`
	OverseerAutonomyTriggerID  *string                          `json:"overseer_autonomy_trigger_id"`
	OverseerAutonomyNextRunAt  *string                          `json:"overseer_autonomy_next_run_at"`
	OverseerAutonomy           *projectOverseerAutonomyResponse `json:"overseer_autonomy"`
	DefaultPipelineID          *string                          `json:"default_pipeline_id"`
	AutomationMode             string                           `json:"automation_mode"`
	AutoTriageEnabled          bool                             `json:"auto_triage_enabled"`
	AutoRouteEnabled           bool                             `json:"auto_route_enabled"`
	AutoEscalateBlocked        bool                             `json:"auto_escalate_blocked"`
	StaleAfterMinutes          int32                            `json:"stale_after_minutes"`
	ReviewPolicy               json.RawMessage                  `json:"review_policy"`
	QualityPolicy              json.RawMessage                  `json:"quality_policy"`
	OverseerConfig             json.RawMessage                  `json:"overseer_config"`
	CreatedAt                  string                           `json:"created_at"`
	UpdatedAt                  string                           `json:"updated_at"`
}

type getProjectControlHandler interface {
	GetProjectControl(http.ResponseWriter, *http.Request)
}

type upsertProjectControlHandler interface {
	UpsertProjectControl(http.ResponseWriter, *http.Request)
}

func callGetProjectControl(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(getProjectControlHandler)
	if !ok {
		t.Fatalf("handler is missing method GetProjectControl")
	}

	h.GetProjectControl(w, req)
}

func callUpsertProjectControl(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(upsertProjectControlHandler)
	if !ok {
		t.Fatalf("handler is missing method UpsertProjectControl")
	}

	h.UpsertProjectControl(w, req)
}

func createProjectControlForeignWorkspaceFixtures(t *testing.T) (string, string) {
	t.Helper()

	ctx := context.Background()
	slug := fmt.Sprintf("project-control-%d", time.Now().UnixNano())

	var workspaceID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description, issue_prefix)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, "Project Control Foreign Workspace", slug, "Temporary workspace for project control tests", "PCF").Scan(&workspaceID); err != nil {
		t.Fatalf("insert foreign workspace fixture: %v", err)
	}

	var runtimeID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status, device_info, metadata, last_seen_at
		)
		VALUES ($1, NULL, $2, 'cloud', $3, 'online', $4, '{}'::jsonb, now())
		RETURNING id
	`, workspaceID, "Project Control Foreign Runtime", "project_control_foreign_runtime", "Project control foreign runtime").Scan(&runtimeID); err != nil {
		t.Fatalf("insert foreign runtime fixture: %v", err)
	}

	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, visibility, max_concurrent_tasks, owner_id
		)
		VALUES ($1, $2, '', 'cloud', '{}'::jsonb, $3, 'workspace', 1, $4)
		RETURNING id
	`, workspaceID, "Project Control Foreign Agent", runtimeID, testUserID).Scan(&agentID); err != nil {
		t.Fatalf("insert foreign agent fixture: %v", err)
	}

	var pipelineID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO pipelines (workspace_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id
	`, workspaceID, "Project Control Foreign Pipeline", "Foreign pipeline for project control tests").Scan(&pipelineID); err != nil {
		t.Fatalf("insert foreign pipeline fixture: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID); err != nil {
			t.Errorf("cleanup foreign workspace fixture: %v", err)
		}
	})

	return agentID, pipelineID
}

func TestProjectControlGet(t *testing.T) {
	project := createPipelineTestProject(t, "Project control get")

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/projects/"+project.ID+"/control", nil)
	req = withURLParam(req, "id", project.ID)
	callGetProjectControl(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetProjectControl: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got projectControlResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("GetProjectControl: decode response: %v", err)
	}

	if got.ProjectID != project.ID {
		t.Fatalf("GetProjectControl: expected project_id %q, got %q", project.ID, got.ProjectID)
	}
	if got.OverseerAgentID != nil {
		t.Fatalf("GetProjectControl: expected nil overseer_agent_id, got %v", *got.OverseerAgentID)
	}
	if got.OverseerAutopilotID != nil {
		t.Fatalf("GetProjectControl: expected nil overseer_autopilot_id, got %v", *got.OverseerAutopilotID)
	}
	if got.OverseerAutonomyStatus != nil {
		t.Fatalf("GetProjectControl: expected nil overseer_autonomy_status, got %v", *got.OverseerAutonomyStatus)
	}
	if got.OverseerAutonomyTriggerID != nil {
		t.Fatalf("GetProjectControl: expected nil overseer_autonomy_trigger_id, got %v", *got.OverseerAutonomyTriggerID)
	}
	if got.OverseerAutonomyNextRunAt != nil {
		t.Fatalf("GetProjectControl: expected nil overseer_autonomy_next_run_at, got %v", *got.OverseerAutonomyNextRunAt)
	}
	if got.OverseerAutonomy != nil {
		t.Fatalf("GetProjectControl: expected nil overseer_autonomy, got %+v", got.OverseerAutonomy)
	}
	if got.DefaultPipelineID != nil {
		t.Fatalf("GetProjectControl: expected nil default_pipeline_id, got %v", *got.DefaultPipelineID)
	}
	if got.AutomationMode != "manual" {
		t.Fatalf("GetProjectControl: expected automation_mode manual, got %q", got.AutomationMode)
	}
	if got.AutoTriageEnabled {
		t.Fatalf("GetProjectControl: expected auto_triage_enabled false")
	}
	if got.AutoRouteEnabled {
		t.Fatalf("GetProjectControl: expected auto_route_enabled false")
	}
	if got.AutoEscalateBlocked {
		t.Fatalf("GetProjectControl: expected auto_escalate_blocked false")
	}
	if got.StaleAfterMinutes != 60 {
		t.Fatalf("GetProjectControl: expected stale_after_minutes 60, got %d", got.StaleAfterMinutes)
	}
	assertJSONEqual(t, got.ReviewPolicy, `{}`)
	assertJSONEqual(t, got.QualityPolicy, `{}`)
	assertJSONEqual(t, got.OverseerConfig, `{}`)
	if got.CreatedAt == "" {
		t.Fatal("GetProjectControl: expected created_at to be populated")
	}
	if got.UpdatedAt == "" {
		t.Fatal("GetProjectControl: expected updated_at to be populated")
	}
}

func TestProjectControlUpsert(t *testing.T) {
	project := createPipelineTestProject(t, "Project control upsert")
	overseerAgentID := createHandlerTestAgent(t, "Project Control Overseer Agent", nil)
	pipelineID, _ := insertPipelineFixture(t, "Project control pipeline", []pipelineStageRequest{{
		Name:     "Build",
		Status:   "done",
		AgentID:  overseerAgentID,
		Position: 1,
	}})

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/projects/"+project.ID+"/control", map[string]any{
		"overseer_agent_id":     overseerAgentID,
		"default_pipeline_id":   pipelineID,
		"automation_mode":       "assisted",
		"auto_triage_enabled":   true,
		"auto_route_enabled":    true,
		"auto_escalate_blocked": true,
		"stale_after_minutes":   120,
		"review_policy":         map[string]any{"required_reviewers": 2},
		"quality_policy":        map[string]any{"minimum_score": 90},
		"overseer_config": map[string]any{
			"scan_interval_hours": 12,
			"scan_focus":          []string{"security", "test_coverage", "code_quality"},
			"product_context":     "Internal AI-native mission control fork for autonomous software delivery",
			"quality_bar":         []string{"tests_required", "docs_updated", "merge_safe"},
			"priority_weights": map[string]any{
				"security":      10,
				"architecture":  7,
				"documentation": 4,
			},
			"max_issues_per_run": 3,
			"require_approval":   true,
		},
	})
	req = withURLParam(req, "id", project.ID)
	callUpsertProjectControl(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpsertProjectControl create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var created projectControlResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("UpsertProjectControl create: decode response: %v", err)
	}
	if created.OverseerAgentID == nil || *created.OverseerAgentID != overseerAgentID {
		t.Fatalf("UpsertProjectControl create: expected overseer_agent_id %q, got %v", overseerAgentID, created.OverseerAgentID)
	}
	if created.OverseerAutopilotID == nil {
		t.Fatalf("UpsertProjectControl create: expected overseer_autopilot_id to be populated")
	}
	autonomyDetails := loadProjectLinkedOverseerAutonomyDetails(t, project.ID)
	assertProjectControlAutonomyResponse(t, created, autonomyDetails)
	assertProjectLinkedOverseerAutopilot(t, project.ID, overseerAgentID, *created.OverseerAutopilotID, "0 */12 * * *")
	if created.DefaultPipelineID == nil || *created.DefaultPipelineID != pipelineID {
		t.Fatalf("UpsertProjectControl create: expected default_pipeline_id %q, got %v", pipelineID, created.DefaultPipelineID)
	}
	if created.AutomationMode != "assisted" {
		t.Fatalf("UpsertProjectControl create: expected automation_mode assisted, got %q", created.AutomationMode)
	}
	if !created.AutoTriageEnabled || !created.AutoRouteEnabled || !created.AutoEscalateBlocked {
		t.Fatalf("UpsertProjectControl create: expected automation flags to be true, got %+v", created)
	}
	if created.StaleAfterMinutes != 120 {
		t.Fatalf("UpsertProjectControl create: expected stale_after_minutes 120, got %d", created.StaleAfterMinutes)
	}
	assertJSONEqual(t, created.ReviewPolicy, `{"required_reviewers":2}`)
	assertJSONEqual(t, created.QualityPolicy, `{"minimum_score":90}`)
	assertJSONEqual(t, created.OverseerConfig, `{
		"scan_interval_hours":12,
		"scan_focus":["security","test_coverage","code_quality"],
		"product_context":"Internal AI-native mission control fork for autonomous software delivery",
		"quality_bar":["tests_required","docs_updated","merge_safe"],
		"priority_weights":{"security":10,"architecture":7,"documentation":4},
		"max_issues_per_run":3,
		"require_approval":true
	}`)

	w = httptest.NewRecorder()
	req = newRequest("PUT", "/api/projects/"+project.ID+"/control", map[string]any{
		"overseer_agent_id":   nil,
		"default_pipeline_id": nil,
	})
	req = withURLParam(req, "id", project.ID)
	callUpsertProjectControl(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpsertProjectControl clear IDs: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cleared projectControlResponse
	if err := json.NewDecoder(w.Body).Decode(&cleared); err != nil {
		t.Fatalf("UpsertProjectControl clear IDs: decode response: %v", err)
	}
	if cleared.OverseerAgentID != nil {
		t.Fatalf("UpsertProjectControl clear IDs: expected nil overseer_agent_id, got %v", *cleared.OverseerAgentID)
	}
	if cleared.OverseerAutopilotID == nil {
		t.Fatalf("UpsertProjectControl clear IDs: expected managed overseer autopilot link to remain present")
	}
	autonomyDetails = loadProjectLinkedOverseerAutonomyDetails(t, project.ID)
	assertProjectControlAutonomyResponse(t, cleared, autonomyDetails)
	assertProjectLinkedOverseerAutopilotPaused(t, project.ID, *cleared.OverseerAutopilotID)
	if cleared.DefaultPipelineID != nil {
		t.Fatalf("UpsertProjectControl clear IDs: expected nil default_pipeline_id, got %v", *cleared.DefaultPipelineID)
	}
	if cleared.AutomationMode != "assisted" {
		t.Fatalf("UpsertProjectControl clear IDs: expected automation_mode to be preserved, got %q", cleared.AutomationMode)
	}
	if cleared.StaleAfterMinutes != 120 {
		t.Fatalf("UpsertProjectControl clear IDs: expected stale_after_minutes to be preserved, got %d", cleared.StaleAfterMinutes)
	}
	assertJSONEqual(t, cleared.ReviewPolicy, `{"required_reviewers":2}`)
	assertJSONEqual(t, cleared.QualityPolicy, `{"minimum_score":90}`)
	assertJSONEqual(t, cleared.OverseerConfig, `{
		"scan_interval_hours":12,
		"scan_focus":["security","test_coverage","code_quality"],
		"product_context":"Internal AI-native mission control fork for autonomous software delivery",
		"quality_bar":["tests_required","docs_updated","merge_safe"],
		"priority_weights":{"security":10,"architecture":7,"documentation":4},
		"max_issues_per_run":3,
		"require_approval":true
	}`)

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/projects/"+project.ID+"/control", nil)
	req = withURLParam(req, "id", project.ID)
	callGetProjectControl(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetProjectControl after upsert: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var fetched projectControlResponse
	if err := json.NewDecoder(w.Body).Decode(&fetched); err != nil {
		t.Fatalf("GetProjectControl after upsert: decode response: %v", err)
	}
	if fetched.OverseerAgentID != nil || fetched.DefaultPipelineID != nil {
		t.Fatalf("GetProjectControl after upsert: expected cleared IDs, got %+v", fetched)
	}
	if fetched.OverseerAutopilotID == nil {
		t.Fatalf("GetProjectControl after upsert: expected overseer_autopilot_id to remain linked")
	}
	autonomyDetails = loadProjectLinkedOverseerAutonomyDetails(t, project.ID)
	assertProjectControlAutonomyResponse(t, fetched, autonomyDetails)
	assertProjectLinkedOverseerAutopilotPaused(t, project.ID, *fetched.OverseerAutopilotID)
	if fetched.AutomationMode != "assisted" || fetched.StaleAfterMinutes != 120 {
		t.Fatalf("GetProjectControl after upsert: expected persisted values, got %+v", fetched)
	}
	assertJSONEqual(t, fetched.OverseerConfig, `{
		"scan_interval_hours":12,
		"scan_focus":["security","test_coverage","code_quality"],
		"product_context":"Internal AI-native mission control fork for autonomous software delivery",
		"quality_bar":["tests_required","docs_updated","merge_safe"],
		"priority_weights":{"security":10,"architecture":7,"documentation":4},
		"max_issues_per_run":3,
		"require_approval":true
	}`)
}

func TestProjectControlRejectsOverseerOutsideWorkspace(t *testing.T) {
	project := createPipelineTestProject(t, "Project control invalid overseer")
	foreignAgentID, _ := createProjectControlForeignWorkspaceFixtures(t)

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/projects/"+project.ID+"/control", map[string]any{
		"overseer_agent_id": foreignAgentID,
	})
	req = withURLParam(req, "id", project.ID)
	callUpsertProjectControl(t, w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("UpsertProjectControl invalid overseer: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "overseer_agent_id") {
		t.Fatalf("UpsertProjectControl invalid overseer: expected error to mention overseer_agent_id, got %q", w.Body.String())
	}
}

func TestProjectControlRejectsDefaultPipelineOutsideWorkspace(t *testing.T) {
	project := createPipelineTestProject(t, "Project control invalid pipeline")
	_, foreignPipelineID := createProjectControlForeignWorkspaceFixtures(t)

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/projects/"+project.ID+"/control", map[string]any{
		"default_pipeline_id": foreignPipelineID,
	})
	req = withURLParam(req, "id", project.ID)
	callUpsertProjectControl(t, w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("UpsertProjectControl invalid pipeline: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "default_pipeline_id") {
		t.Fatalf("UpsertProjectControl invalid pipeline: expected error to mention default_pipeline_id, got %q", w.Body.String())
	}
}

func TestProjectControlRejectsInvalidAutomationMode(t *testing.T) {
	project := createPipelineTestProject(t, "Project control invalid automation mode")

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/projects/"+project.ID+"/control", map[string]any{
		"automation_mode": "robotic",
	})
	req = withURLParam(req, "id", project.ID)
	callUpsertProjectControl(t, w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("UpsertProjectControl invalid automation_mode: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "automation_mode") {
		t.Fatalf("UpsertProjectControl invalid automation_mode: expected error to mention automation_mode, got %q", w.Body.String())
	}
}

func TestProjectControlRejectsNonPositiveStaleAfterMinutes(t *testing.T) {
	project := createPipelineTestProject(t, "Project control invalid stale after")

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/projects/"+project.ID+"/control", map[string]any{
		"stale_after_minutes": 0,
	})
	req = withURLParam(req, "id", project.ID)
	callUpsertProjectControl(t, w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("UpsertProjectControl invalid stale_after_minutes: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "stale_after_minutes") {
		t.Fatalf("UpsertProjectControl invalid stale_after_minutes: expected error to mention stale_after_minutes, got %q", w.Body.String())
	}
}

func TestProjectControlRejectsInvalidOverseerConfig(t *testing.T) {
	project := createPipelineTestProject(t, "Project control invalid overseer config")

	tests := []struct {
		name      string
		body      string
		errorHint string
	}{
		{
			name:      "scan_interval_hours out of range",
			body:      `{"overseer_config":{"scan_interval_hours":5}}`,
			errorHint: "scan_interval_hours",
		},
		{
			name:      "scan_focus invalid value",
			body:      `{"overseer_config":{"scan_focus":["security","chaos"]}}`,
			errorHint: "scan_focus",
		},
		{
			name:      "product_context too long",
			body:      `{"overseer_config":{"product_context":"................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................."}}`,
			errorHint: "product_context",
		},
		{
			name:      "max_issues_per_run too high",
			body:      `{"overseer_config":{"max_issues_per_run":4}}`,
			errorHint: "max_issues_per_run",
		},
		{
			name:      "require_approval wrong type",
			body:      `{"overseer_config":{"require_approval":"yes"}}`,
			errorHint: "require_approval",
		},
		{
			name:      "priority_weights wrong shape",
			body:      `{"overseer_config":{"priority_weights":[1,2,3]}}`,
			errorHint: "priority_weights",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("PUT", "/api/projects/"+project.ID+"/control", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-User-ID", testUserID)
			req.Header.Set("X-Workspace-ID", testWorkspaceID)
			req = withURLParam(req, "id", project.ID)

			callUpsertProjectControl(t, w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("UpsertProjectControl invalid overseer_config (%s): expected 400, got %d: %s", tt.name, w.Code, w.Body.String())
			}
			if !strings.Contains(strings.ToLower(w.Body.String()), tt.errorHint) {
				t.Fatalf("UpsertProjectControl invalid overseer_config (%s): expected error to mention %q, got %q", tt.name, tt.errorHint, w.Body.String())
			}
		})
	}
}
