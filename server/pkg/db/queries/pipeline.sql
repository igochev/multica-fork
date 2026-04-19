-- name: ListPipelines :many
SELECT * FROM pipelines
WHERE workspace_id = $1
ORDER BY created_at ASC;

-- name: GetPipeline :one
SELECT * FROM pipelines
WHERE id = $1;

-- name: GetPipelineInWorkspace :one
SELECT * FROM pipelines
WHERE id = $1 AND workspace_id = $2;

-- name: CreatePipeline :one
INSERT INTO pipelines (
    workspace_id,
    name,
    description
) VALUES (
    $1,
    $2,
    $3
)
RETURNING *;

-- name: UpdatePipeline :one
UPDATE pipelines SET
    name = COALESCE(sqlc.narg('name'), name),
    description = sqlc.narg('description'),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeletePipeline :exec
DELETE FROM pipelines
WHERE id = $1;

-- name: ListPipelineStages :many
SELECT * FROM pipeline_stages
WHERE pipeline_id = $1
ORDER BY position ASC;

-- name: CreatePipelineStage :one
INSERT INTO pipeline_stages (
    pipeline_id,
    name,
    status,
    agent_id,
    stage_instructions,
    position
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
)
RETURNING *;

-- name: UpdatePipelineStage :one
UPDATE pipeline_stages SET
    name = COALESCE(sqlc.narg('name'), name),
    status = COALESCE(sqlc.narg('status'), status),
    agent_id = COALESCE(sqlc.narg('agent_id'), agent_id),
    stage_instructions = sqlc.narg('stage_instructions'),
    position = COALESCE(sqlc.narg('position'), position),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeletePipelineStage :exec
DELETE FROM pipeline_stages
WHERE id = $1;

-- name: GetPipelineWithStagesByWorkspace :many
SELECT
    p.id AS pipeline_id,
    p.workspace_id,
    p.name AS pipeline_name,
    p.description AS pipeline_description,
    p.created_at AS pipeline_created_at,
    p.updated_at AS pipeline_updated_at,
    ps.id AS stage_id,
    ps.pipeline_id AS stage_pipeline_id,
    ps.name AS stage_name,
    ps.status AS stage_status,
    ps.agent_id AS stage_agent_id,
    ps.stage_instructions,
    ps.position AS stage_position,
    ps.created_at AS stage_created_at,
    ps.updated_at AS stage_updated_at
FROM pipelines p
LEFT JOIN pipeline_stages ps ON ps.pipeline_id = p.id
WHERE p.workspace_id = $1 AND p.id = $2
ORDER BY ps.position ASC NULLS LAST;
