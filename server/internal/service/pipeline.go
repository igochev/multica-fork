package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var ErrProjectControlSettingsUnavailable = errors.New("project control settings unavailable until schema exists")

type pipelineQueries interface {
	GetIssuePipeline(ctx context.Context, issueID pgtype.UUID) (db.IssuePipeline, error)
	GetPipelineInWorkspace(ctx context.Context, arg db.GetPipelineInWorkspaceParams) (db.Pipeline, error)
	ListPipelineStages(ctx context.Context, pipelineID pgtype.UUID) ([]db.PipelineStage, error)
	GetNextPipelineStageByPosition(ctx context.Context, arg db.GetNextPipelineStageByPositionParams) (db.PipelineStage, error)
	AdvanceIssuePipelineStage(ctx context.Context, arg db.AdvanceIssuePipelineStageParams) (db.IssuePipeline, error)
}

type pipelineTaskEnqueuer interface {
	EnqueueTaskForAgentIssue(ctx context.Context, issue db.Issue, agentID pgtype.UUID, triggerCommentID ...pgtype.UUID) (db.AgentTaskQueue, error)
}

type PipelineService struct {
	queries pipelineQueries
	taskSvc pipelineTaskEnqueuer
}

func NewPipelineService(q *db.Queries, taskSvc *TaskService) *PipelineService {
	return &PipelineService{queries: q, taskSvc: taskSvc}
}

type ResolvedIssuePipeline struct {
	IssuePipeline db.IssuePipeline
	Pipeline      db.Pipeline
	CurrentStage  db.PipelineStage
	Stages        []db.PipelineStage
}

type PipelineAdvanceResult struct {
	Advanced          bool
	IssuePipeline     *db.IssuePipeline
	PreviousStage     *db.PipelineStage
	CurrentStage      *db.PipelineStage
	EnqueuedTask      *db.AgentTaskQueue
	ReachedFinalStage bool
}

func (s *PipelineService) ResolveEffectivePipelineForIssue(ctx context.Context, issue db.Issue) (*ResolvedIssuePipeline, error) {
	issuePipeline, err := s.queries.GetIssuePipeline(ctx, issue.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if issue.ProjectID.Valid {
				return nil, ErrProjectControlSettingsUnavailable
			}
			return nil, nil
		}
		return nil, fmt.Errorf("get issue pipeline: %w", err)
	}

	pipeline, err := s.queries.GetPipelineInWorkspace(ctx, db.GetPipelineInWorkspaceParams{
		ID:          issuePipeline.PipelineID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("get pipeline in workspace: %w", err)
	}

	stages, err := s.queries.ListPipelineStages(ctx, pipeline.ID)
	if err != nil {
		return nil, fmt.Errorf("list pipeline stages: %w", err)
	}

	currentStage, ok := findPipelineStageByID(stages, issuePipeline.CurrentStageID)
	if !ok {
		return nil, fmt.Errorf("current pipeline stage %s not found", issuePipeline.CurrentStageID.String())
	}

	return &ResolvedIssuePipeline{
		IssuePipeline: issuePipeline,
		Pipeline:      pipeline,
		CurrentStage:  currentStage,
		Stages:        stages,
	}, nil
}

func (s *PipelineService) MaybeAdvanceIssuePipeline(ctx context.Context, issue db.Issue) (*PipelineAdvanceResult, error) {
	resolved, err := s.ResolveEffectivePipelineForIssue(ctx, issue)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return &PipelineAdvanceResult{}, nil
	}

	previous := resolved.CurrentStage
	if issue.Status != previous.Status {
		return &PipelineAdvanceResult{
			IssuePipeline: &resolved.IssuePipeline,
			PreviousStage: &previous,
			CurrentStage:  &previous,
		}, nil
	}

	nextStage, err := s.queries.GetNextPipelineStageByPosition(ctx, db.GetNextPipelineStageByPositionParams{
		PipelineID: resolved.Pipeline.ID,
		Position:   resolved.CurrentStage.Position,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			previous := resolved.CurrentStage
			return &PipelineAdvanceResult{
				IssuePipeline:     &resolved.IssuePipeline,
				PreviousStage:     &previous,
				CurrentStage:      &previous,
				ReachedFinalStage: true,
			}, nil
		}
		return nil, fmt.Errorf("get next pipeline stage: %w", err)
	}

	advanced, err := s.queries.AdvanceIssuePipelineStage(ctx, db.AdvanceIssuePipelineStageParams{
		NextStageID:           nextStage.ID,
		IssueID:               issue.ID,
		CurrentStageID:        resolved.IssuePipeline.CurrentStageID,
		ExpectedStageSequence: resolved.IssuePipeline.StageSequence,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			previous := resolved.CurrentStage
			return &PipelineAdvanceResult{
				IssuePipeline: &resolved.IssuePipeline,
				PreviousStage: &previous,
				CurrentStage:  &previous,
			}, nil
		}
		return nil, fmt.Errorf("advance issue pipeline stage: %w", err)
	}

	queuedTask, err := s.taskSvc.EnqueueTaskForAgentIssue(ctx, issue, nextStage.AgentID)
	if err != nil {
		if rollbackErr := s.rollbackIssuePipelineAdvance(ctx, issue.ID, advanced, resolved.CurrentStage); rollbackErr != nil {
			return nil, fmt.Errorf("enqueue next stage task: %w (rollback failed: %v)", err, rollbackErr)
		}
		return nil, fmt.Errorf("enqueue next stage task: %w", err)
	}

	current := nextStage
	return &PipelineAdvanceResult{
		Advanced:      true,
		IssuePipeline: &advanced,
		PreviousStage: &previous,
		CurrentStage:  &current,
		EnqueuedTask:  &queuedTask,
	}, nil
}

func (s *PipelineService) rollbackIssuePipelineAdvance(ctx context.Context, issueID pgtype.UUID, advanced db.IssuePipeline, previousStage db.PipelineStage) error {
	rollbackCtx := context.WithoutCancel(ctx)
	_, err := s.queries.AdvanceIssuePipelineStage(rollbackCtx, db.AdvanceIssuePipelineStageParams{
		NextStageID:           previousStage.ID,
		IssueID:               issueID,
		CurrentStageID:        advanced.CurrentStageID,
		ExpectedStageSequence: advanced.StageSequence,
	})
	if err != nil {
		return fmt.Errorf("revert issue pipeline stage: %w", err)
	}
	return nil
}

func findPipelineStageByID(stages []db.PipelineStage, stageID pgtype.UUID) (db.PipelineStage, bool) {
	for _, stage := range stages {
		if stage.ID == stageID {
			return stage, true
		}
	}
	return db.PipelineStage{}, false
}
