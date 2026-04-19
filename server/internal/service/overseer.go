package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type overseerQueries interface {
	GetProjectControlSettings(ctx context.Context, id pgtype.UUID) (db.GetProjectControlSettingsRow, error)
	ListTasksByIssue(ctx context.Context, issueID pgtype.UUID) ([]db.AgentTaskQueue, error)
	CreateActivity(ctx context.Context, arg db.CreateActivityParams) (db.ActivityLog, error)
}

type overseerTaskService interface {
	EnqueueTaskForAgentIssue(ctx context.Context, issue db.Issue, agentID pgtype.UUID, triggerCommentID ...pgtype.UUID) (db.AgentTaskQueue, error)
	CancelTask(ctx context.Context, taskID pgtype.UUID) (*db.AgentTaskQueue, error)
}

type OverseerService struct {
	queries overseerQueries
	taskSvc overseerTaskService
	now     func() time.Time
}

type OverseerEscalationResult struct {
	Requested    bool
	EnqueuedTask *db.AgentTaskQueue
}

func NewOverseerService(q *db.Queries, taskSvc *TaskService) *OverseerService {
	return &OverseerService{queries: q, taskSvc: taskSvc, now: time.Now}
}

func (s *OverseerService) MaybeEscalateBlockedIssue(ctx context.Context, issue db.Issue) (*OverseerEscalationResult, error) {
	if !issue.ProjectID.Valid || issue.Status != "blocked" {
		return &OverseerEscalationResult{}, nil
	}

	settings, err := s.queries.GetProjectControlSettings(ctx, issue.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project control settings: %w", err)
	}
	if !settings.AutoEscalateBlocked || !settings.OverseerAgentID.Valid {
		return &OverseerEscalationResult{}, nil
	}

	return s.enqueueEscalationIfNeeded(ctx, issue, settings.OverseerAgentID, "blocked_issue", "overseer_requested_for_blocked_issue")
}

func (s *OverseerService) MaybeEscalateStaleIssue(ctx context.Context, issue db.Issue) (*OverseerEscalationResult, error) {
	if !issue.ProjectID.Valid || issue.Status == "done" || issue.Status == "cancelled" {
		return &OverseerEscalationResult{}, nil
	}

	settings, err := s.queries.GetProjectControlSettings(ctx, issue.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project control settings: %w", err)
	}
	if !settings.OverseerAgentID.Valid || settings.StaleAfterMinutes <= 0 {
		return &OverseerEscalationResult{}, nil
	}

	tasks, err := s.queries.ListTasksByIssue(ctx, issue.ID)
	if err != nil {
		return nil, fmt.Errorf("list issue tasks: %w", err)
	}
	latestInFlightTask, ok := latestInFlightTask(tasks)
	if !ok {
		return &OverseerEscalationResult{}, nil
	}
	if isTaskFresh(latestInFlightTask, time.Duration(settings.StaleAfterMinutes)*time.Minute, s.timeNow()) {
		return &OverseerEscalationResult{}, nil
	}

	return s.enqueueEscalationFromTasks(ctx, issue, settings.OverseerAgentID, tasks, "stale_issue", "overseer_requested_for_stale_issue")
}

func (s *OverseerService) enqueueEscalationIfNeeded(ctx context.Context, issue db.Issue, overseerAgentID pgtype.UUID, reason, action string) (*OverseerEscalationResult, error) {
	tasks, err := s.queries.ListTasksByIssue(ctx, issue.ID)
	if err != nil {
		return nil, fmt.Errorf("list issue tasks: %w", err)
	}

	return s.enqueueEscalationFromTasks(ctx, issue, overseerAgentID, tasks, reason, action)
}

func (s *OverseerService) enqueueEscalationFromTasks(ctx context.Context, issue db.Issue, overseerAgentID pgtype.UUID, tasks []db.AgentTaskQueue, reason, action string) (*OverseerEscalationResult, error) {
	for _, task := range tasks {
		if task.AgentID == overseerAgentID && isOverseerTaskInFlight(task.Status) {
			return &OverseerEscalationResult{}, nil
		}
	}

	enqueuedTask, err := s.taskSvc.EnqueueTaskForAgentIssue(ctx, issue, overseerAgentID)
	if err != nil {
		return nil, fmt.Errorf("enqueue overseer task: %w", err)
	}
	if err := s.createIssueActivity(ctx, issue, overseerAgentID, enqueuedTask.ID, reason, action); err != nil {
		if _, cancelErr := s.taskSvc.CancelTask(context.WithoutCancel(ctx), enqueuedTask.ID); cancelErr != nil {
			return nil, fmt.Errorf("create overseer activity: %w (rollback failed: %v)", err, cancelErr)
		}
		return nil, err
	}

	return &OverseerEscalationResult{Requested: true, EnqueuedTask: &enqueuedTask}, nil
}

func (s *OverseerService) createIssueActivity(ctx context.Context, issue db.Issue, overseerAgentID, taskID pgtype.UUID, reason, action string) error {
	details, err := json.Marshal(map[string]any{
		"reason":            reason,
		"overseer_agent_id": overseerAgentID.String(),
		"task_id":           taskID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal overseer activity details: %w", err)
	}

	_, err = s.queries.CreateActivity(ctx, db.CreateActivityParams{
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
		ActorType:   pgtype.Text{String: "system", Valid: true},
		ActorID:     pgtype.UUID{},
		Action:      action,
		Details:     details,
	})
	if err != nil {
		return fmt.Errorf("create overseer activity: %w", err)
	}
	return nil
}

func latestInFlightTask(tasks []db.AgentTaskQueue) (db.AgentTaskQueue, bool) {
	for _, task := range tasks {
		if isOverseerTaskInFlight(task.Status) {
			return task, true
		}
	}
	return db.AgentTaskQueue{}, false
}

func isTaskFresh(task db.AgentTaskQueue, staleAfter time.Duration, now time.Time) bool {
	if !task.CreatedAt.Valid {
		return false
	}
	return now.Sub(task.CreatedAt.Time) < staleAfter
}

func (s *OverseerService) timeNow() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func isOverseerTaskInFlight(status string) bool {
	switch status {
	case "queued", "dispatched", "running":
		return true
	default:
		return false
	}
}
