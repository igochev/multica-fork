package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestPipelineResolveEffectivePipelineForIssueUsesExplicitIssuePipeline(t *testing.T) {
	issueID := testUUID(1)
	workspaceID := testUUID(2)
	pipelineID := testUUID(3)
	currentStageID := testUUID(4)
	nextStageID := testUUID(5)

	queries := &fakePipelineQueries{
		issuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: currentStageID,
			StageSequence:  7,
		},
		pipeline: db.Pipeline{
			ID:          pipelineID,
			WorkspaceID: workspaceID,
			Name:        "Delivery",
		},
		stages: []db.PipelineStage{
			{ID: currentStageID, PipelineID: pipelineID, Name: "Triage", Position: 1},
			{ID: nextStageID, PipelineID: pipelineID, Name: "Build", Position: 2},
		},
	}

	svc := &PipelineService{queries: queries, taskSvc: &fakeTaskEnqueuer{}}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID}

	resolved, err := svc.ResolveEffectivePipelineForIssue(context.Background(), issue)
	if err != nil {
		t.Fatalf("ResolveEffectivePipelineForIssue returned error: %v", err)
	}
	if resolved == nil {
		t.Fatalf("expected resolved pipeline, got nil")
	}
	if resolved.Pipeline.ID != pipelineID {
		t.Fatalf("expected pipeline %v, got %v", pipelineID, resolved.Pipeline.ID)
	}
	if resolved.CurrentStage.ID != currentStageID {
		t.Fatalf("expected current stage %v, got %v", currentStageID, resolved.CurrentStage.ID)
	}
	if len(resolved.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(resolved.Stages))
	}
}

func TestPipelineResolveEffectivePipelineForIssueUsesProjectDefaultPipelineFallback(t *testing.T) {
	issueID := testUUID(6)
	workspaceID := testUUID(7)
	projectID := testUUID(8)
	pipelineID := testUUID(9)
	currentStageID := testUUID(10)
	nextStageID := testUUID(11)

	queries := &fakePipelineQueries{
		issuePipelineErr: pgx.ErrNoRows,
		projectControl: db.GetProjectControlSettingsRow{
			ProjectID:         projectID,
			DefaultPipelineID: pipelineID,
		},
		pipeline: db.Pipeline{
			ID:          pipelineID,
			WorkspaceID: workspaceID,
			Name:        "Project Default Pipeline",
		},
		stages: []db.PipelineStage{
			{ID: nextStageID, PipelineID: pipelineID, Name: "Review", Position: 2},
			{ID: currentStageID, PipelineID: pipelineID, Name: "Build", Position: 1},
		},
		createdIssuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: currentStageID,
			StageSequence:  0,
		},
	}
	svc := &PipelineService{queries: queries, taskSvc: &fakeTaskEnqueuer{}}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID, ProjectID: projectID}

	resolved, err := svc.ResolveEffectivePipelineForIssue(context.Background(), issue)
	if err != nil {
		t.Fatalf("ResolveEffectivePipelineForIssue returned error: %v", err)
	}
	if resolved == nil {
		t.Fatalf("expected resolved pipeline, got nil")
	}
	if !queries.createIssuePipelineCalled {
		t.Fatalf("expected CreateIssuePipeline to be called for project default fallback")
	}
	if queries.createIssuePipelineParams.IssueID != issueID {
		t.Fatalf("expected fallback issue pipeline issue %v, got %v", issueID, queries.createIssuePipelineParams.IssueID)
	}
	if queries.createIssuePipelineParams.PipelineID != pipelineID {
		t.Fatalf("expected fallback pipeline %v, got %v", pipelineID, queries.createIssuePipelineParams.PipelineID)
	}
	if queries.createIssuePipelineParams.CurrentStageID != currentStageID {
		t.Fatalf("expected fallback current stage %v, got %v", currentStageID, queries.createIssuePipelineParams.CurrentStageID)
	}
	if resolved.CurrentStage.ID != currentStageID {
		t.Fatalf("expected current stage %v, got %v", currentStageID, resolved.CurrentStage.ID)
	}
}

func TestPipelineMaybeAdvanceIssuePipelineEnqueuesNextStageAgent(t *testing.T) {
	issueID := testUUID(10)
	workspaceID := testUUID(11)
	pipelineID := testUUID(12)
	currentStageID := testUUID(13)
	nextStageID := testUUID(14)
	nextAgentID := testUUID(15)
	queuedTaskID := testUUID(16)

	queries := &fakePipelineQueries{
		issuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: currentStageID,
			StageSequence:  3,
		},
		pipeline: db.Pipeline{ID: pipelineID, WorkspaceID: workspaceID, Name: "Delivery"},
		stages: []db.PipelineStage{
			{ID: currentStageID, PipelineID: pipelineID, Name: "Triage", Position: 1},
			{ID: nextStageID, PipelineID: pipelineID, Name: "Build", AgentID: nextAgentID, Position: 2},
		},
		nextStage: db.PipelineStage{ID: nextStageID, PipelineID: pipelineID, Name: "Build", AgentID: nextAgentID, Position: 2},
		advancedIssuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: nextStageID,
			StageSequence:  4,
		},
	}
	enqueuer := &fakeTaskEnqueuer{task: db.AgentTaskQueue{ID: queuedTaskID, AgentID: nextAgentID, IssueID: issueID}}
	svc := &PipelineService{queries: queries, taskSvc: enqueuer}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID, Priority: "high", AssigneeID: testUUID(99)}

	result, err := svc.MaybeAdvanceIssuePipeline(context.Background(), issue)
	if err != nil {
		t.Fatalf("MaybeAdvanceIssuePipeline returned error: %v", err)
	}
	if result == nil || !result.Advanced {
		t.Fatalf("expected advanced result, got %#v", result)
	}
	if queries.advanceParams.NextStageID != nextStageID {
		t.Fatalf("expected next stage %v, got %v", nextStageID, queries.advanceParams.NextStageID)
	}
	if queries.advanceParams.CurrentStageID != currentStageID {
		t.Fatalf("expected current stage %v, got %v", currentStageID, queries.advanceParams.CurrentStageID)
	}
	if queries.advanceParams.ExpectedStageSequence != 3 {
		t.Fatalf("expected stage sequence 3, got %d", queries.advanceParams.ExpectedStageSequence)
	}
	if enqueuer.agentID != nextAgentID {
		t.Fatalf("expected enqueue for agent %v, got %v", nextAgentID, enqueuer.agentID)
	}
	if result.EnqueuedTask == nil || result.EnqueuedTask.ID != queuedTaskID {
		t.Fatalf("expected enqueued task %v, got %#v", queuedTaskID, result.EnqueuedTask)
	}
}

func TestPipelineMaybeAdvanceIssuePipelineDoesNotAdvanceWhenCurrentStageIsNotComplete(t *testing.T) {
	issueID := testUUID(20)
	workspaceID := testUUID(21)
	pipelineID := testUUID(22)
	currentStageID := testUUID(23)
	nextStageID := testUUID(24)
	nextAgentID := testUUID(25)

	queries := &fakePipelineQueries{
		issuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: currentStageID,
			StageSequence:  1,
		},
		pipeline: db.Pipeline{ID: pipelineID, WorkspaceID: workspaceID},
		stages: []db.PipelineStage{
			{ID: currentStageID, PipelineID: pipelineID, Status: "done", Position: 1},
			{ID: nextStageID, PipelineID: pipelineID, Status: "in_review", AgentID: nextAgentID, Position: 2},
		},
	}
	enqueuer := &fakeTaskEnqueuer{}
	svc := &PipelineService{queries: queries, taskSvc: enqueuer}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID, Status: "blocked"}

	result, err := svc.MaybeAdvanceIssuePipeline(context.Background(), issue)
	if err != nil {
		t.Fatalf("MaybeAdvanceIssuePipeline returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected result, got nil")
	}
	if result.Advanced {
		t.Fatalf("expected not advanced result, got %#v", result)
	}
	if len(queries.advanceCalls) != 0 {
		t.Fatalf("expected no advance calls, got %d", len(queries.advanceCalls))
	}
	if enqueuer.called {
		t.Fatalf("expected no task enqueue when current stage is not complete")
	}
}

func TestPipelineMaybeAdvanceIssuePipelineStopsAtFinalStage(t *testing.T) {
	issueID := testUUID(26)
	workspaceID := testUUID(27)
	pipelineID := testUUID(28)
	currentStageID := testUUID(29)

	queries := &fakePipelineQueries{
		issuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: currentStageID,
			StageSequence:  1,
		},
		pipeline:     db.Pipeline{ID: pipelineID, WorkspaceID: workspaceID},
		stages:       []db.PipelineStage{{ID: currentStageID, PipelineID: pipelineID, Status: "done", Position: 1}},
		nextStageErr: pgx.ErrNoRows,
	}
	enqueuer := &fakeTaskEnqueuer{}
	svc := &PipelineService{queries: queries, taskSvc: enqueuer}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID, Status: "done"}

	result, err := svc.MaybeAdvanceIssuePipeline(context.Background(), issue)
	if err != nil {
		t.Fatalf("MaybeAdvanceIssuePipeline returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected result, got nil")
	}
	if result.Advanced {
		t.Fatalf("expected not advanced result, got %#v", result)
	}
	if enqueuer.called {
		t.Fatalf("expected no task enqueue on final stage")
	}
}

func TestPipelineMaybeAdvanceIssuePipelineRollsBackAdvanceWhenEnqueueFails(t *testing.T) {
	issueID := testUUID(40)
	workspaceID := testUUID(41)
	pipelineID := testUUID(42)
	currentStageID := testUUID(43)
	nextStageID := testUUID(44)
	nextAgentID := testUUID(45)

	queries := &fakePipelineQueries{
		issuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: currentStageID,
			StageSequence:  2,
		},
		pipeline: db.Pipeline{ID: pipelineID, WorkspaceID: workspaceID, Name: "Delivery"},
		stages: []db.PipelineStage{
			{ID: currentStageID, PipelineID: pipelineID, Name: "Triage", Position: 1},
			{ID: nextStageID, PipelineID: pipelineID, Name: "Build", AgentID: nextAgentID, Position: 2},
		},
		nextStage: db.PipelineStage{ID: nextStageID, PipelineID: pipelineID, Name: "Build", AgentID: nextAgentID, Position: 2},
		advanceResults: []db.IssuePipeline{
			{IssueID: issueID, PipelineID: pipelineID, CurrentStageID: nextStageID, StageSequence: 3},
			{IssueID: issueID, PipelineID: pipelineID, CurrentStageID: currentStageID, StageSequence: 4},
		},
	}
	enqueuer := &fakeTaskEnqueuer{err: errors.New("boom")}
	svc := &PipelineService{queries: queries, taskSvc: enqueuer}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID, Priority: "medium"}

	result, err := svc.MaybeAdvanceIssuePipeline(context.Background(), issue)
	if err == nil {
		t.Fatalf("expected enqueue error, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result on enqueue failure, got %#v", result)
	}
	if len(queries.advanceCalls) != 2 {
		t.Fatalf("expected advance and rollback calls, got %d", len(queries.advanceCalls))
	}
	rollback := queries.advanceCalls[1]
	if rollback.NextStageID != currentStageID {
		t.Fatalf("expected rollback to previous stage %v, got %v", currentStageID, rollback.NextStageID)
	}
	if rollback.CurrentStageID != nextStageID {
		t.Fatalf("expected rollback from advanced stage %v, got %v", nextStageID, rollback.CurrentStageID)
	}
	if rollback.ExpectedStageSequence != 3 {
		t.Fatalf("expected rollback stage sequence 3, got %d", rollback.ExpectedStageSequence)
	}
}

func TestPipelineMaybeAdvanceIssuePipelineRollsBackAdvanceWithCanceledContext(t *testing.T) {
	issueID := testUUID(46)
	workspaceID := testUUID(47)
	pipelineID := testUUID(48)
	currentStageID := testUUID(49)
	nextStageID := testUUID(60)
	nextAgentID := testUUID(61)

	queries := &fakePipelineQueries{
		issuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: currentStageID,
			StageSequence:  5,
		},
		pipeline: db.Pipeline{ID: pipelineID, WorkspaceID: workspaceID, Name: "Delivery"},
		stages: []db.PipelineStage{
			{ID: currentStageID, PipelineID: pipelineID, Name: "Triage", Position: 1},
			{ID: nextStageID, PipelineID: pipelineID, Name: "Build", AgentID: nextAgentID, Position: 2},
		},
		nextStage: db.PipelineStage{ID: nextStageID, PipelineID: pipelineID, Name: "Build", AgentID: nextAgentID, Position: 2},
		advanceResults: []db.IssuePipeline{
			{IssueID: issueID, PipelineID: pipelineID, CurrentStageID: nextStageID, StageSequence: 6},
			{IssueID: issueID, PipelineID: pipelineID, CurrentStageID: currentStageID, StageSequence: 7},
		},
		rejectCanceledAdvanceCtx: true,
	}
	canceledCtx, cancel := context.WithCancel(context.Background())
	enqueuer := &fakeTaskEnqueuer{
		enqueueFunc: func(_ context.Context, _ db.Issue, _ pgtype.UUID, _ ...pgtype.UUID) (db.AgentTaskQueue, error) {
			cancel()
			return db.AgentTaskQueue{}, context.Canceled
		},
	}
	svc := &PipelineService{queries: queries, taskSvc: enqueuer}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID}

	result, err := svc.MaybeAdvanceIssuePipeline(canceledCtx, issue)
	if err == nil {
		t.Fatalf("expected enqueue error, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result on enqueue failure, got %#v", result)
	}
	if len(queries.advanceCalls) != 2 {
		t.Fatalf("expected advance and rollback calls, got %d", len(queries.advanceCalls))
	}
	if len(queries.advanceCtxErrs) != 2 {
		t.Fatalf("expected ctx observations for advance and rollback, got %d", len(queries.advanceCtxErrs))
	}
	if queries.advanceCtxErrs[1] != nil {
		t.Fatalf("expected rollback context to be usable, got %v", queries.advanceCtxErrs[1])
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestPipelineMaybeAdvanceIssuePipelineHandlesConcurrentAdvanceNoRows(t *testing.T) {
	issueID := testUUID(50)
	workspaceID := testUUID(51)
	pipelineID := testUUID(52)
	currentStageID := testUUID(53)
	nextStageID := testUUID(54)
	nextAgentID := testUUID(55)

	queries := &fakePipelineQueries{
		issuePipeline: db.IssuePipeline{
			IssueID:        issueID,
			PipelineID:     pipelineID,
			CurrentStageID: currentStageID,
			StageSequence:  9,
		},
		pipeline: db.Pipeline{ID: pipelineID, WorkspaceID: workspaceID, Name: "Delivery"},
		stages: []db.PipelineStage{
			{ID: currentStageID, PipelineID: pipelineID, Name: "Triage", Position: 1},
			{ID: nextStageID, PipelineID: pipelineID, Name: "Build", AgentID: nextAgentID, Position: 2},
		},
		nextStage:     db.PipelineStage{ID: nextStageID, PipelineID: pipelineID, Name: "Build", AgentID: nextAgentID, Position: 2},
		advanceErrors: []error{pgx.ErrNoRows},
	}
	enqueuer := &fakeTaskEnqueuer{}
	svc := &PipelineService{queries: queries, taskSvc: enqueuer}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID}

	result, err := svc.MaybeAdvanceIssuePipeline(context.Background(), issue)
	if err != nil {
		t.Fatalf("MaybeAdvanceIssuePipeline returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected result, got nil")
	}
	if result.Advanced {
		t.Fatalf("expected no advance on concurrent no-row update, got %#v", result)
	}
	if result.CurrentStage == nil || result.CurrentStage.ID != currentStageID {
		t.Fatalf("expected current stage to remain %v, got %#v", currentStageID, result.CurrentStage)
	}
	if enqueuer.called {
		t.Fatalf("expected no task enqueue when advance matched no rows")
	}
}

func TestPipelineEnqueueTaskForAgentIssueUsesExplicitAgent(t *testing.T) {
	issueID := testUUID(30)
	explicitAgentID := testUUID(31)
	runtimeID := testUUID(32)
	triggerCommentID := testUUID(33)

	queries := &fakeTaskQueries{
		agent: db.Agent{ID: explicitAgentID, RuntimeID: runtimeID},
		task:  db.AgentTaskQueue{ID: testUUID(34), AgentID: explicitAgentID, RuntimeID: runtimeID, IssueID: issueID},
	}
	issue := db.Issue{
		ID:         issueID,
		Priority:   "high",
		AssigneeID: testUUID(35),
	}

	task, err := enqueueTaskForAgentIssue(context.Background(), queries, issue, explicitAgentID, triggerCommentID)
	if err != nil {
		t.Fatalf("enqueueTaskForAgentIssue returned error: %v", err)
	}
	if task.AgentID != explicitAgentID {
		t.Fatalf("expected queued task for agent %v, got %v", explicitAgentID, task.AgentID)
	}
	if queries.createParams.AgentID != explicitAgentID {
		t.Fatalf("expected create params agent %v, got %v", explicitAgentID, queries.createParams.AgentID)
	}
	if queries.createParams.IssueID != issueID {
		t.Fatalf("expected issue %v, got %v", issueID, queries.createParams.IssueID)
	}
	if queries.createParams.RuntimeID != runtimeID {
		t.Fatalf("expected runtime %v, got %v", runtimeID, queries.createParams.RuntimeID)
	}
	if queries.createParams.Priority != 3 {
		t.Fatalf("expected high priority to map to 3, got %d", queries.createParams.Priority)
	}
	if queries.createParams.TriggerCommentID != triggerCommentID {
		t.Fatalf("expected trigger comment %v, got %v", triggerCommentID, queries.createParams.TriggerCommentID)
	}
}

type fakePipelineQueries struct {
	issuePipeline             db.IssuePipeline
	issuePipelineErr          error
	projectControl            db.GetProjectControlSettingsRow
	projectControlErr         error
	createdIssuePipeline      db.IssuePipeline
	createIssuePipelineErr    error
	createIssuePipelineParams db.CreateIssuePipelineParams
	createIssuePipelineCalled bool
	pipeline                  db.Pipeline
	pipelineErr               error
	stages                    []db.PipelineStage
	stagesErr                 error
	nextStage                 db.PipelineStage
	nextStageErr              error
	advancedIssuePipeline     db.IssuePipeline
	advanceErr                error
	advanceParams             db.AdvanceIssuePipelineStageParams
	advanceCalls              []db.AdvanceIssuePipelineStageParams
	advanceCtxErrs            []error
	advanceResults            []db.IssuePipeline
	advanceErrors             []error
	rejectCanceledAdvanceCtx  bool
}

func (f *fakePipelineQueries) GetIssuePipeline(_ context.Context, _ pgtype.UUID) (db.IssuePipeline, error) {
	if f.issuePipelineErr != nil {
		return db.IssuePipeline{}, f.issuePipelineErr
	}
	return f.issuePipeline, nil
}

func (f *fakePipelineQueries) CreateIssuePipeline(_ context.Context, arg db.CreateIssuePipelineParams) (db.IssuePipeline, error) {
	f.createIssuePipelineCalled = true
	f.createIssuePipelineParams = arg
	if f.createIssuePipelineErr != nil {
		return db.IssuePipeline{}, f.createIssuePipelineErr
	}
	return f.createdIssuePipeline, nil
}

func (f *fakePipelineQueries) GetProjectControlSettings(_ context.Context, _ pgtype.UUID) (db.GetProjectControlSettingsRow, error) {
	if f.projectControlErr != nil {
		return db.GetProjectControlSettingsRow{}, f.projectControlErr
	}
	return f.projectControl, nil
}

func (f *fakePipelineQueries) GetPipelineInWorkspace(_ context.Context, _ db.GetPipelineInWorkspaceParams) (db.Pipeline, error) {
	if f.pipelineErr != nil {
		return db.Pipeline{}, f.pipelineErr
	}
	return f.pipeline, nil
}

func (f *fakePipelineQueries) ListPipelineStages(_ context.Context, _ pgtype.UUID) ([]db.PipelineStage, error) {
	if f.stagesErr != nil {
		return nil, f.stagesErr
	}
	return f.stages, nil
}

func (f *fakePipelineQueries) GetNextPipelineStageByPosition(_ context.Context, _ db.GetNextPipelineStageByPositionParams) (db.PipelineStage, error) {
	if f.nextStageErr != nil {
		return db.PipelineStage{}, f.nextStageErr
	}
	return f.nextStage, nil
}

func (f *fakePipelineQueries) AdvanceIssuePipelineStage(ctx context.Context, arg db.AdvanceIssuePipelineStageParams) (db.IssuePipeline, error) {
	f.advanceParams = arg
	f.advanceCalls = append(f.advanceCalls, arg)
	f.advanceCtxErrs = append(f.advanceCtxErrs, ctx.Err())
	if f.rejectCanceledAdvanceCtx && ctx.Err() != nil {
		return db.IssuePipeline{}, ctx.Err()
	}
	if len(f.advanceErrors) > 0 {
		err := f.advanceErrors[0]
		f.advanceErrors = f.advanceErrors[1:]
		if err != nil {
			return db.IssuePipeline{}, err
		}
	}
	if len(f.advanceResults) > 0 {
		result := f.advanceResults[0]
		f.advanceResults = f.advanceResults[1:]
		return result, nil
	}
	if f.advanceErr != nil {
		return db.IssuePipeline{}, f.advanceErr
	}
	return f.advancedIssuePipeline, nil
}

type fakeTaskEnqueuer struct {
	called      bool
	agentID     pgtype.UUID
	issue       db.Issue
	task        db.AgentTaskQueue
	err         error
	enqueueFunc func(context.Context, db.Issue, pgtype.UUID, ...pgtype.UUID) (db.AgentTaskQueue, error)
}

func (f *fakeTaskEnqueuer) EnqueueTaskForAgentIssue(ctx context.Context, issue db.Issue, agentID pgtype.UUID, triggerCommentID ...pgtype.UUID) (db.AgentTaskQueue, error) {
	f.called = true
	f.issue = issue
	f.agentID = agentID
	if f.enqueueFunc != nil {
		return f.enqueueFunc(ctx, issue, agentID, triggerCommentID...)
	}
	if f.err != nil {
		return db.AgentTaskQueue{}, f.err
	}
	return f.task, nil
}

type fakeTaskQueries struct {
	agent         db.Agent
	agentErr      error
	task          db.AgentTaskQueue
	createTaskErr error
	createParams  db.CreateAgentTaskParams
	createCalled  bool
}

func (f *fakeTaskQueries) GetAgent(_ context.Context, _ pgtype.UUID) (db.Agent, error) {
	if f.agentErr != nil {
		return db.Agent{}, f.agentErr
	}
	return f.agent, nil
}

func (f *fakeTaskQueries) CreateAgentTask(_ context.Context, arg db.CreateAgentTaskParams) (db.AgentTaskQueue, error) {
	f.createCalled = true
	f.createParams = arg
	if f.createTaskErr != nil {
		return db.AgentTaskQueue{}, f.createTaskErr
	}
	return f.task, nil
}

func testUUID(last byte) pgtype.UUID {
	var value [16]byte
	value[15] = last
	return pgtype.UUID{Bytes: value, Valid: true}
}
