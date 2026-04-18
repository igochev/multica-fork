# Multica Fork — Stage-Based Pipeline Routing
> **Scope Document v1.0** | Fork: `igochev/multica-fork` | Base: `multica-ai/multica`
> **Goal:** Eliminate human-in-the-loop column movement. Agents auto-pipeline through stages.

---

## Problem Statement

Current Multica behavior:
1. Human creates issue → assigns to Agent A
2. Agent A works → moves issue to `done`
3. Human manually moves issue to `in_review` column
4. Human manually assigns to Agent B
5. Agent B works → moves to `done`

Step 3 and 4 require human intervention. This is the blocker for fully autonomous pipelines.

**Vision:** Configure a pipeline once → issue flows through stages automatically, each stage routing to the right agent.

---

## Design Principles (Merge-Safe)

1. **New tables only** — do NOT modify existing `issues`, `agents`, or `agent_task_queue` tables
2. **JSONB for flexible metadata** — use JSONB columns for pipeline/stage config that may evolve
3. **Hooks over core changes** — modify the issue status handler to call a pipeline hook, don't rewrite the status logic
4. **Feature-flagged** — pipeline routing only fires when a pipeline is explicitly attached to an issue/project
5. **Additive only** — no existing behavior changes unless an issue has an active pipeline

---

## Data Model

### New Table: `pipelines`

```sql
CREATE TABLE pipelines (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  description   TEXT,
  -- Stages are ordered; the integer is sort order
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Unique per workspace
CREATE UNIQUE INDEX pipeline_workspace_name ON pipelines(workspace_id, name);
```

### New Table: `pipeline_stages`

```sql
CREATE TABLE pipeline_stages (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,              -- e.g. "Build", "Review", "Deploy"
  status      TEXT NOT NULL,              -- maps to issue.status: "todo", "in_progress", "in_review", "done", "blocked"
  agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
  -- Optional: instruction snippet injected INTO the agent's task prompt for this stage
  stage_instructions TEXT,
  position    INT NOT NULL DEFAULT 0,      -- sort order within pipeline
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(pipeline_id, status),
  UNIQUE(pipeline_id, position)
);
```

**Why `status` as a unique constraint per pipeline?** A pipeline can't have two stages both mapped to `in_review` — each status maps to exactly one stage in a given pipeline.

### New Table: `issue_pipelines`

```sql
CREATE TABLE issue_pipelines (
  issue_id    UUID PRIMARY KEY REFERENCES issues(id) ON DELETE CASCADE,
  pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
  current_stage_id UUID REFERENCES pipeline_stages(id),
  -- Tracks whether we've already triggered the next stage (avoids double-dispatch on retries)
  stage_sequence INT NOT NULL DEFAULT 0,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### New Column: `projects.pipeline_id` (nullable, default NULL)

```sql
ALTER TABLE projects ADD COLUMN pipeline_id UUID REFERENCES pipelines(id) ON DELETE SET NULL;
```

If an issue has no direct pipeline but its project has one, the project pipeline is used as fallback.

---

## Pipeline Routing Hook (Core Logic)

### Where to Hook: Issue Status Change Handler

The key is to hook into the moment an issue's `status` changes — not in the daemon, but in the **backend server** when it processes the status update.

**Flow:**
```
Agent calls: multica issue status <id> in_review
    ↓
Server: UPDATE issues SET status = 'in_review' WHERE id = <id>
    ↓
[NEW] Pipeline hook fires:
    1. Look up issue_pipelines for this issue
    2. Get current pipeline + current_stage_id
    3. Find next stage in pipeline (by position > current_stage.position)
    4. If next stage exists:
       a. Update issue_pipelines.current_stage_id
       b. Update issue_pipelines.stage_sequence
       c. Create agent_task_queue entry for next stage's agent_id
       d. Issue status stays at whatever the agent just set it to
```

**Key insight:** The pipeline handoff happens AFTER the agent reports completion. The daemon reports task completion → server marks task done → pipeline hook fires → next agent enqueued.

### Hook Implementation (Backend)

Location: `server/internal/handler/issue.go` in the status update handler.

```go
// Pipeline routing hook — fires after issue status is updated.
// Only triggers if the issue has an active pipeline AND is completing a stage.
func (h *Handler) maybeRouteToNextStage(ctx context.Context, issueID, workspaceID uuid.UUID) error {
    row, err := h.Queries.GetIssuePipeline(ctx, issueID)
    if err != nil || row.PipelineID == nil {
        return nil // no pipeline attached — skip
    }

    // Get current stage and find the next one
    currentStage, nextStage, err := h.Queries.GetPipelineNextStage(ctx,
        db.GetPipelineNextStageParams{
            PipelineID: *row.PipelineID,
            CurrentPos: row.CurrentStagePosition,
        })
    if err != nil || nextStage == nil {
        return nil // no next stage — pipeline complete
    }

    // Update issue pipeline tracking
    h.Queries.UpdateIssuePipelineStage(ctx, db.UpdateIssuePipelineStageParams{
        IssueID:         issueID,
        CurrentStageID:   &nextStage.ID,
        StageSequence:    row.StageSequence + 1,
    })

    // Enqueue next agent task
    h.TaskService.EnqueueTask(ctx, db.EnqueueTaskParams{
        AgentID:  nextStage.AgentID,
        IssueID:  issueID,
        Priority: 0,
        // context includes nextStage.StageInstructions + original issue context
    })

    return nil
}
```

**Critical behavior:** The pipeline hook fires AFTER the status update completes. The issue's `status` column is whatever the agent set it to. The pipeline enqueues the NEXT agent's task independently.

---

## Task Context Injection (Per-Stage Instructions)

When enqueueing the next stage's task, inject stage-specific instructions:

```go
taskContext := buildTaskContext(issue, nextStage.StageInstructions)
// Stage instructions are PREPENDED to the agent's base instructions
// Example stage instructions: "Focus only on code review. Check for:
// - security issues
// - test coverage
// - adherence to coding standards"
// Agent's own Instructions field from agent record still applies after
```

---

## Frontend Changes

### 1. Pipeline Management UI (`/settings/pipelines`)

- List all pipelines for workspace
- Create pipeline: name, description
- Add stages: name, maps-to-status dropdown, agent picker, optional stage instructions
- Drag-to-reorder stages
- Delete pipeline (warns if in use)

### 2. Project → Pipeline Assignment

- In project settings: "Pipeline" dropdown → select pipeline or "None"
- Issues in that project inherit the pipeline unless overridden

### 3. Issue → Pipeline Assignment

- In issue detail panel: "Pipeline" field → shows current pipeline + stage
- Override pipeline: change pipeline mid-stream (resets stage sequence)

### 4. Board — Pipeline Stage Indicator

- On the board: show which pipeline is active on an issue
- Show current stage name as a badge on the issue card
- Color-code stages for quick visual feedback

---

## Files to Change (Estimated Scope)

| File | Change | Est. Lines |
|------|--------|-------------|
| `server/migrations/XXX_pipeline.up.sql` | New tables | ~50 |
| `server/pkg/db/generated/pipeline.sql.go` | Auto-generated | — |
| `server/pkg/db/queries/pipeline.sql` | CRUD queries | ~80 |
| `server/pkg/db/queries/issue_pipeline.sql` | Issue-pipeline queries | ~40 |
| `server/internal/handler/pipeline.go` | Pipeline CRUD endpoints | ~150 |
| `server/internal/handler/issue.go` | Add pipeline hook in status update | ~30 |
| `server/internal/service/task.go` | `EnqueueTaskForStage` method | ~40 |
| `apps/web/app/(dashboard)/settings/pipelines/page.tsx` | Pipeline list page | ~100 |
| `apps/web/app/(dashboard)/settings/pipelines/[id]/page.tsx` | Pipeline detail/edit | ~150 |
| `apps/web/app/(dashboard)/projects/[id]/settings/page.tsx` | Project pipeline picker | ~20 |
| `apps/web/components/board/issue-card.tsx` | Stage badge on cards | ~20 |
| `packages/core/stores/pipeline.ts` | Pipeline state store | ~60 |
| **Total** | | **~740** |

---

## Merge Strategy (Upstream Compatibility)

### Guiding Rules

1. **Never modify existing tables** — only ADD columns/tables
2. **Never modify existing API routes** — only ADD new routes
3. **Hook patterns over core rewrites** — pipeline logic in a separate handler method, called from existing handlers
4. **Feature-gated by pipeline presence** — if `issue_pipelines` row doesn't exist, pipeline code never runs
5. **Separate migration files** — use a high-number migration (e.g. `050_pipeline_routing.up.sql`) so it sort-order places it after existing migrations

### Upstream Conflict Scenarios

| Upstream Change | Our Strategy |
|---|---|
| They add a `pipeline_id` column to `projects` | Our migration will fail → we'll need to ALTER our migration to skip if column exists |
| They add pipeline tables with different schema | Detect collision by table name → raise error and manual review needed |
| They change the issue status handler flow | Our hook must be additive — if their change breaks our hook, detect and log, don't crash |
| They rename the `agents` table | Unlikely, but would require cascade updates in our queries |
| They add a `status_changed` event/hook | Prefer using their event system if available over our own DB trigger |

### Migration Safety Pattern

```sql
-- migration/050_pipeline_routing.up.sql

-- Only add pipeline tables if they don't already exist
-- (handles case where upstream adds them in a later version)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'pipelines') THEN
    CREATE TABLE pipelines (...);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'pipeline_stages') THEN
    CREATE TABLE pipeline_stages (...);
  END IF;
END $$;

-- For projects.pipeline_id: add only if column doesn't exist
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'projects' AND column_name = 'pipeline_id'
  ) THEN
    ALTER TABLE projects ADD COLUMN pipeline_id UUID REFERENCES pipelines(id) ON DELETE SET NULL;
  END IF;
END $$;
```

This idempotent pattern means: our migration runs cleanly whether upstream has added the feature or not. If upstream eventually adds the same feature, our migration becomes a no-op.

### Branch Naming Convention

```
main                    ← synced from upstream (multica-ai/main)
multica-my-feats        ← our stable branch (merges from upstream main + our features)
multica-my-feats-dev    ← development branch (work in progress)
```

When upstream has a significant release:
1. `git checkout main && git pull upstream main`
2. `git checkout multica-my-feats && git merge main` (resolve any conflicts)
3. Test pipeline feature against new upstream
4. Push and verify CI

---

## Out of Scope (Future Phases)

- Conditional routing (e.g., "if PR has failures → blocked stage")
- Parallel stages (multiple agents work simultaneously)
- Stage timeouts / SLA alerts
- Pipeline templates (reusable pipeline blueprints)
- Agent auto-scaling across runtimes

These can be added as later enhancements without restructuring the core design.

---

## Decision Required Before Implementation

1. **Daemon model override** — do we want to scope the Gateway mode change alongside this, or do the pipeline routing first?
2. **Default pipeline** — should every new issue in a project auto-get the project pipeline, or require explicit assignment?
3. **Stage instructions UX** — should stage instructions be a textarea in the UI, or use skill references?

Recommend doing pipeline routing FIRST, then daemon Gateway mode as a follow-up. Pipeline routing is self-contained and immediately delivers value.
