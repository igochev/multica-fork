package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type projectControlReconcileResponse struct {
	ProjectID         string   `json:"project_id"`
	EscalatedIssueIDs []string `json:"escalated_issue_ids"`
	EscalatedCount    int      `json:"escalated_count"`
}

type projectControlReconcileHandler interface {
	ReconcileProjectControl(http.ResponseWriter, *http.Request)
}

func callReconcileProjectControl(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(projectControlReconcileHandler)
	if !ok {
		t.Fatalf("handler is missing method ReconcileProjectControl")
	}

	h.ReconcileProjectControl(w, req)
}

func insertProjectControlReconcileFixture(t *testing.T, projectID, overseerAgentID string, staleAfterMinutes int) {
	t.Helper()

	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `
		INSERT INTO project_control_settings (project_id, overseer_agent_id, stale_after_minutes)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id) DO UPDATE
		SET overseer_agent_id = EXCLUDED.overseer_agent_id,
		    stale_after_minutes = EXCLUDED.stale_after_minutes
	`, projectID, overseerAgentID, staleAfterMinutes); err != nil {
		t.Fatalf("insert reconcile project control fixture: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `DELETE FROM project_control_settings WHERE project_id = $1`, projectID); err != nil {
			t.Errorf("cleanup reconcile project control fixture: %v", err)
		}
	})
}

func insertIssueTaskFixture(t *testing.T, issueID, agentID, runtimeID, status string, createdAt time.Time) string {
	t.Helper()

	var taskID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_task_queue (agent_id, issue_id, runtime_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, agentID, issueID, runtimeID, status, createdAt).Scan(&taskID); err != nil {
		t.Fatalf("insert issue task fixture: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID); err != nil {
			t.Errorf("cleanup issue task fixture: %v", err)
		}
	})

	return taskID
}

func TestProjectControlReconcileEscalatesStaleIssue(t *testing.T) {
	project := createPipelineTestProject(t, "Project control reconcile stale")
	overseerAgentID := createHandlerTestAgent(t, "Project control reconcile overseer", nil)
	workerAgentID := createHandlerTestAgent(t, "Project control reconcile worker", nil)
	insertProjectControlReconcileFixture(t, project.ID, overseerAgentID, 60)

	staleIssue := createPipelineTestIssue(t, map[string]any{
		"title":      "Stale task issue",
		"status":     "in_progress",
		"priority":   "high",
		"project_id": project.ID,
	})
	freshIssue := createPipelineTestIssue(t, map[string]any{
		"title":      "Fresh task issue",
		"status":     "in_progress",
		"priority":   "medium",
		"project_id": project.ID,
	})

	insertIssueTaskFixture(t, staleIssue.ID, workerAgentID, handlerTestRuntimeID(t), "running", time.Now().Add(-2*time.Hour))
	insertIssueTaskFixture(t, freshIssue.ID, workerAgentID, handlerTestRuntimeID(t), "queued", time.Now().Add(-10*time.Minute))

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects/"+project.ID+"/control/reconcile", nil)
	req = withURLParam(req, "id", project.ID)
	callReconcileProjectControl(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ReconcileProjectControl: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got projectControlReconcileResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("ReconcileProjectControl: decode response: %v", err)
	}
	if got.ProjectID != project.ID {
		t.Fatalf("ReconcileProjectControl: expected project_id %q, got %q", project.ID, got.ProjectID)
	}
	if got.EscalatedCount != 1 {
		t.Fatalf("ReconcileProjectControl: expected escalated_count 1, got %d", got.EscalatedCount)
	}
	if len(got.EscalatedIssueIDs) != 1 || got.EscalatedIssueIDs[0] != staleIssue.ID {
		t.Fatalf("ReconcileProjectControl: expected stale issue %q to be escalated, got %#v", staleIssue.ID, got.EscalatedIssueIDs)
	}

	if count := activeTaskCountForIssueAndAgent(t, staleIssue.ID, overseerAgentID); count != 1 {
		t.Fatalf("expected 1 active overseer task for stale issue, got %d", count)
	}
	if count := activeTaskCountForIssueAndAgent(t, freshIssue.ID, overseerAgentID); count != 0 {
		t.Fatalf("expected no overseer task for fresh issue, got %d", count)
	}

	actorType, details, found := latestActivityForIssueAction(t, staleIssue.ID, "overseer_requested_for_stale_issue")
	if !found {
		t.Fatalf("expected stale issue activity log entry")
	}
	if actorType != "system" {
		t.Fatalf("expected system actor type, got %q", actorType)
	}
	if details["reason"] != "stale_issue" {
		t.Fatalf("expected stale_issue reason, got %#v", details["reason"])
	}
}

func TestProjectControlReconcileSkipsProjectWithoutOverseer(t *testing.T) {
	project := createPipelineTestProject(t, "Project control reconcile no overseer")
	workerAgentID := createHandlerTestAgent(t, "Project control reconcile worker only", nil)
	issue := createPipelineTestIssue(t, map[string]any{
		"title":      "Stale but no overseer",
		"status":     "in_progress",
		"priority":   "high",
		"project_id": project.ID,
	})
	insertIssueTaskFixture(t, issue.ID, workerAgentID, handlerTestRuntimeID(t), "running", time.Now().Add(-2*time.Hour))

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects/"+project.ID+"/control/reconcile", nil)
	req = withURLParam(req, "id", project.ID)
	callReconcileProjectControl(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ReconcileProjectControl no overseer: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got projectControlReconcileResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("ReconcileProjectControl no overseer: decode response: %v", err)
	}
	if got.EscalatedCount != 0 || len(got.EscalatedIssueIDs) != 0 {
		t.Fatalf("ReconcileProjectControl no overseer: expected no escalations, got %#v", got)
	}
	if _, _, found := latestActivityForIssueAction(t, issue.ID, "overseer_requested_for_stale_issue"); found {
		t.Fatalf("expected no stale issue activity without configured overseer")
	}
}
