package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestProjectOverseerAutonomySyncCreatesLinkedAutopilotAndScheduleTrigger(t *testing.T) {
	projectID := testUUID(101)
	workspaceID := testUUID(102)
	overseerAgentID := testUUID(103)
	autopilotID := testUUID(104)
	triggerID := testUUID(105)
	fixedNow := time.Date(2026, time.April, 19, 11, 34, 0, 0, time.UTC)

	queries := &fakeProjectOverseerAutonomyQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:       projectID,
			WorkspaceID:     workspaceID,
			ProjectTitle:    "Mission Control",
			OverseerAgentID: overseerAgentID,
			OverseerConfig: mustOverseerConfigJSON(t, map[string]any{
				"scan_interval_hours": 12,
				"scan_focus":          []string{"security", "architecture"},
				"product_context":     "Internal AI-native mission control",
				"quality_bar":         []string{"tests_required", "merge_safe"},
				"priority_weights": map[string]any{
					"security":    10,
					"architecture": 7,
				},
				"max_issues_per_run": 1,
				"require_approval":   true,
			}),
		},
		linkedAutopilotErr: pgx.ErrNoRows,
		createdAutopilot: db.Autopilot{
			ID:            autopilotID,
			WorkspaceID:   workspaceID,
			ProjectID:     projectID,
			AssigneeID:    overseerAgentID,
			Status:        "active",
			ExecutionMode: "create_issue",
		},
		createdTrigger: db.AutopilotTrigger{
			ID:             triggerID,
			AutopilotID:    autopilotID,
			Kind:           "schedule",
			Enabled:        true,
			CronExpression: textValue("0 */12 * * *"),
			Timezone:       textValue("UTC"),
		},
	}

	svc := &ProjectOverseerAutonomyService{
		queries: queries,
		now:     func() time.Time { return fixedNow },
	}

	result, err := svc.SyncProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("SyncProject returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected sync result, got nil")
	}
	if result.AutopilotID != autopilotID {
		t.Fatalf("expected autopilot_id %v, got %v", autopilotID, result.AutopilotID)
	}
	if result.TriggerID == nil || *result.TriggerID != triggerID {
		t.Fatalf("expected trigger_id %v, got %#v", triggerID, result.TriggerID)
	}
	if result.Status != ProjectOverseerAutonomyStatusActive {
		t.Fatalf("expected active status, got %q", result.Status)
	}

	if !queries.createAutopilotCalled {
		t.Fatal("expected linked autopilot to be created")
	}
	if queries.createAutopilotParams.WorkspaceID != workspaceID {
		t.Fatalf("expected workspace_id %v, got %v", workspaceID, queries.createAutopilotParams.WorkspaceID)
	}
	if queries.createAutopilotParams.ProjectID != projectID {
		t.Fatalf("expected project_id %v, got %v", projectID, queries.createAutopilotParams.ProjectID)
	}
	if queries.createAutopilotParams.AssigneeID != overseerAgentID {
		t.Fatalf("expected assignee_id %v, got %v", overseerAgentID, queries.createAutopilotParams.AssigneeID)
	}
	if queries.createAutopilotParams.Status != "active" {
		t.Fatalf("expected active autopilot status, got %q", queries.createAutopilotParams.Status)
	}
	if queries.createAutopilotParams.ExecutionMode != "create_issue" {
		t.Fatalf("expected create_issue execution mode, got %q", queries.createAutopilotParams.ExecutionMode)
	}
	if queries.createAutopilotParams.Title != "Strategic Overseer — Mission Control" {
		t.Fatalf("expected deterministic autopilot title, got %q", queries.createAutopilotParams.Title)
	}
	if got := queries.createAutopilotParams.IssueTitleTemplate.String; got != "Strategic Overseer Scan — {{date}}" {
		t.Fatalf("expected issue title template, got %q", got)
	}
	if queries.createAutopilotParams.CreatedByType != "agent" {
		t.Fatalf("expected created_by_type agent, got %q", queries.createAutopilotParams.CreatedByType)
	}
	if queries.createAutopilotParams.CreatedByID != overseerAgentID {
		t.Fatalf("expected created_by_id %v, got %v", overseerAgentID, queries.createAutopilotParams.CreatedByID)
	}
	if !strings.Contains(queries.createAutopilotParams.Description.String, "Strategic Overseer (CEO)") {
		t.Fatalf("expected strategic overseer prompt text, got %q", queries.createAutopilotParams.Description.String)
	}
	if !strings.Contains(queries.createAutopilotParams.Description.String, `"scan_interval_hours":12`) {
		t.Fatalf("expected machine-readable config header, got %q", queries.createAutopilotParams.Description.String)
	}

	if !queries.createTriggerCalled {
		t.Fatal("expected schedule trigger to be created")
	}
	if queries.createTriggerParams.AutopilotID != autopilotID {
		t.Fatalf("expected trigger autopilot_id %v, got %v", autopilotID, queries.createTriggerParams.AutopilotID)
	}
	if queries.createTriggerParams.Kind != "schedule" {
		t.Fatalf("expected schedule trigger, got %q", queries.createTriggerParams.Kind)
	}
	if !queries.createTriggerParams.Enabled {
		t.Fatal("expected created trigger to be enabled")
	}
	if got := queries.createTriggerParams.CronExpression.String; got != "0 */12 * * *" {
		t.Fatalf("expected 12h cron, got %q", got)
	}
	if got := queries.createTriggerParams.Timezone.String; got != "UTC" {
		t.Fatalf("expected UTC timezone, got %q", got)
	}
	if !queries.createTriggerParams.NextRunAt.Valid {
		t.Fatal("expected next_run_at to be set")
	}

	if !queries.updateLinkCalled {
		t.Fatal("expected project control link to be persisted")
	}
	if queries.updateLinkParams.ProjectID != projectID {
		t.Fatalf("expected link update for project %v, got %v", projectID, queries.updateLinkParams.ProjectID)
	}
	if queries.updateLinkParams.OverseerAutopilotID != autopilotID {
		t.Fatalf("expected link to autopilot %v, got %v", autopilotID, queries.updateLinkParams.OverseerAutopilotID)
	}
}

func TestProjectOverseerAutonomySyncUpdatesExistingLinkedAutopilot(t *testing.T) {
	projectID := testUUID(111)
	workspaceID := testUUID(112)
	existingAutopilotID := testUUID(113)
	oldAssigneeID := testUUID(114)
	newAssigneeID := testUUID(115)
	triggerID := testUUID(116)
	fixedNow := time.Date(2026, time.April, 19, 11, 34, 0, 0, time.UTC)

	queries := &fakeProjectOverseerAutonomyQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:            projectID,
			WorkspaceID:          workspaceID,
			ProjectTitle:         "Mission Control",
			OverseerAgentID:      newAssigneeID,
			OverseerAutopilotID:  existingAutopilotID,
			OverseerConfig:       mustOverseerConfigJSON(t, map[string]any{"scan_interval_hours": 6, "scan_focus": []string{"security"}}),
		},
		linkedAutopilot: db.Autopilot{
			ID:                 existingAutopilotID,
			WorkspaceID:        workspaceID,
			ProjectID:          projectID,
			AssigneeID:         oldAssigneeID,
			Title:              "Old title",
			Description:        textValue("old"),
			Status:             "paused",
			ExecutionMode:      "run_only",
			IssueTitleTemplate: textValue("Old Template"),
		},
		updatedAutopilot: db.Autopilot{ID: existingAutopilotID, WorkspaceID: workspaceID, ProjectID: projectID, AssigneeID: newAssigneeID, Status: "active", ExecutionMode: "create_issue"},
		triggers: []db.AutopilotTrigger{{
			ID:             triggerID,
			AutopilotID:    existingAutopilotID,
			Kind:           "schedule",
			Enabled:        true,
			CronExpression: textValue("0 */12 * * *"),
			Timezone:       textValue("UTC"),
		}},
		updatedTrigger: db.AutopilotTrigger{ID: triggerID, AutopilotID: existingAutopilotID, Kind: "schedule", Enabled: true, CronExpression: textValue("0 */6 * * *"), Timezone: textValue("UTC")},
	}

	svc := &ProjectOverseerAutonomyService{
		queries: queries,
		now:     func() time.Time { return fixedNow },
	}

	result, err := svc.SyncProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("SyncProject returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected sync result, got nil")
	}
	if result.Status != ProjectOverseerAutonomyStatusActive {
		t.Fatalf("expected active status, got %q", result.Status)
	}
	if result.AutopilotID != existingAutopilotID {
		t.Fatalf("expected existing autopilot id %v, got %v", existingAutopilotID, result.AutopilotID)
	}

	if !queries.updateAutopilotCalled {
		t.Fatal("expected linked autopilot to be updated")
	}
	if queries.updateAutopilotParams.ID != existingAutopilotID {
		t.Fatalf("expected autopilot update for %v, got %v", existingAutopilotID, queries.updateAutopilotParams.ID)
	}
	if queries.updateAutopilotParams.AssigneeID != newAssigneeID {
		t.Fatalf("expected assignee update to %v, got %v", newAssigneeID, queries.updateAutopilotParams.AssigneeID)
	}
	if got := queries.updateAutopilotParams.Status.String; got != "active" {
		t.Fatalf("expected active autopilot status, got %q", got)
	}
	if got := queries.updateAutopilotParams.ExecutionMode.String; got != "create_issue" {
		t.Fatalf("expected create_issue execution mode, got %q", got)
	}
	if got := queries.updateAutopilotParams.IssueTitleTemplate.String; got != "Strategic Overseer Scan — {{date}}" {
		t.Fatalf("expected canonical issue title template, got %q", got)
	}

	if !queries.updateTriggerCalled {
		t.Fatal("expected schedule trigger to be updated")
	}
	if queries.updateTriggerParams.ID != triggerID {
		t.Fatalf("expected trigger update for %v, got %v", triggerID, queries.updateTriggerParams.ID)
	}
	if got := queries.updateTriggerParams.CronExpression.String; got != "0 */6 * * *" {
		t.Fatalf("expected 6h cron, got %q", got)
	}
	if !queries.updateTriggerParams.NextRunAt.Valid {
		t.Fatal("expected trigger next_run_at to be recomputed")
	}
	if queries.updateLinkCalled {
		t.Fatal("expected no link rewrite when existing link is still valid")
	}
}

func TestProjectOverseerAutonomySyncPausesManagedAutopilotWhenDisabled(t *testing.T) {
	projectID := testUUID(121)
	workspaceID := testUUID(122)
	overseerAgentID := testUUID(123)
	autopilotID := testUUID(124)
	triggerID := testUUID(125)

	queries := &fakeProjectOverseerAutonomyQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:           projectID,
			WorkspaceID:         workspaceID,
			ProjectTitle:        "Mission Control",
			OverseerAutopilotID: autopilotID,
			OverseerConfig:      mustOverseerConfigJSON(t, map[string]any{}),
		},
		linkedAutopilot: db.Autopilot{
			ID:            autopilotID,
			WorkspaceID:   workspaceID,
			ProjectID:     projectID,
			AssigneeID:    overseerAgentID,
			Status:        "active",
			ExecutionMode: "create_issue",
		},
		updatedAutopilot: db.Autopilot{ID: autopilotID, WorkspaceID: workspaceID, ProjectID: projectID, AssigneeID: overseerAgentID, Status: "paused", ExecutionMode: "create_issue"},
		triggers: []db.AutopilotTrigger{{
			ID:             triggerID,
			AutopilotID:    autopilotID,
			Kind:           "schedule",
			Enabled:        true,
			CronExpression: textValue("0 */12 * * *"),
			Timezone:       textValue("UTC"),
		}},
		updatedTrigger: db.AutopilotTrigger{ID: triggerID, AutopilotID: autopilotID, Kind: "schedule", Enabled: false},
	}

	svc := &ProjectOverseerAutonomyService{queries: queries}

	result, err := svc.SyncProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("SyncProject returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected sync result, got nil")
	}
	if result.Status != ProjectOverseerAutonomyStatusPaused {
		t.Fatalf("expected paused status, got %q", result.Status)
	}
	if !queries.updateAutopilotCalled {
		t.Fatal("expected linked autopilot to be paused")
	}
	if got := queries.updateAutopilotParams.Status.String; got != "paused" {
		t.Fatalf("expected paused autopilot status, got %q", got)
	}
	if !queries.updateTriggerCalled {
		t.Fatal("expected linked schedule trigger to be disabled")
	}
	if queries.updateTriggerParams.Enabled.Valid && queries.updateTriggerParams.Enabled.Bool {
		t.Fatal("expected trigger to be disabled")
	}
	if queries.updateTriggerParams.NextRunAt.Valid {
		t.Fatal("expected disabled trigger next_run_at to be cleared")
	}
	if queries.createAutopilotCalled || queries.createTriggerCalled {
		t.Fatal("expected no new managed resources while disabling")
	}
}

func TestProjectOverseerAutonomySyncIsIdempotent(t *testing.T) {
	projectID := testUUID(131)
	workspaceID := testUUID(132)
	overseerAgentID := testUUID(133)
	autopilotID := testUUID(134)
	triggerID := testUUID(135)

	queries := &fakeProjectOverseerAutonomyQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:           projectID,
			WorkspaceID:         workspaceID,
			ProjectTitle:        "Mission Control",
			OverseerAgentID:     overseerAgentID,
			OverseerAutopilotID: autopilotID,
			OverseerConfig: mustOverseerConfigJSON(t, map[string]any{
				"scan_interval_hours": 24,
				"scan_focus":          []string{"documentation"},
			}),
		},
		linkedAutopilot: db.Autopilot{
			ID:                 autopilotID,
			WorkspaceID:        workspaceID,
			ProjectID:          projectID,
			AssigneeID:         overseerAgentID,
			Title:              "Strategic Overseer — Mission Control",
			Description:        textValue(expectedStrategicOverseerDescription(t, projectID, mustOverseerConfigJSON(t, map[string]any{"scan_interval_hours": 24, "scan_focus": []string{"documentation"}}))),
			Status:             "active",
			ExecutionMode:      "create_issue",
			IssueTitleTemplate: textValue("Strategic Overseer Scan — {{date}}"),
		},
		triggers: []db.AutopilotTrigger{{
			ID:             triggerID,
			AutopilotID:    autopilotID,
			Kind:           "schedule",
			Enabled:        true,
			CronExpression: textValue("0 9 * * *"),
			Timezone:       textValue("UTC"),
			NextRunAt:      pgtype.Timestamptz{Time: time.Date(2026, time.April, 20, 9, 0, 0, 0, time.UTC), Valid: true},
			Label:          textValue(strategicOverseerTriggerLabel),
		}},
	}

	svc := &ProjectOverseerAutonomyService{queries: queries}

	result, err := svc.SyncProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("SyncProject returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected sync result, got nil")
	}
	if result.Status != ProjectOverseerAutonomyStatusActive {
		t.Fatalf("expected active status, got %q", result.Status)
	}
	if queries.createAutopilotCalled || queries.updateAutopilotCalled {
		t.Fatal("expected idempotent sync to avoid autopilot writes")
	}
	if queries.createTriggerCalled || queries.updateTriggerCalled {
		t.Fatal("expected idempotent sync to avoid trigger writes")
	}
	if queries.updateLinkCalled {
		t.Fatal("expected idempotent sync to avoid link writes")
	}
}

func TestProjectOverseerAutonomySyncRepairsMissingTriggerNextRunAt(t *testing.T) {
	projectID := testUUID(141)
	workspaceID := testUUID(142)
	overseerAgentID := testUUID(143)
	autopilotID := testUUID(144)
	triggerID := testUUID(145)
	fixedNow := time.Date(2026, time.April, 19, 11, 34, 0, 0, time.UTC)
	rawConfig := mustOverseerConfigJSON(t, map[string]any{"scan_interval_hours": 12, "scan_focus": []string{"security"}})

	queries := &fakeProjectOverseerAutonomyQueries{
		settings: db.GetProjectControlSettingsRow{
			ProjectID:           projectID,
			WorkspaceID:         workspaceID,
			ProjectTitle:        "Mission Control",
			OverseerAgentID:     overseerAgentID,
			OverseerAutopilotID: autopilotID,
			OverseerConfig:      rawConfig,
		},
		linkedAutopilot: db.Autopilot{
			ID:                 autopilotID,
			WorkspaceID:        workspaceID,
			ProjectID:          projectID,
			AssigneeID:         overseerAgentID,
			Title:              "Strategic Overseer — Mission Control",
			Description:        textValue(expectedStrategicOverseerDescription(t, projectID, rawConfig)),
			Status:             "active",
			ExecutionMode:      "create_issue",
			IssueTitleTemplate: textValue("Strategic Overseer Scan — {{date}}"),
		},
		triggers: []db.AutopilotTrigger{{
			ID:             triggerID,
			AutopilotID:    autopilotID,
			Kind:           "schedule",
			Enabled:        true,
			CronExpression: textValue("0 */12 * * *"),
			Timezone:       textValue("UTC"),
			Label:          textValue(strategicOverseerTriggerLabel),
		},
		},
		updatedTrigger: db.AutopilotTrigger{
			ID:             triggerID,
			AutopilotID:    autopilotID,
			Kind:           "schedule",
			Enabled:        true,
			CronExpression: textValue("0 */12 * * *"),
			Timezone:       textValue("UTC"),
			Label:          textValue(strategicOverseerTriggerLabel),
		},
	}

	svc := &ProjectOverseerAutonomyService{
		queries: queries,
		now:     func() time.Time { return fixedNow },
	}

	result, err := svc.SyncProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("SyncProject returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected sync result, got nil")
	}
	if !queries.updateTriggerCalled {
		t.Fatal("expected matching trigger with NULL next_run_at to be repaired")
	}
	if queries.updateTriggerParams.ID != triggerID {
		t.Fatalf("expected trigger repair for %v, got %v", triggerID, queries.updateTriggerParams.ID)
	}
	if !queries.updateTriggerParams.NextRunAt.Valid {
		t.Fatal("expected repaired trigger next_run_at to be recomputed")
	}
	if result.NextRunAt == nil || !result.NextRunAt.Valid {
		t.Fatal("expected sync result next_run_at to be populated after repair")
	}
	if queries.createTriggerCalled {
		t.Fatal("expected missing next_run_at repair to avoid creating duplicate triggers")
	}
}

func TestProjectOverseerAutonomyCronExpression(t *testing.T) {
	tests := []struct {
		name      string
		interval  int32
		wantCron  string
		wantTZ    string
		wantError string
	}{
		{name: "6 hours", interval: 6, wantCron: "0 */6 * * *", wantTZ: "UTC"},
		{name: "12 hours", interval: 12, wantCron: "0 */12 * * *", wantTZ: "UTC"},
		{name: "24 hours", interval: 24, wantCron: "0 9 * * *", wantTZ: "UTC"},
		{name: "below range", interval: 5, wantError: "scan_interval_hours must be between 6 and 24"},
		{name: "above range", interval: 25, wantError: "scan_interval_hours must be between 6 and 24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronExpr, timezone, err := deriveOverseerSchedule(tt.interval)
			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("deriveOverseerSchedule returned error: %v", err)
			}
			if cronExpr != tt.wantCron {
				t.Fatalf("expected cron %q, got %q", tt.wantCron, cronExpr)
			}
			if timezone != tt.wantTZ {
				t.Fatalf("expected timezone %q, got %q", tt.wantTZ, timezone)
			}
		})
	}
}

type fakeProjectOverseerAutonomyQueries struct {
	settings               db.GetProjectControlSettingsRow
	settingsErr            error
	settingsProjectID      pgtype.UUID
	linkedAutopilot        db.Autopilot
	linkedAutopilotErr     error
	linkedAutopilotProject pgtype.UUID
	triggers               []db.AutopilotTrigger
	triggersErr            error
	triggersAutopilotID    pgtype.UUID
	createdAutopilot       db.Autopilot
	createAutopilotCalled  bool
	createAutopilotParams  db.CreateAutopilotParams
	createAutopilotErr     error
	updatedAutopilot       db.Autopilot
	updateAutopilotCalled  bool
	updateAutopilotParams  db.UpdateAutopilotParams
	updateAutopilotErr     error
	createdTrigger         db.AutopilotTrigger
	createTriggerCalled    bool
	createTriggerParams    db.CreateAutopilotTriggerParams
	createTriggerErr       error
	updatedTrigger         db.AutopilotTrigger
	updateTriggerCalled    bool
	updateTriggerParams    db.UpdateAutopilotTriggerParams
	updateTriggerErr       error
	updateLinkCalled       bool
	updateLinkParams       db.UpdateProjectControlOverseerAutopilotLinkParams
	updateLinkErr          error
}

func (f *fakeProjectOverseerAutonomyQueries) GetProjectControlSettings(_ context.Context, id pgtype.UUID) (db.GetProjectControlSettingsRow, error) {
	f.settingsProjectID = id
	if f.settingsErr != nil {
		return db.GetProjectControlSettingsRow{}, f.settingsErr
	}
	return f.settings, nil
}

func (f *fakeProjectOverseerAutonomyQueries) GetProjectLinkedOverseerAutopilot(_ context.Context, projectID pgtype.UUID) (db.Autopilot, error) {
	f.linkedAutopilotProject = projectID
	if f.linkedAutopilotErr != nil {
		return db.Autopilot{}, f.linkedAutopilotErr
	}
	return f.linkedAutopilot, nil
}

func (f *fakeProjectOverseerAutonomyQueries) ListAutopilotTriggers(_ context.Context, autopilotID pgtype.UUID) ([]db.AutopilotTrigger, error) {
	f.triggersAutopilotID = autopilotID
	if f.triggersErr != nil {
		return nil, f.triggersErr
	}
	return f.triggers, nil
}

func (f *fakeProjectOverseerAutonomyQueries) CreateAutopilot(_ context.Context, arg db.CreateAutopilotParams) (db.Autopilot, error) {
	f.createAutopilotCalled = true
	f.createAutopilotParams = arg
	if f.createAutopilotErr != nil {
		return db.Autopilot{}, f.createAutopilotErr
	}
	return f.createdAutopilot, nil
}

func (f *fakeProjectOverseerAutonomyQueries) UpdateAutopilot(_ context.Context, arg db.UpdateAutopilotParams) (db.Autopilot, error) {
	f.updateAutopilotCalled = true
	f.updateAutopilotParams = arg
	if f.updateAutopilotErr != nil {
		return db.Autopilot{}, f.updateAutopilotErr
	}
	return f.updatedAutopilot, nil
}

func (f *fakeProjectOverseerAutonomyQueries) CreateAutopilotTrigger(_ context.Context, arg db.CreateAutopilotTriggerParams) (db.AutopilotTrigger, error) {
	f.createTriggerCalled = true
	f.createTriggerParams = arg
	if f.createTriggerErr != nil {
		return db.AutopilotTrigger{}, f.createTriggerErr
	}
	return f.createdTrigger, nil
}

func (f *fakeProjectOverseerAutonomyQueries) UpdateAutopilotTrigger(_ context.Context, arg db.UpdateAutopilotTriggerParams) (db.AutopilotTrigger, error) {
	f.updateTriggerCalled = true
	f.updateTriggerParams = arg
	if f.updateTriggerErr != nil {
		return db.AutopilotTrigger{}, f.updateTriggerErr
	}
	return f.updatedTrigger, nil
}

func (f *fakeProjectOverseerAutonomyQueries) UpdateProjectControlOverseerAutopilotLink(_ context.Context, arg db.UpdateProjectControlOverseerAutopilotLinkParams) (db.ProjectControlSetting, error) {
	f.updateLinkCalled = true
	f.updateLinkParams = arg
	if f.updateLinkErr != nil {
		return db.ProjectControlSetting{}, f.updateLinkErr
	}
	return db.ProjectControlSetting{ProjectID: arg.ProjectID, OverseerAutopilotID: arg.OverseerAutopilotID}, nil
}

func mustOverseerConfigJSON(t *testing.T, value map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal overseer config: %v", err)
	}
	return encoded
}

func expectedStrategicOverseerDescription(t *testing.T, projectID pgtype.UUID, rawConfig []byte) string {
	t.Helper()
	description, err := buildStrategicOverseerDescription(projectID, rawConfig)
	if err != nil {
		t.Fatalf("buildStrategicOverseerDescription returned error: %v", err)
	}
	return description
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}
