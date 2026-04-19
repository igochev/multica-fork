-- name: GetProjectControlSettings :one
SELECT
    p.id AS project_id,
    p.workspace_id,
    p.title AS project_title,
    pcs.overseer_agent_id,
    pcs.overseer_autopilot_id,
    pcs.default_pipeline_id,
    COALESCE(pcs.automation_mode, 'manual') AS automation_mode,
    COALESCE(pcs.auto_triage_enabled, false) AS auto_triage_enabled,
    COALESCE(pcs.auto_route_enabled, false) AS auto_route_enabled,
    COALESCE(pcs.auto_escalate_blocked, false) AS auto_escalate_blocked,
    COALESCE(pcs.stale_after_minutes, 60) AS stale_after_minutes,
    COALESCE(pcs.review_policy, '{}'::jsonb) AS review_policy,
    COALESCE(pcs.quality_policy, '{}'::jsonb) AS quality_policy,
    COALESCE(pcs.overseer_config, '{}'::jsonb) AS overseer_config,
    COALESCE(pcs.created_at, p.created_at) AS created_at,
    COALESCE(pcs.updated_at, p.updated_at) AS updated_at
FROM project p
LEFT JOIN project_control_settings pcs ON pcs.project_id = p.id
WHERE p.id = $1;

-- name: UpsertProjectControlSettings :one
INSERT INTO project_control_settings (
    project_id,
    overseer_agent_id,
    default_pipeline_id,
    automation_mode,
    auto_triage_enabled,
    auto_route_enabled,
    auto_escalate_blocked,
    stale_after_minutes,
    review_policy,
    quality_policy,
    overseer_config
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11
)
ON CONFLICT (project_id)
DO UPDATE SET
    overseer_agent_id = EXCLUDED.overseer_agent_id,
    default_pipeline_id = EXCLUDED.default_pipeline_id,
    automation_mode = EXCLUDED.automation_mode,
    auto_triage_enabled = EXCLUDED.auto_triage_enabled,
    auto_route_enabled = EXCLUDED.auto_route_enabled,
    auto_escalate_blocked = EXCLUDED.auto_escalate_blocked,
    stale_after_minutes = EXCLUDED.stale_after_minutes,
    review_policy = EXCLUDED.review_policy,
    quality_policy = EXCLUDED.quality_policy,
    overseer_config = EXCLUDED.overseer_config,
    updated_at = now()
RETURNING *;

-- name: UpdateProjectControlOverseerAutopilotLink :one
UPDATE project_control_settings
SET overseer_autopilot_id = $2,
    updated_at = now()
WHERE project_id = $1
RETURNING *;

-- name: ListProjectsWithAutomationEnabled :many
SELECT p.*
FROM project p
JOIN project_control_settings pcs ON pcs.project_id = p.id
WHERE p.workspace_id = $1
  AND (
    pcs.automation_mode <> 'manual'
    OR pcs.auto_triage_enabled
    OR pcs.auto_route_enabled
    OR pcs.auto_escalate_blocked
  )
ORDER BY p.created_at DESC;
