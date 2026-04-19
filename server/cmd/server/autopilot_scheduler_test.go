package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestRecoverLostTriggersRepairsMissingNextRunAtWithoutDuplicatingTriggers(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}

	projectID := createIntegrationTestProject(t, "scheduler recovery reconcile")
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

	resp := authRequest(t, "POST", "/api/projects/"+projectID+"/control/overseer/reconcile", nil)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("reconcile route: expected 200, got %d", resp.StatusCode)
	}

	queries := db.New(testPool)
	var triggerID pgtype.UUID
	var autopilotID pgtype.UUID
	if err := testPool.QueryRow(context.Background(), `
		SELECT t.id, t.autopilot_id
		FROM autopilot_trigger t
		JOIN project_control_settings pcs ON pcs.overseer_autopilot_id = t.autopilot_id
		WHERE pcs.project_id = $1 AND t.kind = 'schedule'
	`, projectID).Scan(&triggerID, &autopilotID); err != nil {
		t.Fatalf("load overseer trigger: %v", err)
	}

	if _, err := testPool.Exec(context.Background(), `
		UPDATE autopilot_trigger
		SET next_run_at = NULL, updated_at = now() - interval '1 minute'
		WHERE id = $1
	`, triggerID); err != nil {
		t.Fatalf("clear next_run_at: %v", err)
	}

	recoverLostTriggers(context.Background(), queries)

	var nextRunAt time.Time
	var triggerCount int
	if err := testPool.QueryRow(context.Background(), `
		SELECT t.next_run_at, (
			SELECT count(*) FROM autopilot_trigger WHERE autopilot_id = $2 AND kind = 'schedule'
		)
		FROM autopilot_trigger t
		WHERE t.id = $1
	`, triggerID, autopilotID).Scan(&nextRunAt, &triggerCount); err != nil {
		t.Fatalf("verify recovered trigger: %v", err)
	}
	if nextRunAt.IsZero() {
		t.Fatal("expected recovered trigger next_run_at to be repopulated")
	}
	if triggerCount != 1 {
		t.Fatalf("expected scheduler recovery to keep one schedule trigger, got %d", triggerCount)
	}
}
