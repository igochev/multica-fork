package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type projectControlOverseerReconcileAPIResponse struct {
	ProjectID          string `json:"project_id"`
	CreatedAutopilot   bool   `json:"created_autopilot"`
	CreatedTrigger     bool   `json:"created_trigger"`
	UpdatedProjectLink bool   `json:"updated_project_link"`
	Autonomy           *struct {
		AutopilotID string  `json:"autopilot_id"`
		Status      string  `json:"status"`
		TriggerID   *string `json:"trigger_id"`
		NextRunAt   *string `json:"next_run_at"`
	} `json:"autonomy"`
}

func TestProjectControlOverseerReconcileRoute(t *testing.T) {
	projectID := createIntegrationTestProject(t, "router overseer reconcile")
	overseerAgentID := getAgentID(t)

	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO project_control_settings (project_id, overseer_agent_id, overseer_config)
		VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (project_id) DO UPDATE
		SET overseer_agent_id = EXCLUDED.overseer_agent_id,
		    overseer_config = EXCLUDED.overseer_config,
		    overseer_autopilot_id = NULL,
		    updated_at = now()
	`, projectID, overseerAgentID, `{"scan_interval_hours":12,"scan_focus":["security"]}`); err != nil {
		t.Fatalf("insert project control settings: %v", err)
	}

	resp := authRequest(t, http.MethodPost, "/api/projects/"+projectID+"/control/overseer/reconcile", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got projectControlOverseerReconcileAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ProjectID != projectID {
		t.Fatalf("expected project_id %q, got %q", projectID, got.ProjectID)
	}
	if !got.CreatedAutopilot || !got.CreatedTrigger || !got.UpdatedProjectLink {
		t.Fatalf("expected creation summary, got %+v", got)
	}
	if got.Autonomy == nil {
		t.Fatal("expected autonomy summary")
	}
	if got.Autonomy.AutopilotID == "" || got.Autonomy.Status != "active" || got.Autonomy.TriggerID == nil || got.Autonomy.NextRunAt == nil {
		t.Fatalf("expected populated autonomy summary, got %+v", got.Autonomy)
	}
}

func createIntegrationTestProject(t *testing.T, title string) string {
	t.Helper()

	resp := authRequest(t, http.MethodPost, "/api/projects", map[string]any{"title": title})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d", resp.StatusCode)
	}
	var project struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatalf("decode project response: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM project WHERE id = $1`, project.ID)
	})
	return project.ID
}
