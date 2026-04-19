# Elite Mission Control — Stage Routing + Overseer O1 Implementation Plan

> **For Hermes:** Use `subagent-driven-development` + `test-driven-development` to implement this plan task-by-task. Do not re-plan. Execute one task at a time, keep changes additive, and do not proceed to the next task until the current task passes spec review and code-quality review.

**Goal:** Add the first real Mission Control control-plane slices to the Multica fork: deterministic stage-based routing plus explicit Overseer O1 project control with blocked escalation first and stale escalation second.

**Architecture:** Keep upstream semantics clean. Do **not** overload `project.lead_*`. Introduce explicit pipeline tables and explicit `project_control_settings`. Route work through existing issue + task seams (`server/internal/handler/issue.go`, `server/internal/service/task.go`) using new focused services (`pipeline.go`, `overseer.go`). Use existing `/settings` tab architecture for pipeline management and existing project detail UI for project-level automation settings.

**Tech Stack:** Go + Chi + sqlc + PostgreSQL on the backend; React Query + shared `packages/core` + shared `packages/views` + Next.js/Electron shells on the frontend.

---

## Scope and intentional design choices

### In scope
- workspace-level pipeline CRUD
- stage-based automatic handoff after issue status changes
- project-scoped explicit control settings
- explicit Overseer agent selection
- blocked-issue escalation to Overseer
- stale-issue reconciliation path as a follow-up slice in the same plan
- pipeline management UI in existing Settings page
- project automation UI in Project Detail

### Out of scope for this plan
- daemon gateway mode changes
- hidden Project Lead semantics
- generic workflow builder DSL
- full review-orchestration brain in the first slice
- CLI support for pipelines/control settings in the first pass

### Important refinement to the earlier stage-routing spec
Do **not** add `project.pipeline_id` directly to the `project` table.

Instead use:
- `project_control_settings.default_pipeline_id`

Why:
- keeps project base table cleaner
- keeps execution control in the control-plane config layer
- aligns with the newly adopted Overseer architecture
- is easier to reason about during upstream sync

---

## Read before implementing

- `docs/architecture/elite-mission-control.md`
- `docs/architecture/overseer-design-spec.md`
- `SPEC-STAGE-ROUTING.md`
- `CLAUDE.md`
- `PLAN.md`

---

## Milestone A — Deterministic stage-routing backbone

### Task 1: Write failing backend tests for pipeline CRUD and stage routing

**Objective:** Define the backend contract first, before touching schema or handlers.

**Files:**
- Create: `server/internal/handler/pipeline_test.go`
- Modify: `server/internal/handler/handler_test.go` (only if shared test helpers are needed)

**Step 1: Write failing tests for pipeline CRUD**
Cover at minimum:
- create pipeline with ordered stages
- list pipelines for workspace
- update pipeline metadata/stages
- delete pipeline
- reject duplicate stage positions within a pipeline

**Step 2: Write failing test for project control fallback pipeline**
Target behavior:
- issue with no direct pipeline uses `project_control_settings.default_pipeline_id`

**Step 3: Write failing test for stage handoff**
Target behavior:
- when an issue completes the current stage status, the next stage agent is queued
- current issue status remains what the actor set
- duplicate enqueue does not happen on repeated updates

**Step 4: Run targeted tests and verify RED**
Run:
```bash
cd server && go test ./internal/handler -run 'TestPipeline|TestStageRouting' -count=1
```
Expected:
- FAIL from missing routes / missing queries / missing tables

**Step 5: Commit test scaffold**
```bash
git add server/internal/handler/pipeline_test.go server/internal/handler/handler_test.go
git commit -m "test: add failing pipeline routing handler coverage"
```

---

### Task 2: Add schema for pipelines and issue-pipeline tracking

**Objective:** Create the additive DB layer for deterministic routing.

**Files:**
- Create: `server/migrations/049_pipeline_routing.up.sql`
- Create: `server/migrations/049_pipeline_routing.down.sql`

**Step 1: Add new tables**
Create:
- `pipelines`
- `pipeline_stages`
- `issue_pipelines`

Use these rules:
- `workspace_id` on `pipelines`
- ordered `position` on `pipeline_stages`
- one current stage reference on `issue_pipelines`
- `stage_sequence` int for optimistic advancement / dedup tracking

**Step 2: Add useful indexes/constraints**
Include at least:
- unique `(workspace_id, name)` on pipelines
- unique `(pipeline_id, position)` on pipeline stages
- primary key on `issue_pipelines.issue_id`

**Step 3: Run migrations locally**
Run:
```bash
make migrate-up
```
Expected:
- migration applies successfully

**Step 4: Commit**
```bash
git add server/migrations/049_pipeline_routing.*
git commit -m "feat: add pipeline routing schema"
```

---

### Task 3: Add sqlc queries for pipelines and issue-pipeline state

**Objective:** Create the backend data-access seam before handler/service implementation.

**Files:**
- Create: `server/pkg/db/queries/pipeline.sql`
- Create: `server/pkg/db/queries/issue_pipeline.sql`
- Modify (generated): `server/pkg/db/generated/*.go`

**Step 1: Add pipeline CRUD queries**
Include queries for:
- list/get/create/update/delete pipeline
- list/create/update/delete pipeline stages
- get pipeline with stages by workspace

**Step 2: Add issue-pipeline queries**
Include queries for:
- get effective pipeline for issue (direct mapping first, project-control fallback second)
- create/update/delete issue-pipeline mapping
- advance current stage with optimistic guard
- get next stage by position

**Step 3: Regenerate sqlc**
Run:
```bash
make sqlc
```
Expected:
- generated files updated without error

**Step 4: Run handler tests again**
Run:
```bash
cd server && go test ./internal/handler -run 'TestPipeline|TestStageRouting' -count=1
```
Expected:
- still FAIL, but now due to missing handlers/services rather than missing generated code

**Step 5: Commit**
```bash
git add server/pkg/db/queries/pipeline.sql server/pkg/db/queries/issue_pipeline.sql server/pkg/db/generated
git commit -m "feat: add sqlc queries for pipelines and issue routing"
```

---

### Task 4: Implement pipeline handlers and wire routes

**Objective:** Expose workspace-level pipeline CRUD via API.

**Files:**
- Create: `server/internal/handler/pipeline.go`
- Modify: `server/cmd/server/router.go`
- Modify: `server/internal/handler/handler.go` only if constructor dependencies need to expand

**Step 1: Implement API request/response types**
Add request/response structs for:
- pipeline
- pipeline stage
- create/update payloads

**Step 2: Implement handlers**
Add handlers for:
- `GET /api/pipelines`
- `POST /api/pipelines`
- `GET /api/pipelines/{id}`
- `PUT /api/pipelines/{id}`
- `DELETE /api/pipelines/{id}`

**Step 3: Wire routes in router**
Modify:
- `server/cmd/server/router.go`

Put them under workspace-member scope.

**Step 4: Run targeted tests and make them pass**
Run:
```bash
cd server && go test ./internal/handler -run 'TestPipeline' -count=1
```
Expected:
- PASS for CRUD tests

**Step 5: Commit**
```bash
git add server/internal/handler/pipeline.go server/cmd/server/router.go
git commit -m "feat: add pipeline management API"
```

---

### Task 5: Add a focused pipeline service and explicit enqueue helper

**Objective:** Keep routing policy out of the generic issue handler and avoid hacking `TaskService.EnqueueTaskForIssue` for stage-to-stage handoff.

**Files:**
- Create: `server/internal/service/pipeline.go`
- Modify: `server/internal/service/task.go`
- Modify: `server/internal/handler/handler.go`

**Step 1: Add explicit task enqueue helper**
In `server/internal/service/task.go`, add a method like:
- `EnqueueTaskForAgentIssue(ctx, issue, agentID, triggerCommentID...)`

It should:
- load the specified agent
- validate runtime/archive state
- create `agent_task_queue` row for that agent + issue
- reuse the same queue semantics as current issue assignment flow

**Step 2: Add pipeline service skeleton**
In `server/internal/service/pipeline.go`, add a service with methods like:
- `ResolveEffectivePipelineForIssue(...)`
- `MaybeAdvanceIssuePipeline(...)`

**Step 3: Add service injection in handler constructor**
Create the service once in handler bootstrap, similar to `TaskService` / `AutopilotService`.

**Step 4: Write/extend failing service tests if needed**
Create if useful:
- `server/internal/service/pipeline_test.go`

**Step 5: Run targeted tests**
Run:
```bash
cd server && go test ./internal/service -run 'TestPipeline' -count=1
```
Expected:
- RED first, then GREEN after implementation

**Step 6: Commit**
```bash
git add server/internal/service/pipeline.go server/internal/service/task.go server/internal/handler/handler.go
git commit -m "feat: add pipeline service and explicit stage enqueue helper"
```

---

### Task 6: Hook issue status changes into stage routing

**Objective:** Make real issue lifecycle changes advance the pipeline and enqueue the next agent.

**Files:**
- Modify: `server/internal/handler/issue.go`
- Modify: `server/internal/service/pipeline.go`
- Modify: `server/internal/handler/pipeline_test.go`

**Step 1: Restrict routing trigger to authoritative issue updates**
Only fire routing when:
- issue status actually changed
- effective pipeline exists
- current stage is complete and next stage exists

**Step 2: Advance issue pipeline atomically**
Use optimistic guard / sequence increment to avoid duplicate next-stage enqueue.

**Step 3: Keep current issue status unchanged**
Important rule:
- the stage hook queues the next agent
- it does not overwrite the status just set by the actor

**Step 4: Record useful activity evidence**
If practical in this slice, add activity entries like:
- `pipeline_stage_advanced`
- `pipeline_stage_enqueued`

**Step 5: Run targeted tests**
Run:
```bash
cd server && go test ./internal/handler -run 'TestStageRouting' -count=1
cd server && go test ./internal/service -run 'TestPipeline' -count=1
```
Expected:
- PASS

**Step 6: Commit**
```bash
git add server/internal/handler/issue.go server/internal/service/pipeline.go server/internal/handler/pipeline_test.go
 git commit -m "feat: route pipeline stages from issue status changes"
```

---

## Milestone B — Explicit project control settings + blocked escalation

### Task 7: Add project control schema and sqlc queries

**Objective:** Create an explicit control-plane config layer without changing Project Lead semantics.

**Files:**
- Create: `server/migrations/050_project_control_settings.up.sql`
- Create: `server/migrations/050_project_control_settings.down.sql`
- Create: `server/pkg/db/queries/project_control.sql`
- Modify (generated): `server/pkg/db/generated/*.go`

**Step 1: Add `project_control_settings`**
Columns should include at minimum:
- `project_id`
- `overseer_agent_id`
- `default_pipeline_id`
- `automation_mode`
- `auto_triage_enabled`
- `auto_route_enabled`
- `auto_escalate_blocked`
- `stale_after_minutes`
- optional `review_policy`
- optional `quality_policy`

**Step 2: Add sqlc queries**
Include:
- get project control settings
- upsert project control settings
- list projects with automation enabled if needed

**Step 3: Regenerate sqlc and run migrations**
Run:
```bash
make sqlc
make migrate-up
```
Expected:
- success

**Step 4: Commit**
```bash
git add server/migrations/050_project_control_settings.* server/pkg/db/queries/project_control.sql server/pkg/db/generated
git commit -m "feat: add explicit project control settings schema"
```

---

### Task 8: Add backend API for project control settings

**Objective:** Make control settings explicit and fetchable without bloating base project CRUD semantics.

**Files:**
- Create: `server/internal/handler/project_control.go`
- Modify: `server/cmd/server/router.go`
- Create: `server/internal/handler/project_control_test.go`

**Step 1: Add failing handler tests**
Cover:
- get control settings for a project
- upsert control settings
- reject overseer agent outside workspace
- reject default pipeline outside workspace

**Step 2: Implement handlers**
Add routes:
- `GET /api/projects/{id}/control`
- `PUT /api/projects/{id}/control`

**Step 3: Wire routes**
Modify:
- `server/cmd/server/router.go`

**Step 4: Run targeted tests**
Run:
```bash
cd server && go test ./internal/handler -run 'TestProjectControl' -count=1
```
Expected:
- PASS

**Step 5: Commit**
```bash
git add server/internal/handler/project_control.go server/internal/handler/project_control_test.go server/cmd/server/router.go
git commit -m "feat: add project control settings API"
```

---

### Task 9: Add Overseer service and blocked-issue escalation first

**Objective:** Deliver the smallest valuable Overseer slice before tackling stale reconciliation.

**Files:**
- Create: `server/internal/service/overseer.go`
- Create: `server/internal/service/overseer_test.go`
- Modify: `server/internal/handler/handler.go`
- Modify: `server/internal/handler/issue.go`
- Modify: `server/pkg/db/queries/activity.sql` only if helper query additions are needed

**Step 1: Write failing service tests**
Cover:
- blocked issue in project with `auto_escalate_blocked = true` creates an Overseer task
- does not enqueue when project has no Overseer configured
- does not duplicate when pending/running Overseer task already exists for same issue + agent

**Step 2: Implement `OverseerService`**
Responsibilities:
- load project control settings
- decide whether to escalate
- enqueue Overseer agent task via explicit enqueue helper
- write activity log evidence

**Step 3: Hook blocked transition from issue update flow**
In `server/internal/handler/issue.go`, after authoritative status update:
- if issue becomes `blocked`, call Overseer service

**Step 4: Record activity log**
Use existing `CreateActivity` query with actions like:
- `overseer_requested_for_blocked_issue`

**Step 5: Run targeted tests**
Run:
```bash
cd server && go test ./internal/service -run 'TestOverseer' -count=1
cd server && go test ./internal/handler -run 'TestProjectControl|TestBlockedEscalation' -count=1
```
Expected:
- PASS

**Step 6: Commit**
```bash
git add server/internal/service/overseer.go server/internal/service/overseer_test.go server/internal/handler/issue.go server/internal/handler/handler.go
 git commit -m "feat: add Overseer blocked-issue escalation"
```

---

## Milestone C — Stale escalation and frontend UI

### Task 10: Add stale reconciliation path

**Objective:** Give Overseer O1 a real recovery path for silent/stuck execution.

**Files:**
- Modify: `server/internal/service/overseer.go`
- Create: `server/internal/handler/project_control_reconcile.go` or extend `project_control.go`
- Create: `server/internal/handler/project_control_reconcile_test.go`
- Modify: `server/cmd/server/router.go`

**Step 1: Define stale rule**
Use the first practical rule:
- latest queued/dispatched/running task for an issue is older than `stale_after_minutes`
- issue is not `done` / `cancelled`
- project has Overseer configured

**Step 2: Add reconcile entrypoint**
Prefer a project-scoped route first:
- `POST /api/projects/{id}/control/reconcile`

This keeps scope small and testable.

**Step 3: Escalate stale issues through Overseer service**
Record activity action like:
- `overseer_requested_for_stale_issue`

**Step 4: Run targeted tests**
Run:
```bash
cd server && go test ./internal/handler -run 'TestProjectControlReconcile|TestStaleEscalation' -count=1
cd server && go test ./internal/service -run 'TestOverseer' -count=1
```
Expected:
- PASS

**Step 5: Commit**
```bash
git add server/internal/service/overseer.go server/internal/handler/project_control_reconcile.go server/internal/handler/project_control_reconcile_test.go server/cmd/server/router.go
git commit -m "feat: add stale issue reconciliation for Overseer"
```

---

### Task 11: Add frontend pipeline types, client methods, and settings UI

**Objective:** Expose pipeline CRUD in the current shared UI architecture without creating a new route system.

**Files:**
- Create: `packages/core/types/pipeline.ts`
- Modify: `packages/core/types/index.ts`
- Modify: `packages/core/api/client.ts`
- Create: `packages/core/pipelines/queries.ts`
- Create: `packages/core/pipelines/mutations.ts`
- Create: `packages/core/pipelines/index.ts`
- Create: `packages/views/settings/components/pipelines-tab.tsx`
- Create: `packages/views/settings/components/pipelines-tab.test.tsx`
- Modify: `packages/views/settings/components/settings-page.tsx`

**Step 1: Write failing UI test for Pipelines tab**
Cover:
- tab renders in Settings page
- existing pipelines list renders
- create/update actions call mutations

**Step 2: Add core types and API methods**
Client methods:
- `listPipelines()`
- `getPipeline(id)`
- `createPipeline(data)`
- `updatePipeline(id, data)`
- `deletePipeline(id)`

**Step 3: Add React Query wrappers**
Create query/mutation helpers under `packages/core/pipelines/`.

**Step 4: Add Pipelines tab to existing settings architecture**
Modify `settings-page.tsx` to include a new workspace tab:
- `pipelines`

**Step 5: Implement basic Pipelines tab UI**
First slice UI can be simple but complete:
- list pipelines
- create pipeline name/description
- edit stages in a structured form
- reorder stages numerically first if drag-and-drop is too much for slice one

**Step 6: Run targeted frontend tests**
Run:
```bash
pnpm --filter @multica/views exec vitest run settings/components/pipelines-tab.test.tsx
```
Expected:
- PASS

**Step 7: Commit**
```bash
git add packages/core/types/pipeline.ts packages/core/types/index.ts packages/core/api/client.ts packages/core/pipelines packages/views/settings/components/pipelines-tab.tsx packages/views/settings/components/pipelines-tab.test.tsx packages/views/settings/components/settings-page.tsx
git commit -m "feat: add pipeline management UI in settings"
```

---

### Task 12: Add project automation UI in Project Detail

**Objective:** Make Overseer/project control settings explicit where operators already manage project metadata.

**Files:**
- Create: `packages/core/types/project-control.ts`
- Modify: `packages/core/types/index.ts`
- Modify: `packages/core/api/client.ts`
- Create: `packages/core/project-control/queries.ts`
- Create: `packages/core/project-control/mutations.ts`
- Create: `packages/core/project-control/index.ts`
- Modify: `packages/views/projects/components/project-detail.tsx`
- Extend or create: `packages/views/projects/components/project-detail.test.tsx`

**Step 1: Write failing Project Detail test**
Cover:
- automation section renders
- Overseer selector renders
- default pipeline selector renders
- toggles save through mutation

**Step 2: Add core types and client methods**
Methods:
- `getProjectControl(id)`
- `updateProjectControl(id, data)`
- optional `reconcileProjectControl(id)`

**Step 3: Add React Query wrappers**
Create `project-control` query/mutation helpers.

**Step 4: Add Automation section to `ProjectDetail`**
Show at minimum:
- Overseer
- automation mode
- default pipeline
- auto-escalate blocked
- stale threshold
- reconcile button if stale reconciliation route was implemented

**Step 5: Run targeted frontend tests**
Run:
```bash
pnpm --filter @multica/views exec vitest run projects/components/project-detail.test.tsx
```
Expected:
- PASS

**Step 6: Commit**
```bash
git add packages/core/types/project-control.ts packages/core/types/index.ts packages/core/api/client.ts packages/core/project-control packages/views/projects/components/project-detail.tsx packages/views/projects/components/project-detail.test.tsx
git commit -m "feat: add project automation controls to project detail"
```

---

### Task 13: End-to-end verification and cleanup

**Objective:** Prove the whole slice works and document any follow-up gaps.

**Files:**
- Modify if needed: `PLAN.md`
- Modify if needed: `docs/architecture/fork-delta-map.md` (create only if implementation spans enough upstream-sensitive files)

**Step 1: Run backend verification**
Run:
```bash
cd server && go test ./...
```
Expected:
- PASS

**Step 2: Run frontend verification**
Run:
```bash
pnpm typecheck
pnpm test
```
Expected:
- PASS

**Step 3: Run full project verification**
Run:
```bash
make check
```
Expected:
- PASS

**Step 4: Manual verification checklist**
Verify manually:
1. create pipeline in Settings
2. attach pipeline + Overseer to a project in Project Detail
3. create issue under project and link to pipeline if needed
4. move/complete issue through one stage and confirm next agent gets queued
5. move issue to `blocked` and confirm Overseer task gets queued
6. run reconcile for stale logic and confirm Overseer task gets queued when conditions match
7. confirm activity evidence exists for pipeline/overseer actions

**Step 5: Final commit**
```bash
git add -A
git commit -m "feat: implement stage routing and Overseer O1 control plane"
```

---

## Review gates

### Spec compliance gate
The implementation is only acceptable if all of these are true:
- Project Lead semantics remain unchanged
- stage routing uses explicit pipeline tables
- project control is explicit, not implicit
- blocked escalation works through Overseer
- stale reconciliation is explicit and auditable
- settings UI uses existing `/settings` tab architecture
- project automation UI lives in Project Detail

### Code quality gate
Reject and fix if any of these appear:
- routing logic duplicated across handlers
- UI-only state pretending to be server truth
- daemon changes introduced for no immediate need
- broad invasive edits to unrelated upstream code
- hidden semantics encoded into `project.lead_*`

---

## Recommended execution order for Hermes

When using Hermes to implement this plan, execute in this exact order:
1. Tasks 1–6 (pipeline backbone)
2. Tasks 7–9 (project control + blocked escalation)
3. Task 10 (stale reconciliation)
4. Tasks 11–12 (frontend)
5. Task 13 (full validation)

Do **not** start frontend before backend contracts stabilize.
Do **not** start stale reconciliation before blocked escalation is working.

---

## Best-practice Hermes execution prompt

Use this exact pattern when you want implementation instead of more planning:

```text
Implement docs/plans/2026-04-19-elite-mission-control-stage-routing-overseer-o1.md using subagent-driven-development and strict TDD.

Rules:
- Do not re-plan.
- Start with Task 1 only.
- Make code changes, run the required tests, and report concrete pass/fail results.
- After each task, do spec review first, then code-quality review.
- Do not continue to the next task until the current task is green.
```

If you want a fresh execution-only session, use the same prompt in a new Hermes chat and point it to this exact file.

---

## Final note

This plan intentionally favors:
- additive backend seams
- explicit control-plane config
- existing UI architecture reuse
- phased delivery with validation after each slice

That is the safest path to real Mission Control capability without making future upstream sync miserable.
