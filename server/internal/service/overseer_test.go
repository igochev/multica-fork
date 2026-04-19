package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestOverseerEscalatesBlockedIssue(t *testing.T) {
	issueID := testUUID(1)
	workspaceID := testUUID(2)
	projectID := testUUID(3)
	overseerAgentID := testUUID(4)
	taskID := testUUID(5)

	queries := &fakeOverseerQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:           projectID,
			OverseerAgentID:     overseerAgentID,
			AutoEscalateBlocked: true,
		},
		activity: db.ActivityLog{ID: testUUID(6), IssueID: issueID},
	}
	taskSvc := &fakeOverseerTaskService{
		task: db.AgentTaskQueue{ID: taskID, IssueID: issueID, AgentID: overseerAgentID, Status: "queued"},
	}
	svc := &OverseerService{queries: queries, taskSvc: taskSvc}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID, ProjectID: projectID, Status: "blocked", Priority: "high"}

	result, err := svc.MaybeEscalateBlockedIssue(context.Background(), issue)
	if err != nil {
		t.Fatalf("MaybeEscalateBlockedIssue returned error: %v", err)
	}
	if result == nil || !result.Requested {
		t.Fatalf("expected requested result, got %#v", result)
	}
	if result.EnqueuedTask == nil || result.EnqueuedTask.ID != taskID {
		t.Fatalf("expected enqueued task %v, got %#v", taskID, result.EnqueuedTask)
	}
	if queries.settingsProjectID != projectID {
		t.Fatalf("expected project control lookup for %v, got %v", projectID, queries.settingsProjectID)
	}
	if queries.listTasksIssueID != issueID {
		t.Fatalf("expected task lookup for issue %v, got %v", issueID, queries.listTasksIssueID)
	}
	if !taskSvc.enqueueCalled {
		t.Fatalf("expected overseer task enqueue")
	}
	if taskSvc.agentID != overseerAgentID {
		t.Fatalf("expected enqueue for overseer %v, got %v", overseerAgentID, taskSvc.agentID)
	}
	if !queries.createActivityCalled {
		t.Fatalf("expected activity log entry to be created")
	}
	if queries.createActivityParams.Action != "overseer_requested_for_blocked_issue" {
		t.Fatalf("expected activity action overseer_requested_for_blocked_issue, got %q", queries.createActivityParams.Action)
	}
	if queries.createActivityParams.ActorType.String != "system" {
		t.Fatalf("expected system actor type, got %q", queries.createActivityParams.ActorType.String)
	}
	if queries.createActivityParams.ActorID.Valid {
		t.Fatalf("expected system activity actor_id to be null")
	}

	var details map[string]any
	if err := json.Unmarshal(queries.createActivityParams.Details, &details); err != nil {
		t.Fatalf("expected JSON activity details, got error: %v", err)
	}
	if details["reason"] != "blocked_issue" {
		t.Fatalf("expected blocked_issue reason, got %#v", details["reason"])
	}
}

func TestOverseerDoesNotEnqueueWithoutConfiguredOverseer(t *testing.T) {
	issueID := testUUID(10)
	projectID := testUUID(11)

	queries := &fakeOverseerQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:           projectID,
			AutoEscalateBlocked: true,
		},
	}
	taskSvc := &fakeOverseerTaskService{}
	svc := &OverseerService{queries: queries, taskSvc: taskSvc}
	issue := db.Issue{ID: issueID, ProjectID: projectID, Status: "blocked"}

	result, err := svc.MaybeEscalateBlockedIssue(context.Background(), issue)
	if err != nil {
		t.Fatalf("MaybeEscalateBlockedIssue returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected result, got nil")
	}
	if result.Requested {
		t.Fatalf("expected no overseer escalation, got %#v", result)
	}
	if taskSvc.enqueueCalled {
		t.Fatalf("expected no task enqueue without configured overseer")
	}
	if queries.createActivityCalled {
		t.Fatalf("expected no activity log without queued overseer task")
	}
}

func TestOverseerDoesNotDuplicateExistingInFlightTask(t *testing.T) {
	for _, status := range []string{"queued", "dispatched", "running"} {
		t.Run(status, func(t *testing.T) {
			issueID := testUUID(20)
			projectID := testUUID(21)
			overseerAgentID := testUUID(22)

			queries := &fakeOverseerQueries{
				settings: db.GetProjectControlSettingsRow{
					ProjectID:           projectID,
					OverseerAgentID:     overseerAgentID,
					AutoEscalateBlocked: true,
				},
				tasks: []db.AgentTaskQueue{{
					ID:      testUUID(23),
					IssueID: issueID,
					AgentID: overseerAgentID,
					Status:  status,
				}},
			}
			enqueuer := &fakeOverseerTaskService{}
			svc := &OverseerService{queries: queries, taskSvc: enqueuer}
			issue := db.Issue{ID: issueID, ProjectID: projectID, Status: "blocked"}

			result, err := svc.MaybeEscalateBlockedIssue(context.Background(), issue)
			if err != nil {
				t.Fatalf("MaybeEscalateBlockedIssue returned error: %v", err)
			}
			if result == nil {
				t.Fatalf("expected result, got nil")
			}
			if result.Requested {
				t.Fatalf("expected no duplicate overseer request, got %#v", result)
			}
			if enqueuer.enqueueCalled {
				t.Fatalf("expected no duplicate task enqueue when %s task exists", status)
			}
			if queries.createActivityCalled {
				t.Fatalf("expected no activity log when %s task exists", status)
			}
		})
	}
}

func TestOverseerRollsBackTaskWhenActivityLoggingFails(t *testing.T) {
	issueID := testUUID(30)
	workspaceID := testUUID(31)
	projectID := testUUID(32)
	overseerAgentID := testUUID(33)
	taskID := testUUID(34)

	queries := &fakeOverseerQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:           projectID,
			OverseerAgentID:     overseerAgentID,
			AutoEscalateBlocked: true,
		},
		activityErr: context.Canceled,
	}
	taskSvc := &fakeOverseerTaskService{
		task: db.AgentTaskQueue{ID: taskID, IssueID: issueID, AgentID: overseerAgentID, Status: "queued"},
	}
	svc := &OverseerService{queries: queries, taskSvc: taskSvc}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID, ProjectID: projectID, Status: "blocked"}

	result, err := svc.MaybeEscalateBlockedIssue(context.Background(), issue)
	if err == nil {
		t.Fatalf("expected error when activity logging fails, got result %#v", result)
	}
	if result != nil {
		t.Fatalf("expected nil result on activity failure, got %#v", result)
	}
	if !taskSvc.cancelCalled {
		t.Fatalf("expected overseer task rollback when activity logging fails")
	}
	if taskSvc.cancelTaskID != taskID {
		t.Fatalf("expected rollback for task %v, got %v", taskID, taskSvc.cancelTaskID)
	}
}

func TestOverseerEscalatesStaleIssue(t *testing.T) {
	issueID := testUUID(40)
	workspaceID := testUUID(41)
	projectID := testUUID(42)
	overseerAgentID := testUUID(43)
	workerAgentID := testUUID(44)
	taskID := testUUID(45)
	staleTaskID := testUUID(46)

	queries := &fakeOverseerQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:         projectID,
			OverseerAgentID:   overseerAgentID,
			StaleAfterMinutes: 60,
		},
		tasks: []db.AgentTaskQueue{{
			ID:        staleTaskID,
			IssueID:   issueID,
			AgentID:   workerAgentID,
			Status:    "running",
			CreatedAt: timestamptz(time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)),
		}},
		activity: db.ActivityLog{ID: testUUID(47), IssueID: issueID},
	}
	taskSvc := &fakeOverseerTaskService{
		task: db.AgentTaskQueue{ID: taskID, IssueID: issueID, AgentID: overseerAgentID, Status: "queued"},
	}
	svc := &OverseerService{queries: queries, taskSvc: taskSvc, now: func() time.Time {
		return time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	}}
	issue := db.Issue{ID: issueID, WorkspaceID: workspaceID, ProjectID: projectID, Status: "in_progress", Priority: "high"}

	result, err := svc.MaybeEscalateStaleIssue(context.Background(), issue)
	if err != nil {
		t.Fatalf("MaybeEscalateStaleIssue returned error: %v", err)
	}
	if result == nil || !result.Requested {
		t.Fatalf("expected requested result, got %#v", result)
	}
	if result.EnqueuedTask == nil || result.EnqueuedTask.ID != taskID {
		t.Fatalf("expected enqueued task %v, got %#v", taskID, result.EnqueuedTask)
	}
	if !taskSvc.enqueueCalled {
		t.Fatalf("expected overseer task enqueue")
	}
	if queries.createActivityParams.Action != "overseer_requested_for_stale_issue" {
		t.Fatalf("expected stale issue activity action, got %q", queries.createActivityParams.Action)
	}

	var details map[string]any
	if err := json.Unmarshal(queries.createActivityParams.Details, &details); err != nil {
		t.Fatalf("expected JSON activity details, got error: %v", err)
	}
	if details["reason"] != "stale_issue" {
		t.Fatalf("expected stale_issue reason, got %#v", details["reason"])
	}
}

func TestOverseerDoesNotEscalateFreshIssue(t *testing.T) {
	issueID := testUUID(50)
	projectID := testUUID(51)
	overseerAgentID := testUUID(52)
	workerAgentID := testUUID(53)

	queries := &fakeOverseerQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:         projectID,
			OverseerAgentID:   overseerAgentID,
			StaleAfterMinutes: 60,
		},
		tasks: []db.AgentTaskQueue{{
			ID:        testUUID(54),
			IssueID:   issueID,
			AgentID:   workerAgentID,
			Status:    "queued",
			CreatedAt: timestamptz(time.Date(2026, time.January, 1, 11, 30, 0, 0, time.UTC)),
		}},
	}
	taskSvc := &fakeOverseerTaskService{}
	svc := &OverseerService{queries: queries, taskSvc: taskSvc, now: func() time.Time {
		return time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	}}
	issue := db.Issue{ID: issueID, ProjectID: projectID, Status: "in_progress"}

	result, err := svc.MaybeEscalateStaleIssue(context.Background(), issue)
	if err != nil {
		t.Fatalf("MaybeEscalateStaleIssue returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected result, got nil")
	}
	if result.Requested {
		t.Fatalf("expected no stale escalation, got %#v", result)
	}
	if taskSvc.enqueueCalled {
		t.Fatalf("expected no task enqueue for fresh issue")
	}
	if queries.createActivityCalled {
		t.Fatalf("expected no activity log for fresh issue")
	}
}

type fakeOverseerQueries struct {
	settings             db.GetProjectControlSettingsRow
	settingsErr          error
	settingsProjectID    pgtype.UUID
	tasks                []db.AgentTaskQueue
	tasksErr             error
	listTasksIssueID     pgtype.UUID
	activity             db.ActivityLog
	activityErr          error
	createActivityCalled bool
	createActivityParams db.CreateActivityParams
}

func (f *fakeOverseerQueries) GetProjectControlSettings(_ context.Context, id pgtype.UUID) (db.GetProjectControlSettingsRow, error) {
	f.settingsProjectID = id
	if f.settingsErr != nil {
		return db.GetProjectControlSettingsRow{}, f.settingsErr
	}
	return f.settings, nil
}

func (f *fakeOverseerQueries) ListTasksByIssue(_ context.Context, issueID pgtype.UUID) ([]db.AgentTaskQueue, error) {
	f.listTasksIssueID = issueID
	if f.tasksErr != nil {
		return nil, f.tasksErr
	}
	return f.tasks, nil
}

func (f *fakeOverseerQueries) CreateActivity(_ context.Context, arg db.CreateActivityParams) (db.ActivityLog, error) {
	f.createActivityCalled = true
	f.createActivityParams = arg
	if f.activityErr != nil {
		return db.ActivityLog{}, f.activityErr
	}
	return f.activity, nil
}

type fakeOverseerTaskService struct {
	enqueueCalled bool
	cancelCalled  bool
	agentID       pgtype.UUID
	issue         db.Issue
	task          db.AgentTaskQueue
	err           error
	cancelTaskID  pgtype.UUID
	cancelledTask db.AgentTaskQueue
	cancelErr     error
}

func (f *fakeOverseerTaskService) EnqueueTaskForAgentIssue(_ context.Context, issue db.Issue, agentID pgtype.UUID, _ ...pgtype.UUID) (db.AgentTaskQueue, error) {
	f.enqueueCalled = true
	f.issue = issue
	f.agentID = agentID
	if f.err != nil {
		return db.AgentTaskQueue{}, f.err
	}
	return f.task, nil
}

func (f *fakeOverseerTaskService) CancelTask(_ context.Context, taskID pgtype.UUID) (*db.AgentTaskQueue, error) {
	f.cancelCalled = true
	f.cancelTaskID = taskID
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}
	if f.cancelledTask.ID.Valid {
		return &f.cancelledTask, nil
	}
	return &db.AgentTaskQueue{ID: taskID}, nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
