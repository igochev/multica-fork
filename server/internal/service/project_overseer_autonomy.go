package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	strategicOverseerAutopilotTitle       = "Strategic Overseer — %s"
	strategicOverseerIssueTitleTemplate   = "Strategic Overseer Scan — {{date}}"
	strategicOverseerTriggerLabel         = "Strategic Overseer Schedule"
	strategicOverseerDefaultTimezone      = "UTC"
	strategicOverseerDefaultPriority      = "high"
)

type ProjectOverseerAutonomyStatus string

const (
	ProjectOverseerAutonomyStatusActive ProjectOverseerAutonomyStatus = "active"
	ProjectOverseerAutonomyStatusPaused ProjectOverseerAutonomyStatus = "paused"
)

type ProjectOverseerAutonomySyncResult struct {
	AutopilotID pgtype.UUID
	TriggerID   *pgtype.UUID
	NextRunAt   *pgtype.Timestamptz
	Status      ProjectOverseerAutonomyStatus
	CreatedAutopilot   bool
	UpdatedAutopilot   bool
	CreatedTrigger     bool
	UpdatedTrigger     bool
	UpdatedProjectLink bool
}

type ProjectOverseerAutonomyTriggerSyncResult struct {
	TriggerID *pgtype.UUID
	NextRunAt *pgtype.Timestamptz
	Created   bool
	Updated   bool
}

type projectOverseerAutonomyQueries interface {
	GetProjectControlSettings(ctx context.Context, id pgtype.UUID) (db.GetProjectControlSettingsRow, error)
	GetProjectLinkedOverseerAutopilot(ctx context.Context, projectID pgtype.UUID) (db.Autopilot, error)
	ListAutopilotTriggers(ctx context.Context, autopilotID pgtype.UUID) ([]db.AutopilotTrigger, error)
	CreateAutopilot(ctx context.Context, arg db.CreateAutopilotParams) (db.Autopilot, error)
	UpdateAutopilot(ctx context.Context, arg db.UpdateAutopilotParams) (db.Autopilot, error)
	CreateAutopilotTrigger(ctx context.Context, arg db.CreateAutopilotTriggerParams) (db.AutopilotTrigger, error)
	UpdateAutopilotTrigger(ctx context.Context, arg db.UpdateAutopilotTriggerParams) (db.AutopilotTrigger, error)
	UpdateProjectControlOverseerAutopilotLink(ctx context.Context, arg db.UpdateProjectControlOverseerAutopilotLinkParams) (db.ProjectControlSetting, error)
}

type ProjectOverseerAutonomyService struct {
	queries projectOverseerAutonomyQueries
	now     func() time.Time
}

func NewProjectOverseerAutonomyService(q *db.Queries) *ProjectOverseerAutonomyService {
	return &ProjectOverseerAutonomyService{queries: q, now: time.Now}
}

func (s *ProjectOverseerAutonomyService) SyncProject(ctx context.Context, projectID pgtype.UUID) (*ProjectOverseerAutonomySyncResult, error) {
	settings, err := s.queries.GetProjectControlSettings(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project control settings: %w", err)
	}

	desired, err := desiredProjectOverseerAutonomy(settings)
	if err != nil {
		return nil, err
	}

	managedAutopilot, err := s.queries.GetProjectLinkedOverseerAutopilot(ctx, projectID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get linked overseer autopilot: %w", err)
	}
	linkedExists := err == nil

	if !desired.enabled {
		if !linkedExists {
			return &ProjectOverseerAutonomySyncResult{Status: ProjectOverseerAutonomyStatusPaused}, nil
		}
		return s.pauseManagedAutopilot(ctx, managedAutopilot)
	}

	if !linkedExists {
		return s.createManagedAutopilot(ctx, settings, desired)
	}
	return s.upsertManagedAutopilot(ctx, settings, managedAutopilot, desired)
}

type desiredOverseerAutonomy struct {
	enabled     bool
	workspaceID pgtype.UUID
	projectID   pgtype.UUID
	projectTitle string
	assigneeID  pgtype.UUID
	title       string
	description string
	cron        string
	timezone    string
}

func desiredProjectOverseerAutonomy(settings db.GetProjectControlSettingsRow) (desiredOverseerAutonomy, error) {
	if !settings.OverseerAgentID.Valid || len(settings.OverseerConfig) == 0 || isEmptyJSONObject(settings.OverseerConfig) {
		return desiredOverseerAutonomy{}, nil
	}

	var config struct {
		ScanIntervalHours *int32 `json:"scan_interval_hours"`
	}
	if err := json.Unmarshal(settings.OverseerConfig, &config); err != nil {
		return desiredOverseerAutonomy{}, fmt.Errorf("parse overseer config: %w", err)
	}
	if config.ScanIntervalHours == nil {
		return desiredOverseerAutonomy{}, nil
	}

	cronExpr, timezone, err := deriveOverseerSchedule(*config.ScanIntervalHours)
	if err != nil {
		return desiredOverseerAutonomy{}, err
	}
	description, err := buildStrategicOverseerDescription(settings.ProjectID, settings.OverseerConfig)
	if err != nil {
		return desiredOverseerAutonomy{}, err
	}

	return desiredOverseerAutonomy{
		enabled:      true,
		workspaceID:  settings.WorkspaceID,
		projectID:    settings.ProjectID,
		projectTitle: settings.ProjectTitle,
		assigneeID:   settings.OverseerAgentID,
		title:        fmt.Sprintf(strategicOverseerAutopilotTitle, settings.ProjectTitle),
		description:  description,
		cron:         cronExpr,
		timezone:     timezone,
	}, nil
}

func (s *ProjectOverseerAutonomyService) createManagedAutopilot(ctx context.Context, settings db.GetProjectControlSettingsRow, desired desiredOverseerAutonomy) (*ProjectOverseerAutonomySyncResult, error) {
	autopilot, err := s.queries.CreateAutopilot(ctx, db.CreateAutopilotParams{
		WorkspaceID:        desired.workspaceID,
		ProjectID:          desired.projectID,
		Title:              desired.title,
		Description:        pgtype.Text{String: desired.description, Valid: true},
		AssigneeID:         desired.assigneeID,
		Priority:           strategicOverseerDefaultPriority,
		Status:             "active",
		ExecutionMode:      "create_issue",
		IssueTitleTemplate: pgtype.Text{String: strategicOverseerIssueTitleTemplate, Valid: true},
		CreatedByType:      "agent",
		CreatedByID:        desired.assigneeID,
	})
	if err != nil {
		return nil, fmt.Errorf("create overseer autopilot: %w", err)
	}

	trigger, err := s.createScheduleTrigger(ctx, autopilot.ID, desired)
	if err != nil {
		return nil, err
	}
	if _, err := s.queries.UpdateProjectControlOverseerAutopilotLink(ctx, db.UpdateProjectControlOverseerAutopilotLinkParams{
		ProjectID:           settings.ProjectID,
		OverseerAutopilotID: autopilot.ID,
	}); err != nil {
		return nil, fmt.Errorf("persist overseer autopilot link: %w", err)
	}

	return &ProjectOverseerAutonomySyncResult{
		AutopilotID: autopilot.ID,
		TriggerID:   ptrUUID(trigger.ID),
		NextRunAt:   ptrTimestamp(trigger.NextRunAt),
		Status:      ProjectOverseerAutonomyStatusActive,
		CreatedAutopilot:   true,
		CreatedTrigger:     true,
		UpdatedProjectLink: true,
	}, nil
}

func (s *ProjectOverseerAutonomyService) upsertManagedAutopilot(ctx context.Context, settings db.GetProjectControlSettingsRow, autopilot db.Autopilot, desired desiredOverseerAutonomy) (*ProjectOverseerAutonomySyncResult, error) {
	updatedAutopilot := false
	if !autopilotMatchesDesired(autopilot, desired) {
		updated, err := s.queries.UpdateAutopilot(ctx, db.UpdateAutopilotParams{
			ID:                 autopilot.ID,
			Title:              pgtype.Text{String: desired.title, Valid: true},
			Description:        pgtype.Text{String: desired.description, Valid: true},
			AssigneeID:         desired.assigneeID,
			ProjectID:          desired.projectID,
			Priority:           pgtype.Text{String: strategicOverseerDefaultPriority, Valid: true},
			Status:             pgtype.Text{String: "active", Valid: true},
			ExecutionMode:      pgtype.Text{String: "create_issue", Valid: true},
			IssueTitleTemplate: pgtype.Text{String: strategicOverseerIssueTitleTemplate, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("update overseer autopilot: %w", err)
		}
		autopilot = updated
		updatedAutopilot = true
	}

	triggerSync, err := s.ensureScheduleTrigger(ctx, autopilot.ID, desired)
	if err != nil {
		return nil, err
	}
	updatedProjectLink := false
	if !settings.OverseerAutopilotID.Valid || settings.OverseerAutopilotID != autopilot.ID {
		if _, err := s.queries.UpdateProjectControlOverseerAutopilotLink(ctx, db.UpdateProjectControlOverseerAutopilotLinkParams{
			ProjectID:           settings.ProjectID,
			OverseerAutopilotID: autopilot.ID,
		}); err != nil {
			return nil, fmt.Errorf("persist overseer autopilot link: %w", err)
		}
		updatedProjectLink = true
	}

	return &ProjectOverseerAutonomySyncResult{
		AutopilotID: autopilot.ID,
		TriggerID:   triggerSync.TriggerID,
		NextRunAt:   triggerSync.NextRunAt,
		Status:      ProjectOverseerAutonomyStatusActive,
		UpdatedAutopilot:   updatedAutopilot,
		CreatedTrigger:     triggerSync.Created,
		UpdatedTrigger:     triggerSync.Updated,
		UpdatedProjectLink: updatedProjectLink,
	}, nil
}

func (s *ProjectOverseerAutonomyService) pauseManagedAutopilot(ctx context.Context, autopilot db.Autopilot) (*ProjectOverseerAutonomySyncResult, error) {
	if autopilot.Status != "paused" {
		updated, err := s.queries.UpdateAutopilot(ctx, db.UpdateAutopilotParams{
			ID:                 autopilot.ID,
			Status:             pgtype.Text{String: "paused", Valid: true},
			ExecutionMode:      pgtype.Text{String: autopilot.ExecutionMode, Valid: true},
			IssueTitleTemplate: autopilot.IssueTitleTemplate,
		})
		if err != nil {
			return nil, fmt.Errorf("pause overseer autopilot: %w", err)
		}
		autopilot = updated
	}

	triggers, err := s.queries.ListAutopilotTriggers(ctx, autopilot.ID)
	if err != nil {
		return nil, fmt.Errorf("list overseer autopilot triggers: %w", err)
	}
	var triggerID *pgtype.UUID
	var nextRunAt *pgtype.Timestamptz
	for _, trigger := range triggers {
		if trigger.Kind != "schedule" {
			continue
		}
		triggerID = ptrUUID(trigger.ID)
		nextRunAt = ptrTimestamp(trigger.NextRunAt)
		if trigger.Enabled {
			if _, err := s.queries.UpdateAutopilotTrigger(ctx, db.UpdateAutopilotTriggerParams{
				ID:        trigger.ID,
				Enabled:   pgtype.Bool{Bool: false, Valid: true},
				NextRunAt: pgtype.Timestamptz{},
			}); err != nil {
				return nil, fmt.Errorf("disable overseer autopilot trigger: %w", err)
			}
			nextRunAt = nil
		}
	}

	return &ProjectOverseerAutonomySyncResult{
		AutopilotID: autopilot.ID,
		TriggerID:   triggerID,
		NextRunAt:   nextRunAt,
		Status:      ProjectOverseerAutonomyStatusPaused,
	}, nil
}

func (s *ProjectOverseerAutonomyService) ensureScheduleTrigger(ctx context.Context, autopilotID pgtype.UUID, desired desiredOverseerAutonomy) (ProjectOverseerAutonomyTriggerSyncResult, error) {
	triggers, err := s.queries.ListAutopilotTriggers(ctx, autopilotID)
	if err != nil {
		return ProjectOverseerAutonomyTriggerSyncResult{}, fmt.Errorf("list overseer autopilot triggers: %w", err)
	}
	for _, trigger := range triggers {
		if trigger.Kind != "schedule" {
			continue
		}
		if triggerMatchesDesired(trigger, desired) {
			return ProjectOverseerAutonomyTriggerSyncResult{TriggerID: ptrUUID(trigger.ID), NextRunAt: ptrTimestamp(trigger.NextRunAt)}, nil
		}
		plannedNextRunAt := nextRunAt(s.timeNow(), desired.cron, desired.timezone)
		updated, err := s.queries.UpdateAutopilotTrigger(ctx, db.UpdateAutopilotTriggerParams{
			ID:             trigger.ID,
			Enabled:        pgtype.Bool{Bool: true, Valid: true},
			CronExpression: pgtype.Text{String: desired.cron, Valid: true},
			Timezone:       pgtype.Text{String: desired.timezone, Valid: true},
			NextRunAt:      plannedNextRunAt,
			Label:          pgtype.Text{String: strategicOverseerTriggerLabel, Valid: true},
		})
		if err != nil {
			return ProjectOverseerAutonomyTriggerSyncResult{}, fmt.Errorf("update overseer autopilot trigger: %w", err)
		}
		return ProjectOverseerAutonomyTriggerSyncResult{TriggerID: ptrUUID(updated.ID), NextRunAt: ptrTimestamp(plannedNextRunAt), Updated: true}, nil
	}

	trigger, err := s.createScheduleTrigger(ctx, autopilotID, desired)
	if err != nil {
		return ProjectOverseerAutonomyTriggerSyncResult{}, err
	}
	return ProjectOverseerAutonomyTriggerSyncResult{TriggerID: ptrUUID(trigger.ID), NextRunAt: ptrTimestamp(trigger.NextRunAt), Created: true}, nil
}

func (s *ProjectOverseerAutonomyService) createScheduleTrigger(ctx context.Context, autopilotID pgtype.UUID, desired desiredOverseerAutonomy) (db.AutopilotTrigger, error) {
	trigger, err := s.queries.CreateAutopilotTrigger(ctx, db.CreateAutopilotTriggerParams{
		AutopilotID:    autopilotID,
		Kind:           "schedule",
		Enabled:        true,
		CronExpression: pgtype.Text{String: desired.cron, Valid: true},
		Timezone:       pgtype.Text{String: desired.timezone, Valid: true},
		NextRunAt:      nextRunAt(s.timeNow(), desired.cron, desired.timezone),
		Label:          pgtype.Text{String: strategicOverseerTriggerLabel, Valid: true},
	})
	if err != nil {
		return db.AutopilotTrigger{}, fmt.Errorf("create overseer autopilot trigger: %w", err)
	}
	return trigger, nil
}

func deriveOverseerSchedule(intervalHours int32) (string, string, error) {
	switch intervalHours {
	case 6:
		return "0 */6 * * *", strategicOverseerDefaultTimezone, nil
	case 12:
		return "0 */12 * * *", strategicOverseerDefaultTimezone, nil
	case 24:
		return "0 9 * * *", strategicOverseerDefaultTimezone, nil
	default:
		return "", "", fmt.Errorf("scan_interval_hours must be between 6 and 24")
	}
}

func buildStrategicOverseerDescription(projectID pgtype.UUID, rawConfig []byte) (string, error) {
	normalized, err := normalizeJSON(rawConfig)
	if err != nil {
		return "", fmt.Errorf("normalize overseer config: %w", err)
	}

	prompt := strings.Join([]string{
		"Strategic Overseer (CEO)",
		"You are the strategic CEO of this project.",
		"Review the codebase, board state, documentation, architecture, and risks.",
		"Create at most one high-leverage issue for the most important opportunity you find.",
		"Issues must stay at user-story level, not implementation-step level.",
		"If no meaningful work should be created, do nothing.",
		"",
		"--- OVERSEER_CONFIG ---",
		fmt.Sprintf(`{"project_id":"%s","config":%s}`, projectID.String(), normalized),
	}, "\n")
	return prompt, nil
}

func normalizeJSON(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "{}", nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func autopilotMatchesDesired(autopilot db.Autopilot, desired desiredOverseerAutonomy) bool {
	return autopilot.ProjectID == desired.projectID &&
		autopilot.AssigneeID == desired.assigneeID &&
		autopilot.Title == desired.title &&
		autopilot.Description.Valid && autopilot.Description.String == desired.description &&
		autopilot.Status == "active" &&
		autopilot.ExecutionMode == "create_issue" &&
		autopilot.IssueTitleTemplate.Valid && autopilot.IssueTitleTemplate.String == strategicOverseerIssueTitleTemplate
}

func triggerMatchesDesired(trigger db.AutopilotTrigger, desired desiredOverseerAutonomy) bool {
	return trigger.Enabled &&
		trigger.CronExpression.Valid && trigger.CronExpression.String == desired.cron &&
		trigger.Timezone.Valid && trigger.Timezone.String == desired.timezone &&
		trigger.NextRunAt.Valid &&
		trigger.Label.Valid && trigger.Label.String == strategicOverseerTriggerLabel
}

func isEmptyJSONObject(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "{}"
}

func nextRunAt(now time.Time, cronExpr, timezone string) pgtype.Timestamptz {
	next, err := ComputeNextRunFrom(cronExpr, timezone, now)
	if err != nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: next, Valid: true}
}

func (s *ProjectOverseerAutonomyService) timeNow() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func ptrUUID(value pgtype.UUID) *pgtype.UUID {
	copied := value
	return &copied
}

func ptrTimestamp(value pgtype.Timestamptz) *pgtype.Timestamptz {
	if !value.Valid {
		return nil
	}
	copied := value
	return &copied
}
