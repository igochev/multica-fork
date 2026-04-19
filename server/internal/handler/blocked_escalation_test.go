package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func insertBlockedEscalationProjectControlFixture(t *testing.T, projectID, overseerAgentID string, autoEscalate bool) {
	t.Helper()

	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `
		INSERT INTO project_control_settings (project_id, overseer_agent_id, auto_escalate_blocked)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id) DO UPDATE
		SET overseer_agent_id = EXCLUDED.overseer_agent_id,
		    auto_escalate_blocked = EXCLUDED.auto_escalate_blocked
	`, projectID, overseerAgentID, autoEscalate); err != nil {
		t.Fatalf("insert blocked escalation project control fixture: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(ctx, `DELETE FROM project_control_settings WHERE project_id = $1`, projectID); err != nil {
			t.Errorf("cleanup blocked escalation project control fixture: %v", err)
		}
	})
}

func activeTaskCountForIssueAndAgent(t *testing.T, issueID, agentID string) int {
	t.Helper()

	var count int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2 AND status IN ('queued', 'dispatched', 'running')
	`, issueID, agentID).Scan(&count); err != nil {
		t.Fatalf("count active tasks: %v", err)
	}

	return count
}

func latestActivityForIssueAction(t *testing.T, issueID, action string) (string, map[string]any, bool) {
	t.Helper()

	var details []byte
	var actorType string
	err := testPool.QueryRow(context.Background(), `
		SELECT actor_type, details
		FROM activity_log
		WHERE issue_id = $1 AND action = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, issueID, action).Scan(&actorType, &details)
	if err != nil {
		return "", nil, false
	}

	var payload map[string]any
	if err := json.Unmarshal(details, &payload); err != nil {
		t.Fatalf("decode activity details: %v", err)
	}

	return actorType, payload, true
}

func TestBlockedEscalationEnqueuesOverseerTaskOnBlockedTransition(t *testing.T) {
	project := createPipelineTestProject(t, "Blocked escalation project")
	overseerAgentID := createHandlerTestAgent(t, "Blocked escalation overseer", nil)
	insertBlockedEscalationProjectControlFixture(t, project.ID, overseerAgentID, true)

	issue := createPipelineTestIssue(t, map[string]any{
		"title":      "Needs help",
		"status":     "todo",
		"priority":   "high",
		"project_id": project.ID,
	})

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/issues/"+issue.ID, map[string]any{"status": "blocked"})
	req = withURLParam(req, "id", issue.ID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateIssue blocked escalation: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if count := activeTaskCountForIssueAndAgent(t, issue.ID, overseerAgentID); count != 1 {
		t.Fatalf("expected 1 active overseer task, got %d", count)
	}

	actorType, details, found := latestActivityForIssueAction(t, issue.ID, "overseer_requested_for_blocked_issue")
	if !found {
		t.Fatalf("expected overseer activity log entry")
	}
	if actorType != "system" {
		t.Fatalf("expected system actor type, got %q", actorType)
	}
	if details["reason"] != "blocked_issue" {
		t.Fatalf("expected blocked_issue reason, got %#v", details["reason"])
	}
	if details["overseer_agent_id"] != overseerAgentID {
		t.Fatalf("expected overseer_agent_id %q, got %#v", overseerAgentID, details["overseer_agent_id"])
	}
	if _, ok := details["task_id"].(string); !ok || details["task_id"] == "" {
		t.Fatalf("expected activity task_id string, got %#v", details["task_id"])
	}
}

func TestBlockedEscalationDoesNotEnqueueWithoutBlockedTransition(t *testing.T) {
	project := createPipelineTestProject(t, "Blocked escalation no transition")
	overseerAgentID := createHandlerTestAgent(t, "Blocked escalation overseer no transition", nil)
	insertBlockedEscalationProjectControlFixture(t, project.ID, overseerAgentID, true)

	issue := createPipelineTestIssue(t, map[string]any{
		"title":      "Already blocked",
		"status":     "blocked",
		"priority":   "medium",
		"project_id": project.ID,
	})

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/issues/"+issue.ID, map[string]any{"title": "Already blocked, renamed"})
	req = withURLParam(req, "id", issue.ID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateIssue no blocked transition: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if count := activeTaskCountForIssueAndAgent(t, issue.ID, overseerAgentID); count != 0 {
		t.Fatalf("expected no active overseer task, got %d", count)
	}
	if _, _, found := latestActivityForIssueAction(t, issue.ID, "overseer_requested_for_blocked_issue"); found {
		t.Fatalf("expected no overseer activity log without blocked transition")
	}
}
