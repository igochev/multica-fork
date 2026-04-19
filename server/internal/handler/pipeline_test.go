package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type pipelineStageRequest struct {
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	AgentID           string  `json:"agent_id"`
	StageInstructions *string `json:"stage_instructions,omitempty"`
	Position          int     `json:"position"`
}

type pipelineResponse struct {
	ID          string                  `json:"id"`
	WorkspaceID string                  `json:"workspace_id"`
	Name        string                  `json:"name"`
	Description *string                 `json:"description"`
	Stages      []pipelineStageResponse `json:"stages"`
}

type pipelineStageResponse struct {
	ID                string  `json:"id"`
	PipelineID        string  `json:"pipeline_id"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	AgentID           string  `json:"agent_id"`
	StageInstructions *string `json:"stage_instructions,omitempty"`
	Position          int     `json:"position"`
}

type createPipelineHandler interface {
	CreatePipeline(http.ResponseWriter, *http.Request)
}

type listPipelinesHandler interface {
	ListPipelines(http.ResponseWriter, *http.Request)
}

type updatePipelineHandler interface {
	UpdatePipeline(http.ResponseWriter, *http.Request)
}

type deletePipelineHandler interface {
	DeletePipeline(http.ResponseWriter, *http.Request)
}

type getPipelineHandler interface {
	GetPipeline(http.ResponseWriter, *http.Request)
}

func callCreatePipeline(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(createPipelineHandler)
	if !ok {
		t.Fatalf("handler is missing method CreatePipeline")
	}

	h.CreatePipeline(w, req)
}

func callListPipelines(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(listPipelinesHandler)
	if !ok {
		t.Fatalf("handler is missing method ListPipelines")
	}

	h.ListPipelines(w, req)
}

func callUpdatePipeline(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(updatePipelineHandler)
	if !ok {
		t.Fatalf("handler is missing method UpdatePipeline")
	}

	h.UpdatePipeline(w, req)
}

func callDeletePipeline(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(deletePipelineHandler)
	if !ok {
		t.Fatalf("handler is missing method DeletePipeline")
	}

	h.DeletePipeline(w, req)
}

func callGetPipeline(t *testing.T, w http.ResponseWriter, req *http.Request) {
	t.Helper()

	h, ok := any(testHandler).(getPipelineHandler)
	if !ok {
		t.Fatalf("handler is missing method GetPipeline")
	}

	h.GetPipeline(w, req)
}

func createPipelineTestProject(t *testing.T, title string) ProjectResponse {
	t.Helper()

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": title,
	})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("CreateProject: decode response: %v", err)
	}

	t.Cleanup(func() {
		cleanupReq := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		cleanupReq = withURLParam(cleanupReq, "id", project.ID)
		w := httptest.NewRecorder()
		testHandler.DeleteProject(w, cleanupReq)
		if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
			t.Errorf("DeleteProject cleanup: expected 200/204, got %d: %s", w.Code, w.Body.String())
		}
	})

	return project
}

func createPipelineTestIssue(t *testing.T, body map[string]any) IssueResponse {
	t.Helper()

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, body)
	testHandler.CreateIssue(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateIssue: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var issue IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&issue); err != nil {
		t.Fatalf("CreateIssue: decode response: %v", err)
	}

	t.Cleanup(func() {
		cleanupReq := newRequest("DELETE", "/api/issues/"+issue.ID, nil)
		cleanupReq = withURLParam(cleanupReq, "id", issue.ID)
		w := httptest.NewRecorder()
		testHandler.DeleteIssue(w, cleanupReq)
		if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
			t.Errorf("DeleteIssue cleanup: expected 200/204, got %d: %s", w.Code, w.Body.String())
		}
	})

	return issue
}

func insertPipelineFixture(t *testing.T, name string, stages []pipelineStageRequest) (string, []string) {
	t.Helper()

	ctx := context.Background()
	var pipelineID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO pipelines (workspace_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id
	`, testWorkspaceID, name, name+" description").Scan(&pipelineID); err != nil {
		t.Fatalf("insert pipeline fixture: %v", err)
	}

	stageIDs := make([]string, 0, len(stages))
	for _, stage := range stages {
		var stageID string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO pipeline_stages (pipeline_id, name, status, agent_id, stage_instructions, position)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, pipelineID, stage.Name, stage.Status, stage.AgentID, stage.StageInstructions, stage.Position).Scan(&stageID); err != nil {
			t.Fatalf("insert pipeline stage fixture: %v", err)
		}
		stageIDs = append(stageIDs, stageID)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(ctx, `DELETE FROM pipelines WHERE id = $1`, pipelineID); err != nil {
			t.Errorf("cleanup pipeline fixture: %v", err)
		}
	})

	return pipelineID, stageIDs
}

func insertProjectControlSettingsFixture(t *testing.T, projectID, pipelineID string) {
	t.Helper()

	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `
		INSERT INTO project_control_settings (project_id, default_pipeline_id)
		VALUES ($1, $2)
	`, projectID, pipelineID); err != nil {
		t.Fatalf("insert project_control_settings fixture: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(ctx, `DELETE FROM project_control_settings WHERE project_id = $1`, projectID); err != nil {
			t.Errorf("cleanup project_control_settings fixture: %v", err)
		}
	})
}

func insertIssuePipelineFixture(t *testing.T, issueID, pipelineID, currentStageID string, stageSequence int) {
	t.Helper()

	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `
		INSERT INTO issue_pipelines (issue_id, pipeline_id, current_stage_id, stage_sequence)
		VALUES ($1, $2, $3, $4)
	`, issueID, pipelineID, currentStageID, stageSequence); err != nil {
		t.Fatalf("insert issue_pipelines fixture: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(ctx, `DELETE FROM issue_pipelines WHERE issue_id = $1`, issueID); err != nil {
			t.Errorf("cleanup issue_pipelines fixture: %v", err)
		}
	})
}

func queuedTaskCountForIssueAndAgent(t *testing.T, issueID, agentID string) int {
	t.Helper()

	var count int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2 AND status = 'queued'
	`, issueID, agentID).Scan(&count); err != nil {
		t.Fatalf("count queued tasks: %v", err)
	}

	return count
}

func TestPipelineCreate(t *testing.T) {
	buildAgentID := createHandlerTestAgent(t, "Pipeline Build Agent", nil)
	reviewAgentID := createHandlerTestAgent(t, "Pipeline Review Agent", nil)

	createBody := map[string]any{
		"name":        "Autonomous Delivery",
		"description": "Build, review, deploy",
		"stages": []pipelineStageRequest{
			{Name: "Build", Status: "done", AgentID: buildAgentID, Position: 1},
			{Name: "Review", Status: "in_review", AgentID: reviewAgentID, Position: 2},
		},
	}

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/pipelines?workspace_id="+testWorkspaceID, createBody)
	callCreatePipeline(t, w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreatePipeline: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created pipelineResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("CreatePipeline: decode response: %v", err)
	}
	t.Cleanup(func() {
		cleanupReq := newRequest("DELETE", "/api/pipelines/"+created.ID, nil)
		cleanupReq = withURLParam(cleanupReq, "id", created.ID)
		w := httptest.NewRecorder()
		callDeletePipeline(t, w, cleanupReq)
		if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
			t.Errorf("DeletePipeline cleanup: expected 200/204, got %d: %s", w.Code, w.Body.String())
		}
	})

	if created.Name != "Autonomous Delivery" {
		t.Fatalf("CreatePipeline: expected name %q, got %q", "Autonomous Delivery", created.Name)
	}
	if len(created.Stages) != 2 {
		t.Fatalf("CreatePipeline: expected 2 stages, got %d", len(created.Stages))
	}
	if created.Stages[0].Position != 1 || created.Stages[0].Name != "Build" {
		t.Fatalf("CreatePipeline: expected first stage to remain ordered, got %+v", created.Stages[0])
	}
	if created.Stages[1].Position != 2 || created.Stages[1].Name != "Review" {
		t.Fatalf("CreatePipeline: expected second stage to remain ordered, got %+v", created.Stages[1])
	}
}

func TestPipelineList(t *testing.T) {
	buildAgentID := createHandlerTestAgent(t, "Pipeline List Build Agent", nil)
	reviewAgentID := createHandlerTestAgent(t, "Pipeline List Review Agent", nil)

	pipelineID, _ := insertPipelineFixture(t, "Listed pipeline", []pipelineStageRequest{
		{Name: "Build", Status: "done", AgentID: buildAgentID, Position: 1},
		{Name: "Review", Status: "in_review", AgentID: reviewAgentID, Position: 2},
	})

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/pipelines?workspace_id="+testWorkspaceID, nil)
	callListPipelines(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListPipelines: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listed struct {
		Pipelines []pipelineResponse `json:"pipelines"`
		Total     int                `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&listed); err != nil {
		t.Fatalf("ListPipelines: decode response: %v", err)
	}
	if listed.Total < 1 {
		t.Fatalf("ListPipelines: expected at least 1 pipeline, got %d", listed.Total)
	}

	found := false
	for _, pipeline := range listed.Pipelines {
		if pipeline.ID == pipelineID && pipeline.Name == "Listed pipeline" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListPipelines: expected response to include pipeline %q (%s)", "Listed pipeline", pipelineID)
	}
}

func TestPipelineGet(t *testing.T) {
	buildAgentID := createHandlerTestAgent(t, "Pipeline Get Build Agent", nil)
	reviewAgentID := createHandlerTestAgent(t, "Pipeline Get Review Agent", nil)

	pipelineID, _ := insertPipelineFixture(t, "Fetched pipeline", []pipelineStageRequest{
		{Name: "Review", Status: "in_review", AgentID: reviewAgentID, Position: 2},
		{Name: "Build", Status: "done", AgentID: buildAgentID, Position: 1},
	})

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/pipelines/"+pipelineID, nil)
	req = withURLParam(req, "id", pipelineID)
	callGetPipeline(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetPipeline: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got pipelineResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("GetPipeline: decode response: %v", err)
	}
	if got.ID != pipelineID {
		t.Fatalf("GetPipeline: expected id %q, got %q", pipelineID, got.ID)
	}
	if len(got.Stages) != 2 {
		t.Fatalf("GetPipeline: expected 2 stages, got %d", len(got.Stages))
	}
	if got.Stages[0].Position != 1 || got.Stages[0].Name != "Build" {
		t.Fatalf("GetPipeline: expected first stage ordered by position, got %+v", got.Stages[0])
	}
	if got.Stages[1].Position != 2 || got.Stages[1].Name != "Review" {
		t.Fatalf("GetPipeline: expected second stage ordered by position, got %+v", got.Stages[1])
	}
}

func TestPipelineUpdate(t *testing.T) {
	buildAgentID := createHandlerTestAgent(t, "Pipeline Update Build Agent", nil)
	reviewAgentID := createHandlerTestAgent(t, "Pipeline Update Review Agent", nil)
	deployAgentID := createHandlerTestAgent(t, "Pipeline Update Deploy Agent", nil)

	pipelineID, _ := insertPipelineFixture(t, "Pipeline to update", []pipelineStageRequest{
		{Name: "Build", Status: "done", AgentID: buildAgentID, Position: 1},
		{Name: "Review", Status: "in_review", AgentID: reviewAgentID, Position: 2},
	})

	updateBody := map[string]any{
		"name":        "Autonomous Delivery v2",
		"description": "Build, review, deploy with release handoff",
		"stages": []pipelineStageRequest{
			{Name: "Build", Status: "done", AgentID: buildAgentID, Position: 1},
			{Name: "Review", Status: "in_review", AgentID: reviewAgentID, Position: 2},
			{Name: "Deploy", Status: "cancelled", AgentID: deployAgentID, Position: 3},
		},
	}
	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/pipelines/"+pipelineID, updateBody)
	req = withURLParam(req, "id", pipelineID)
	callUpdatePipeline(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdatePipeline: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated pipelineResponse
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("UpdatePipeline: decode response: %v", err)
	}
	if updated.Name != "Autonomous Delivery v2" {
		t.Fatalf("UpdatePipeline: expected name %q, got %q", "Autonomous Delivery v2", updated.Name)
	}
	if len(updated.Stages) != 3 {
		t.Fatalf("UpdatePipeline: expected 3 stages, got %d", len(updated.Stages))
	}
	if updated.Stages[2].Name != "Deploy" || updated.Stages[2].Position != 3 {
		t.Fatalf("UpdatePipeline: expected deploy stage appended at position 3, got %+v", updated.Stages[2])
	}
}

func TestPipelineUpdateAllowsMetadataOnlyChanges(t *testing.T) {
	buildAgentID := createHandlerTestAgent(t, "Pipeline Metadata Build Agent", nil)
	reviewAgentID := createHandlerTestAgent(t, "Pipeline Metadata Review Agent", nil)

	pipelineID, _ := insertPipelineFixture(t, "Pipeline metadata only", []pipelineStageRequest{
		{Name: "Build", Status: "done", AgentID: buildAgentID, Position: 1},
		{Name: "Review", Status: "in_review", AgentID: reviewAgentID, Position: 2},
	})

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/pipelines/"+pipelineID, map[string]any{
		"name":        "Pipeline metadata only renamed",
		"description": "Metadata-only update should preserve stages",
	})
	req = withURLParam(req, "id", pipelineID)
	callUpdatePipeline(t, w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdatePipeline metadata-only: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated pipelineResponse
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("UpdatePipeline metadata-only: decode response: %v", err)
	}
	if updated.Name != "Pipeline metadata only renamed" {
		t.Fatalf("UpdatePipeline metadata-only: expected renamed pipeline, got %q", updated.Name)
	}
	if len(updated.Stages) != 2 {
		t.Fatalf("UpdatePipeline metadata-only: expected existing 2 stages to be preserved, got %d", len(updated.Stages))
	}
	if updated.Stages[0].Position != 1 || updated.Stages[0].Name != "Build" {
		t.Fatalf("UpdatePipeline metadata-only: expected first stage preserved, got %+v", updated.Stages[0])
	}
	if updated.Stages[1].Position != 2 || updated.Stages[1].Name != "Review" {
		t.Fatalf("UpdatePipeline metadata-only: expected second stage preserved, got %+v", updated.Stages[1])
	}
}

func TestPipelineDelete(t *testing.T) {
	buildAgentID := createHandlerTestAgent(t, "Pipeline Delete Build Agent", nil)
	reviewAgentID := createHandlerTestAgent(t, "Pipeline Delete Review Agent", nil)

	pipelineID, _ := insertPipelineFixture(t, "Pipeline to delete", []pipelineStageRequest{
		{Name: "Build", Status: "done", AgentID: buildAgentID, Position: 1},
		{Name: "Review", Status: "in_review", AgentID: reviewAgentID, Position: 2},
	})

	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/pipelines/"+pipelineID, nil)
	req = withURLParam(req, "id", pipelineID)
	callDeletePipeline(t, w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DeletePipeline: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPipelineRejectsDuplicateStagePositions(t *testing.T) {
	buildAgentID := createHandlerTestAgent(t, "Duplicate Position Build Agent", nil)
	reviewAgentID := createHandlerTestAgent(t, "Duplicate Position Review Agent", nil)

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/pipelines?workspace_id="+testWorkspaceID, map[string]any{
		"name": "Invalid duplicate positions",
		"stages": []pipelineStageRequest{
			{Name: "Build", Status: "done", AgentID: buildAgentID, Position: 1},
			{Name: "Review", Status: "in_review", AgentID: reviewAgentID, Position: 1},
		},
	})
	callCreatePipeline(t, w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CreatePipeline duplicate positions: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "position") {
		t.Fatalf("CreatePipeline duplicate positions: expected error to mention position, got %q", w.Body.String())
	}
}

func TestPipelineProjectControlFallback(t *testing.T) {
	builderAgentID := createHandlerTestAgent(t, "Fallback Builder Agent", nil)
	reviewerAgentID := createHandlerTestAgent(t, "Fallback Reviewer Agent", nil)
	project := createPipelineTestProject(t, "Fallback Pipeline Project")

	pipelineID, _ := insertPipelineFixture(t, "Project default pipeline", []pipelineStageRequest{
		{Name: "Build", Status: "done", AgentID: builderAgentID, Position: 1},
		{Name: "Review", Status: "in_review", AgentID: reviewerAgentID, Position: 2},
	})
	insertProjectControlSettingsFixture(t, project.ID, pipelineID)

	issue := createPipelineTestIssue(t, map[string]any{
		"title":         "Issue should inherit project default pipeline",
		"status":        "in_progress",
		"assignee_type": "agent",
		"assignee_id":   builderAgentID,
		"project_id":    project.ID,
	})

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/issues/"+issue.ID, map[string]any{
		"status": "done",
	})
	req = withURLParam(req, "id", issue.ID)
	req.Header.Set("X-Agent-ID", builderAgentID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateIssue fallback pipeline handoff: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("UpdateIssue fallback pipeline handoff: decode response: %v", err)
	}
	if updated.Status != "done" {
		t.Fatalf("UpdateIssue fallback pipeline handoff: expected issue status to remain %q, got %q", "done", updated.Status)
	}

	if count := queuedTaskCountForIssueAndAgent(t, issue.ID, reviewerAgentID); count != 1 {
		t.Fatalf("UpdateIssue fallback pipeline handoff: expected exactly 1 queued reviewer task from project default pipeline, got %d", count)
	}
}

func TestStageRoutingQueuesNextStageAgentOnce(t *testing.T) {
	builderAgentID := createHandlerTestAgent(t, "Stage Routing Builder Agent", nil)
	reviewerAgentID := createHandlerTestAgent(t, "Stage Routing Reviewer Agent", nil)

	pipelineID, stageIDs := insertPipelineFixture(t, "Issue attached pipeline", []pipelineStageRequest{
		{Name: "Build", Status: "done", AgentID: builderAgentID, Position: 1},
		{Name: "Review", Status: "in_review", AgentID: reviewerAgentID, Position: 2},
	})

	issue := createPipelineTestIssue(t, map[string]any{
		"title":         "Issue should hand off to next pipeline stage",
		"status":        "in_progress",
		"assignee_type": "agent",
		"assignee_id":   builderAgentID,
	})
	insertIssuePipelineFixture(t, issue.ID, pipelineID, stageIDs[0], 0)

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/issues/"+issue.ID, map[string]any{
		"status": "done",
	})
	req = withURLParam(req, "id", issue.ID)
	req.Header.Set("X-Agent-ID", builderAgentID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateIssue stage routing handoff: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("UpdateIssue stage routing handoff: decode response: %v", err)
	}
	if updated.Status != "done" {
		t.Fatalf("UpdateIssue stage routing handoff: expected issue status to remain %q, got %q", "done", updated.Status)
	}
	if count := queuedTaskCountForIssueAndAgent(t, issue.ID, reviewerAgentID); count != 1 {
		t.Fatalf("UpdateIssue stage routing handoff: expected exactly 1 queued reviewer task after first completion, got %d", count)
	}

	w = httptest.NewRecorder()
	req = newRequest("PUT", "/api/issues/"+issue.ID, map[string]any{
		"status": "done",
	})
	req = withURLParam(req, "id", issue.ID)
	req.Header.Set("X-Agent-ID", builderAgentID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateIssue repeated completion: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if count := queuedTaskCountForIssueAndAgent(t, issue.ID, reviewerAgentID); count != 1 {
		t.Fatalf("UpdateIssue repeated completion: expected duplicate enqueue protection to keep reviewer queue count at 1, got %d", count)
	}
}

func TestStageRoutingDoesNotAdvanceWithoutAuthoritativeStatusChange(t *testing.T) {
	builderAgentID := createHandlerTestAgent(t, "Stage Routing No-Change Builder Agent", nil)
	reviewerAgentID := createHandlerTestAgent(t, "Stage Routing No-Change Reviewer Agent", nil)

	pipelineID, stageIDs := insertPipelineFixture(t, "Issue attached pipeline no change", []pipelineStageRequest{
		{Name: "Build", Status: "done", AgentID: builderAgentID, Position: 1},
		{Name: "Review", Status: "in_review", AgentID: reviewerAgentID, Position: 2},
	})

	issue := createPipelineTestIssue(t, map[string]any{
		"title":         "Issue should not hand off without authoritative status change",
		"status":        "in_progress",
		"assignee_type": "agent",
		"assignee_id":   builderAgentID,
	})
	insertIssuePipelineFixture(t, issue.ID, pipelineID, stageIDs[0], 0)

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/issues/"+issue.ID, map[string]any{
		"priority": "urgent",
	})
	req = withURLParam(req, "id", issue.ID)
	req.Header.Set("X-Agent-ID", builderAgentID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateIssue without status change: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if count := queuedTaskCountForIssueAndAgent(t, issue.ID, reviewerAgentID); count != 0 {
		t.Fatalf("UpdateIssue without status change: expected no queued reviewer task, got %d", count)
	}
}

func TestStageRoutingDoesNotAdvanceWhenStatusDoesNotCompleteCurrentStage(t *testing.T) {
	builderAgentID := createHandlerTestAgent(t, "Stage Routing Incomplete Builder Agent", nil)
	reviewerAgentID := createHandlerTestAgent(t, "Stage Routing Incomplete Reviewer Agent", nil)

	pipelineID, stageIDs := insertPipelineFixture(t, "Issue attached pipeline incomplete", []pipelineStageRequest{
		{Name: "Build", Status: "done", AgentID: builderAgentID, Position: 1},
		{Name: "Review", Status: "in_review", AgentID: reviewerAgentID, Position: 2},
	})

	issue := createPipelineTestIssue(t, map[string]any{
		"title":         "Issue should not hand off when stage is not complete",
		"status":        "in_progress",
		"assignee_type": "agent",
		"assignee_id":   builderAgentID,
	})
	insertIssuePipelineFixture(t, issue.ID, pipelineID, stageIDs[0], 0)

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/issues/"+issue.ID, map[string]any{
		"status": "blocked",
	})
	req = withURLParam(req, "id", issue.ID)
	req.Header.Set("X-Agent-ID", builderAgentID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateIssue with incomplete stage status: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("UpdateIssue with incomplete stage status: decode response: %v", err)
	}
	if updated.Status != "blocked" {
		t.Fatalf("UpdateIssue with incomplete stage status: expected issue status to remain %q, got %q", "blocked", updated.Status)
	}
	if count := queuedTaskCountForIssueAndAgent(t, issue.ID, reviewerAgentID); count != 0 {
		t.Fatalf("UpdateIssue with incomplete stage status: expected no queued reviewer task, got %d", count)
	}
}
