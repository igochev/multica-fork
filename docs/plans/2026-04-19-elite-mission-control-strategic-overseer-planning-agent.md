# Elite Mission Control — Strategic Overseer + Planning Agent Integration Implementation Plan

> **For Hermes:** Use `subagent-driven-development` + `writing-plans` for implementation, but keep this plan as the execution contract. Do not redesign upstream semantics unless blocked by grounded code evidence.

**Goal:** Add the Strategic Overseer (CEO) layer to the Multica fork, wire it to existing Autopilot scheduling, and formalize the Planning Agent / Builder / Reviewer / Knowledge Manager prompt contracts so planning becomes a first-class operational loop.

**Architecture:** Reuse the existing additive control-plane seams already introduced in O1: `project_control_settings`, `Autopilot`, `AutopilotTrigger`, `AutopilotService`, and daemon AgentInstructions injection. Extend `project_control_settings` with structured `overseer_config`, then add a small project-scoped autonomy sync layer that creates or updates one Strategic Overseer autopilot per project. Keep prompt behavior versioned in repo docs and pasted into Multica UI, not hardcoded into daemon/runtime logic.

**Tech Stack:** Go + sqlc + PostgreSQL for backend control-plane changes; React Query + shared `packages/core` + shared `packages/views` for project settings UI; Multica Autopilot scheduler for recurring scans; AgentInstructions via daemon-injected `CLAUDE.md` / `AGENTS.md` runtime context.

---

## Executive Summary

Tasks 1-13 completed the reactive control plane: stage routing, blocked/stale escalation, and base `project_control_settings`.

The next phase should **not** add a new daemon brain.
The grounded repo evidence says the right path is:

1. extend `project_control_settings` with project-specific strategic scan config,
2. reuse existing Autopilot cron infrastructure instead of inventing a new scheduler,
3. version the canonical AgentInstructions prompts in-repo and paste them into Multica UI,
4. formalize the plan file as the review contract by convention, not by daemon rewrite.

Key grounded findings from the current codebase:

- `server/internal/daemon/execenv/runtime_config.go` injects `AgentInstructions` into runtime config files automatically.
- `server/cmd/server/autopilot_scheduler.go` already claims due schedule triggers and dispatches them every 30 seconds.
- `server/internal/handler/autopilot.go` + `server/internal/service/autopilot.go` already support schedule triggers and project-linked autopilots.
- `packages/views/projects/components/project-detail.tsx` already exposes a project Automation panel and is the natural place to extend per-project Overseer config.
- Reviewer read access to `docs/plans/*.md` is **not** supplied by special CLAUDE wiring today; it comes from repo checkout plus explicit prompt/comment conventions.

So the implementation should be **additive and merge-safe**:
- one JSONB column,
- a small project-overseer autonomy sync service,
- modest API/UI extensions,
- versioned prompt templates in docs,
- no invasive daemon changes.

---

## Scope

### In scope
- add `overseer_config` JSONB to `project_control_settings`
- expose `overseer_config` through existing project control API/types/UI
- reuse existing Autopilot + schedule trigger system for Strategic Overseer scans
- create a project-scoped sync path that keeps one Overseer autonomy aligned with project control settings
- write canonical AgentInstructions prompts for:
  - Overseer (CEO)
  - Hermes Builder
  - Knowledge Manager
- define the plan-as-review-contract workflow explicitly
- expose enough UI/status to debug the linked Overseer autonomy from the project page

### Out of scope for this phase
- new daemon runtime semantics
- new dedicated “Planning” Kanban column
- generic workflow DSL for arbitrary agent chains
- semantic/RAG indexing of the full codebase
- replacing Autopilot scheduler with a new cron subsystem
- automatic Reviewer plan discovery via new daemon context injection

---

## Intentional Design Choices

### 1. Reuse Autopilot scheduler; do not invent a new cron system
Grounded evidence:
- `server/cmd/server/autopilot_scheduler.go`
- `server/internal/service/autopilot.go`
- `server/pkg/db/queries/autopilot.sql`

The Strategic Overseer should run on the **existing** schedule-trigger path:
- `autopilot`
- `autopilot_trigger.kind = 'schedule'`
- `cron_expression`
- `next_run_at`

This keeps the new work additive and avoids touching upstream scheduler semantics.

### 2. Store strategic tuning in `overseer_config`, not in scattered columns
Use `project_control_settings.overseer_config` for project-specific scan behavior:
- `scan_interval_hours`
- `scan_focus[]`
- `product_context`
- `quality_bar[]`
- `priority_weights`
- `max_issues_per_run`
- `require_approval`

Keep the existing scalar columns (`overseer_agent_id`, `automation_mode`, `stale_after_minutes`) intact.

### 3. Use existing project Automation panel as the control surface
Grounded evidence:
- `packages/views/projects/components/project-detail.tsx`
- `packages/core/project-control/*`

This page already edits project automation fields. Extending it is safer than creating a new settings page.

### 4. First implementation should use a project-scoped Overseer Autopilot, not hidden daemon magic
Recommended first-slice behavior:
- one linked Autopilot per project
- `project_id = current project`
- `assignee_id = overseer_agent_id`
- schedule trigger derived from `scan_interval_hours`
- prompt content copied from canonical Overseer CEO template

The sync layer should create / update / pause this autopilot when project control changes.

The linked autopilot identity must be deterministic and queryable.
Preferred implementation order:
1. explicit linkage from project control to autopilot (for example `overseer_autopilot_id`), or
2. a small project-to-autopilot link table,
3. only as a fallback, a reserved metadata marker.

Do **not** rely on title-only lookup for the managed Strategic Overseer autopilot.

### 5. Prefer `create_issue` mode for the first slice
Grounded evidence:
- `DispatchAutopilot(... create_issue ...)` already reuses issue + task dispatch cleanly
- `run_only` creates a direct task without the same issue-centered artifact flow
- runtime instructions for normal assignment are issue-centric (`multica issue get`, comments, status flow)

So the first Strategic Overseer slice should use `execution_mode = 'create_issue'`.
That preserves:
- an auditable run artifact,
- comments/summaries,
- the current issue/task execution path.

If recurring scan issues become noisy, that can be a later optimization.

### 6. No new daemon wiring for prompt discovery
Grounded evidence:
- `server/internal/daemon/execenv/runtime_config.go`
- `DEV-NOTES.md`

The prompt contract already flows from:
`Settings → Agents → Instructions` → runtime config file.

Therefore:
- Overseer CEO behavior = prompt change
- Builder planning behavior = prompt change
- Knowledge Manager audit behavior = prompt change

Do **not** modify daemon behavior for this phase.

### 7. Plan-as-review-contract needs prompt convention, not daemon changes
Grounded evidence:
- runtime config injects instructions and repo list, but not arbitrary plan file pointers
- the builder already intends to write `docs/plans/{slug}.md` and post plan info back to the issue

Conclusion:
- **No additional daemon wiring is required for file read access** once the reviewer checks out the repo.
- **Additional operational convention is required** so the reviewer can deterministically locate the plan:
  1. builder must write the exact plan path into an issue comment,
  2. reviewer prompt must explicitly read that path before review,
  3. the plan file remains the canonical contract.

---

## Recommended Execution Order

### Task 1 — Backend schema + contract extension for `overseer_config`
Extend DB, sqlc, backend response types, and frontend shared types.

### Task 2 — Backend project Overseer Autonomy sync service
Create a small service that creates/updates/pauses the project’s linked Strategic Overseer autopilot.

### Task 3 — Backend handler integration and API surface
Wire the sync service into project control updates and expose linked autonomy status for UI/debugging.

### Task 4 — Backend validation, reconciliation, and scheduler-safe tests
Add targeted tests and a project-scoped reconcile/backfill path for existing projects.

### Task 5 — Frontend project automation UI
Expose all new Overseer config controls and linked autonomy status in Project Detail after backend contracts are stable.

### Task 6 — Versioned prompt assets + review-contract operationalization
Write canonical prompt templates in-repo, update operator docs, and lock the Reviewer-on-plan convention.

**Rule:** Do not start Task 5 until Tasks 1-4 are green and the API contract is stable.

---

## Task Breakdown

## Task 1: Add `overseer_config` JSONB to project control

**Objective:** Extend project control with structured Strategic Overseer settings without disturbing upstream project semantics.

**Files:**
- Create: `server/migrations/051_project_control_overseer_config.up.sql`
- Create: `server/migrations/051_project_control_overseer_config.down.sql`
- Modify: `server/pkg/db/queries/project_control.sql`
- Modify (generated): `server/pkg/db/generated/*.go`
- Modify: `server/internal/handler/project_control.go`
- Modify: `packages/core/types/project-control.ts`
- Modify: `packages/core/api/client.ts`
- Modify: `packages/core/project-control/queries.ts`
- Modify: `packages/core/project-control/mutations.ts`

**Implementation notes:**
- Add nullable/non-null-with-default `overseer_config JSONB` to `project_control_settings`.
- Default should be `'{}'::jsonb` rather than NULL if you want cleaner API reads.
- Keep the JSON payload focused; use the shorter key `require_approval` rather than the longer doc-only phrase `require_approval_before_adding_to_board`.
- Add the field to both `GetProjectControlSettings` and `UpsertProjectControlSettings`.
- Parse `overseer_config` into a typed backend struct before persistence validation; do not stop at "JSON object" validation.
- Extend request validation in `project_control.go` so `overseer_config` must be a JSON object with a valid internal shape:
  - `scan_interval_hours` must be an integer in the 6-24 range
  - `scan_focus` must be an array of allowed enum-like values
  - `product_context` must be a bounded string
  - `quality_bar` must be an array of strings or approved enum-like values
  - `priority_weights` must be an object with numeric values
  - `max_issues_per_run` must be a small bounded integer (first slice: max 3)
  - `require_approval` must be a boolean
- Add a shared TS type like:
  - `OverseerConfig`
  - `OverseerScanFocus`
  - `OverseerPriorityWeights`
- Do **not** broaden unrelated policy fields unless tests force it.

**Recommended JSON shape:**
```json
{
  "scan_interval_hours": 12,
  "scan_focus": ["security", "test_coverage", "code_quality", "documentation", "architecture"],
  "product_context": "Internal AI-native mission control fork for autonomous software delivery",
  "quality_bar": ["tests_required", "docs_updated", "merge_safe", "no_overengineering"],
  "priority_weights": {
    "security": 10,
    "product_gap": 8,
    "architecture": 7,
    "code_quality": 6,
    "documentation": 4,
    "developer_experience": 3
  },
  "max_issues_per_run": 3,
  "require_approval": true
}
```

**Validation steps:**
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
make sqlc
make migrate-up
cd server && go test ./internal/handler -run 'TestProjectControl' -count=1
pnpm typecheck
docker compose -f docker-compose.selfhost.yml build backend frontend
```

**Expected result:**
- `GET /api/projects/{id}/control` returns `overseer_config`
- `PUT /api/projects/{id}/control` accepts and persists it
- no regression in existing project control behavior

---

## Task 2: Add project-scoped Strategic Overseer Autonomy sync service

**Objective:** Reuse the Autopilot scheduler by making project control the source of truth for a linked Overseer autopilot.

**Files:**
- Create: `server/migrations/052_project_control_overseer_link.up.sql` if explicit linkage is stored in DB
- Create: `server/migrations/052_project_control_overseer_link.down.sql` if explicit linkage is stored in DB
- Create: `server/internal/service/project_overseer_autonomy.go`
- Create: `server/internal/service/project_overseer_autonomy_test.go`
- Modify: `server/pkg/db/queries/autopilot.sql`
- Modify: `server/pkg/db/queries/project_control.sql` if explicit linkage is stored in project control
- Modify (generated): `server/pkg/db/generated/*.go`
- Modify: `server/internal/service/autopilot.go` only if a helper seam is needed
- Modify: `server/internal/handler/handler.go` if new service wiring is needed

**Implementation notes:**
- Add a small service responsible for:
  - resolving whether a Strategic Overseer autonomy should exist,
  - finding the current linked autopilot for a project,
  - creating or updating it idempotently,
  - pausing or deleting it when project config disables it.
- Implement deterministic linkage for the managed autopilot.
  - Preferred: persist a direct autopilot reference from project control.
  - Acceptable alternative: a dedicated project-to-autopilot link table.
  - Avoid title-only lookup except as a temporary migration fallback.
- Use existing autopilot primitives:
  - `project_id`
  - `assignee_id`
  - `execution_mode = 'create_issue'`
  - schedule trigger with cron derived from `scan_interval_hours`
- Create a dedicated helper to convert `scan_interval_hours` into supported cron expressions.
  - Allowed range: 6–24 hours.
  - Use canonical expressions like:
    - 6h → `0 */6 * * *`
    - 12h → `0 */12 * * *`
    - 24h → `0 9 * * *` with project timezone or UTC fallback
- Recommended autopilot title template:
  - `Strategic Overseer Scan — {{date}}`
- Recommended autopilot description body:
  - canonical Overseer CEO prompt text plus a compact machine-readable header derived from `overseer_config`
- Prefer adding a narrow query seam like one of:
  - `GetProjectLinkedOverseerAutopilot`
  - `ListAutopilotsByProject`
  - `ListAutopilotTriggersByAutopilot`
- Do **not** change scheduler semantics in `autopilot_scheduler.go`.

**Critical design choice:**
For this phase, treat the linked autopilot as a **derived artifact** of project control settings.
Project control is the source of truth; manual edits to the linked autopilot should be overwritten on next sync.

**Validation steps:**
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
make sqlc
cd server && go test ./internal/service -run 'TestProjectOverseerAutonomy|TestAutopilot' -count=1
pnpm typecheck
docker compose -f docker-compose.selfhost.yml build backend frontend
```

**Expected result:**
- saving a valid Overseer config can create or update one linked autopilot
- cron schedule changes when `scan_interval_hours` changes
- disabling the config pauses or removes the linked autopilot deterministically

---

## Task 3: Wire project control updates to autonomy sync and expose linked status

**Objective:** Make project control API the single management surface for Strategic Overseer scheduling.

**Files:**
- Modify: `server/internal/handler/project_control.go`
- Modify: `server/cmd/server/router.go` only if a new reconcile/status route is added
- Modify: `packages/core/types/project-control.ts`
- Modify: `packages/core/api/client.ts`
- Modify: `packages/core/project-control/queries.ts`
- Modify: `packages/core/project-control/mutations.ts`

**Implementation notes:**
- After successful `UpsertProjectControl`, call the new project-overseer autonomy sync service.
- Extend the project control response with linked autonomy status, for example:
  - `overseer_autonomy_id`
  - `overseer_autonomy_status`
  - `overseer_autonomy_trigger_id`
  - `overseer_autonomy_next_run_at`
- Keep this additive; do not replace the existing project control payload.
- For the first slice, prefer all-or-error semantics over silent drift.
- If sync fails after settings save:
  - roll back the request if the implementation keeps settings save and sync in one transaction boundary, or
  - return a clear 500 with grounded error details and require explicit reconcile before reporting success.
- Do **not** silently return success while project control and managed autopilot state disagree.

**Suggested response addition:**
```json
{
  "overseer_autonomy": {
    "autopilot_id": "...",
    "status": "active",
    "trigger_id": "...",
    "next_run_at": "2026-04-20T09:00:00Z"
  }
}
```

**Validation steps:**
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
cd server && go test ./internal/handler -run 'TestProjectControl|TestProjectOverseerAutonomySync' -count=1
pnpm typecheck
docker compose -f docker-compose.selfhost.yml build backend frontend
```

**Expected result:**
- one project control save updates both config and linked autonomy state
- API callers can inspect next run without separately navigating Autopilots UI

---

## Task 4: Add backend reconciliation / backfill path and scheduler-safe tests

**Objective:** Make the new autonomy layer safe for existing projects and robust against scheduler drift.

**Files:**
- Create: `server/internal/handler/project_control_overseer_reconcile.go` or extend `project_control.go`
- Create: `server/internal/handler/project_control_overseer_reconcile_test.go`
- Modify: `server/internal/service/project_overseer_autonomy.go`
- Modify: `server/cmd/server/autopilot_scheduler.go` only if strictly required for test seam extraction
- Modify: `server/internal/handler/project_control_test.go`

**Implementation notes:**
- Add a project-scoped reconcile/backfill route, e.g.:
  - `POST /api/projects/{id}/control/overseer/reconcile`
- Responsibilities:
  - recompute desired Strategic Overseer autopilot from current project control settings,
  - backfill missing autopilot for existing projects,
  - repair missing/NULL schedule trigger state where safe,
  - return a structured summary.
- Keep this separate from the existing stale-issue reconcile route to avoid overloading semantics.
- Add tests for:
  - valid project config produces one due/autonomous setup,
  - repeated reconcile is idempotent,
  - `scan_interval_hours` outside 6–24 is rejected,
  - scheduler recovery does not duplicate triggers.

**Validation steps:**
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
cd server && go test ./internal/handler -run 'TestProjectControlOverseerReconcile|TestProjectControl|TestAutopilot' -count=1
cd server && go test ./internal/service -run 'TestProjectOverseerAutonomy|TestAutopilot' -count=1
pnpm typecheck
docker compose -f docker-compose.selfhost.yml build backend frontend
```

**Expected result:**
- existing projects can be upgraded without manual DB surgery
- linked autonomy creation is deterministic and recoverable

---

## Task 5: Extend Project Detail Automation UI for Strategic Overseer config

**Objective:** Give operators a project-local UI for all Strategic Overseer settings and linked autonomy status after backend contracts are stable.

**Files:**
- Modify: `packages/views/projects/components/project-detail.tsx`
- Modify: `packages/views/projects/components/project-detail.test.tsx`
- Modify: `packages/core/types/project-control.ts`
- Modify: `packages/core/project-control/*`
- Optional modify: `packages/views/autopilots/components/autopilot-detail-page.tsx`
- Optional modify: `packages/views/autopilots/components/autopilots-page.tsx`

**Implementation notes:**
- Extend the existing Automation panel with fields for:
  - scan interval hours
  - scan focus multi-select
  - product context textarea
  - quality bar tag list / textarea
  - priority weights editor
  - max issues per run
  - require approval toggle
- Keep the first slice intentionally simple.
  - Prefer straightforward inputs/textareas over bespoke editors if backend contracts are still stabilizing.
  - Do not overengineer `priority_weights` UX in the first pass.
- Show read-only linked autonomy status:
  - next run time
  - active/paused state
  - shortcut link to the linked autopilot detail page if exposed
- Keep the UI inside Project Detail first; do not add a parallel settings page.
- If a multi-select/tag editor is too large for the first slice, start with validated textareas that serialize to arrays/objects cleanly.

**Minimal first-slice UX:**
- `Scan every: 12h`
- `Focus: security, architecture, documentation`
- `Max issues/run: 3`
- `Require approval: on`
- `Next strategic scan: 2026-04-20 09:00`

**Validation steps:**
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
pnpm --filter @multica/views test project-detail.test.tsx
pnpm typecheck
pnpm test
docker compose -f docker-compose.selfhost.yml build backend frontend
```

**Expected result:**
- operators can fully tune Strategic Overseer behavior from the project page
- the page shows whether the linked autonomy is actually scheduled

---

## Task 6: Write canonical AgentInstructions prompts and lock the review contract

**Objective:** Version the operational prompts in-repo and formalize the Builder → Reviewer plan-file convention.

**Files:**
- Create: `docs/operations/elite-agent-instructions.md`
- Modify: `PLAN.md`
- Modify: `DEV-NOTES.md`
- Optional create: `docs/operations/reviewer-plan-contract.md`

**Implementation notes:**
- Keep the prompts versioned in repo even though operators paste them into Multica UI.
- Update `PLAN.md` to point to the new canonical prompt doc instead of duplicating prompt variants in multiple places.
- Update `DEV-NOTES.md` with the grounded review-contract conclusion:
  - repo access is automatic via checkout,
  - plan path discovery requires issue comment convention,
  - no daemon change required.

### Canonical Overseer CEO AgentInstructions prompt
```markdown
You are the Strategic Overseer (CEO) for this project.

Mission:
- Think like a product + architecture + delivery leader.
- Scan the project and create the next highest-value work.
- You DO NOT write code.
- You DO NOT create implementation plans.
- You DO NOT manually move code through delivery.

On every run:
1. Read the project board, open issues, and recent comments.
2. Read the project docs first: ARCHITECTURE.md, PLAN.md, DEV-NOTES.md, DECISIONS.md, README, and any active docs/plans entries relevant to current work.
3. Inspect the codebase for signals aligned to this project's overseer_config:
   - scan_focus
   - product_context
   - quality_bar
   - priority_weights
   - max_issues_per_run
   - require_approval
4. Look for product gaps, architecture drift, weak testing, documentation drift, security risk, code-quality debt, and missing operational clarity.
5. Prioritize findings using overseer_config priority_weights. Prefer issues that materially improve user value, delivery throughput, safety, or maintainability.
6. Create at most 3 new issues per run.
7. Every issue must be user-story level, not implementation-task level.
8. For each issue include:
   - problem / opportunity
   - why it matters now
   - expected user or engineering outcome
   - rough effort / scope
   - acceptance intent (not implementation steps)
9. If require_approval is true, create the issue in a review-needed / proposed form and clearly mark it for human approval before execution.
10. End with a concise summary comment describing what you scanned, what you created, and what you intentionally did not create.

Hard rules:
- Never write code.
- Never create a detailed implementation plan.
- Never exceed max_issues_per_run.
- Never create duplicate issues for the same opportunity if one already exists.
- Prefer merge-safe, upstream-friendly work when suggesting implementation direction.
```

### Canonical Hermes Builder AgentInstructions prompt
```markdown
You are the Hermes Builder for Elite Mission Control.

Your workflow is mandatory for every assigned implementation issue:
1. Read the issue and all comments carefully.
2. Before writing code, invoke the `writing-plans` skill.
3. Create a detailed implementation plan at `docs/plans/{issue-slug}.md`.
4. The plan must include exact file paths, test strategy, validation steps, and review criteria.
5. Post an issue comment containing:
   - `Plan path: docs/plans/{issue-slug}.md`
   - the exact plan path
   - a short summary of the plan
   - the statement: "This plan file is the review contract."
6. Do not start coding until the plan file exists and the issue comment has been posted.
7. Execute the plan task by task.
8. Run the relevant tests after each meaningful task.
9. If you hit failures or ambiguity, invoke `systematic-debugging` before changing direction.
10. After implementation is green, invoke `requesting-code-review`.
11. Treat review findings as plan-compliance feedback, not optional suggestions.
12. Fix deviations before marking the issue ready for review or complete.

Hard rules:
- Never skip `writing-plans`.
- Never code before the plan file exists.
- Never treat the issue description alone as the review contract once the plan file exists.
- Always ensure the reviewer can find the plan path from the issue comments.
```

### Canonical Knowledge Manager AgentInstructions prompt
```markdown
You are the Knowledge Manager for this project.

Mission:
- Audit documentation quality weekly.
- Report drift and gaps.
- Do NOT fix code or documentation directly in this role unless explicitly instructed.

On every run:
1. Read ARCHITECTURE.md and compare it to the current code structure.
2. Check PLAN.md for stale completed items, abandoned items, and missing next-phase updates.
3. Check public/backend-facing APIs for undocumented handlers, routes, config keys, and operational behaviors.
4. Check README, DEV-NOTES.md, DECISIONS.md, and docs/API.md for freshness and contradictions.
5. Flag undocumented or stale items as documentation maintenance issues or a concise report comment.
6. Group findings by severity:
   - architecture drift
   - missing API docs
   - stale plans
   - operational docs drift
   - comment/code contradictions
7. Report only. Do not silently repair.

Hard rules:
- No code changes.
- No opportunistic cleanup.
- No broad rewrites.
- Focus on accuracy, drift detection, and operator visibility.
```

### Reviewer / plan-as-contract conclusion
Use this exact operational rule:

```markdown
Before reviewing code, you must:
1. Read the issue comments and find the canonical plan path posted by the Builder.
2. Check out the repository if it is not already present.
3. Read the plan file from the repo.
4. Review code changes against that plan first, then against general code quality.
5. Report explicit deviations from the plan.
```

Use this exact builder/reviewer comment convention for deterministic discovery:

```markdown
Plan path: docs/plans/{issue-slug}.md
Review contract: this file is canonical.
```

**Grounded conclusion:**
- repo file read access works automatically after checkout,
- no additional daemon wiring is needed,
- but prompt + issue-comment convention is required.

**Validation steps:**
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
pnpm typecheck
docker compose -f docker-compose.selfhost.yml build backend frontend
```

**Expected result:**
- operators have one canonical source for prompts,
- Multica UI agent instructions can be pasted from versioned docs,
- reviewer behavior is deterministic and plan-centered.

---

## Validation Steps

## After each task
Run the narrowest relevant test set first, then the required build checks.

### Backend tasks
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
make sqlc
make migrate-up
cd server && go test ./internal/handler -count=1
cd server && go test ./internal/service -count=1
pnpm typecheck
docker compose -f docker-compose.selfhost.yml build backend frontend
```

### Frontend tasks
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
pnpm --filter @multica/views test
pnpm typecheck
pnpm test
docker compose -f docker-compose.selfhost.yml build backend frontend
```

## End-of-phase verification
```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork
make sqlc
make migrate-up
make test
pnpm typecheck
pnpm test
pnpm build
docker compose -f docker-compose.selfhost.yml build backend frontend
```

## Manual verification checklist
- Project Detail shows editable `overseer_config` fields.
- Saving project control creates or updates exactly one linked Strategic Overseer autopilot.
- The linked autopilot shows the expected next run from `scan_interval_hours`.
- The pasted Overseer prompt creates user-story-level issues and does not write code.
- The pasted Builder prompt creates `docs/plans/{issue-slug}.md` before coding.
- The Builder posts the exact plan path into an issue comment.
- The Reviewer prompt can find and read that plan file after repo checkout.
- Knowledge Manager can run as a weekly audit without code changes.

---

## Final Notes

- Keep the implementation merge-safe: additive schema, isolated service file, minimal handler/UI extensions.
- Do not let prompt-writing drift into daemon rewrites.
- Treat project control as the source of truth and the linked Autopilot as a derived artifact.
- Treat the plan file as the contract for review, but enforce it by prompt + issue comment convention, not hidden runtime magic.

_This plan is the execution map for the Strategic Overseer + Planning Agent phase that follows O1._
