-- name: GetIssuePipeline :one
SELECT * FROM issue_pipelines
WHERE issue_id = $1;

-- name: CreateIssuePipeline :one
INSERT INTO issue_pipelines (
    issue_id,
    pipeline_id,
    current_stage_id,
    stage_sequence
) VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING *;

-- name: UpdateIssuePipeline :one
UPDATE issue_pipelines SET
    pipeline_id = $2,
    current_stage_id = $3,
    stage_sequence = $4,
    updated_at = now()
WHERE issue_id = $1
RETURNING *;

-- name: DeleteIssuePipeline :exec
DELETE FROM issue_pipelines
WHERE issue_id = $1;

-- name: AdvanceIssuePipelineStage :one
UPDATE issue_pipelines
SET current_stage_id = sqlc.arg('next_stage_id'),
    stage_sequence = stage_sequence + 1,
    updated_at = now()
WHERE issue_id = sqlc.arg('issue_id')
  AND current_stage_id = sqlc.arg('current_stage_id')
  AND stage_sequence = sqlc.arg('expected_stage_sequence')
RETURNING *;

-- name: GetNextPipelineStageByPosition :one
SELECT * FROM pipeline_stages
WHERE pipeline_id = $1
  AND position > $2
ORDER BY position ASC
LIMIT 1;
