# Overseer Design Spec

> Status: proposed architecture for explicit Overseer support  
> Scope: Multica fork control-plane design  
> Last updated: 2026-04-19

---

## 1. Purpose

This document defines how to introduce an explicit **Overseer** role into the Multica fork without overloading existing Multica concepts and without creating unnecessary merge pain with upstream.

The Overseer is the **control-plane brain** for project execution. It is not the same as:
- Project Lead
- issue assignee
- runtime transport
- a generic autopilot for everything

The Overseer is responsible for **thinking, routing, escalating, and reconciling**, while builders/reviewers remain the **execution-plane workers**.

---

## 2. Key findings from current codebase

### 2.1 Project Lead is not sufficient
Current project lead is stored as:
- `project.lead_type`
- `project.lead_id`

Grounded locations:
- `server/migrations/034_projects.up.sql`
- `server/internal/handler/project.go`
- `packages/views/modals/create-project.tsx`
- `packages/views/projects/components/project-detail.tsx`
- `packages/views/projects/components/projects-page.tsx`

It is currently only project metadata and UI display.

### 2.2 Real execution is issue-assignee-driven
Grounded locations:
- `server/internal/handler/issue.go`
- `server/internal/service/task.go`

Current behavior:
- issues assigned to agents get queued for execution
- moving out of backlog can trigger execution
- assignee changes cancel/requeue tasks

Therefore the Overseer should **not** replace issue assignee mechanics. It should sit **above** them as orchestration policy.

---

## 3. Definition of the Overseer role

The Overseer is a designated agent that owns **project-level orchestration decisions**.

### 3.1 Responsibilities
The Overseer may:
- triage incoming work
- determine readiness for execution
- choose or attach a pipeline
- route work to Builder / Review stages
- handle blocked-item escalation
- request additional clarification or decomposition
- detect stale execution and trigger recovery
- approve progression to next stage when policy requires it

### 3.2 Non-responsibilities
The Overseer should **not**:
- directly replace the builder on normal coding work
- own every comment loop by default
- silently mutate issue state with no audit trail
- be encoded as a hidden interpretation of Project Lead
- require daemon gateway-mode changes as a prerequisite

### 3.3 Mental model
Use this model:

- **Project Lead** = who owns the project
- **Overseer** = who controls workflow decisions
- **Builder** = who executes the coding task
- **Reviewer** = who evaluates the result

One actor may fill multiple roles in small setups, but the **concepts remain separate**.

---

## 4. Design goals

1. **Explicitness** — overseer policy must be visible in config and UI
2. **Auditability** — every automated routing decision must leave evidence
3. **Additivity** — build around current issue/task seams
4. **Merge-safety** — no hidden semantic rewrites of upstream fields
5. **Progressive rollout** — minimal first slice, richer policy later
6. **Manual override** — operator can always inspect and intervene

---

## 5. Proposed architecture

## 5.1 New control-plane configuration layer
Introduce explicit project automation settings instead of attaching behavior to `project.lead_*`.

### Proposed table: `project_control_settings`

```sql
CREATE TABLE project_control_settings (
  project_id UUID PRIMARY KEY REFERENCES project(id) ON DELETE CASCADE,
  overseer_agent_id UUID REFERENCES agent(id) ON DELETE SET NULL,
  default_pipeline_id UUID REFERENCES pipelines(id) ON DELETE SET NULL,
  automation_mode TEXT NOT NULL DEFAULT 'manual'
    CHECK (automation_mode IN ('manual', 'assisted', 'autonomous')),
  auto_triage_enabled BOOLEAN NOT NULL DEFAULT false,
  auto_route_enabled BOOLEAN NOT NULL DEFAULT false,
  auto_escalate_blocked BOOLEAN NOT NULL DEFAULT false,
  stale_after_minutes INT NOT NULL DEFAULT 60,
  review_policy JSONB,
  quality_policy JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Why this is the right seam
- explicit
- additive
- project-scoped
- upstream-safe
- does not reinterpret existing project fields

---

## 5.2 Optional issue-level control state
Some Overseer behavior needs issue-scoped tracking.

### Proposed table: `issue_control_state`

```sql
CREATE TABLE issue_control_state (
  issue_id UUID PRIMARY KEY REFERENCES issue(id) ON DELETE CASCADE,
  control_status TEXT NOT NULL DEFAULT 'idle'
    CHECK (control_status IN (
      'idle',
      'triage_pending',
      'triage_complete',
      'routing_pending',
      'waiting_on_builder',
      'waiting_on_review',
      'blocked_escalated',
      'stale',
      'reconciling'
    )),
  overseer_agent_id UUID REFERENCES agent(id) ON DELETE SET NULL,
  last_overseer_task_id UUID,
  last_routed_stage_id UUID,
  last_escalated_at TIMESTAMPTZ,
  last_reconciled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

This is not required for the first slice, but it is the cleanest path if Overseer state becomes non-trivial.

---

## 5.3 Policy engine seam
Add a dedicated server-side policy seam.

### Proposed backend module
- `server/internal/service/overseer.go`

Responsibilities:
- load project control settings
- decide whether Overseer should be invoked
- create/requeue Overseer task
- record audit events
- coordinate with pipeline routing and stale detection

This service should be called from existing authoritative handlers/services rather than from the UI.

---

## 6. Integration points

## 6.1 Project creation / project settings
Extend project settings to allow:
- choosing Overseer agent
- choosing automation mode
- enabling/disabling auto-triage / auto-route / blocked escalation
- attaching default pipeline

Do **not** change `project.lead_*` semantics.

### Likely files
Backend:
- `server/internal/handler/project.go`
- new query files under `server/pkg/db/queries/`

Frontend:
- `packages/views/projects/components/project-detail.tsx`
- possibly project creation/edit modal if desired later

## 6.2 Issue creation
On issue creation, the Overseer may be invoked when:
- project has control settings
- `auto_triage_enabled = true`
- issue enters a triage-eligible state

Recommended first-slice behavior:
- create issue normally
- if project automation is enabled, enqueue Overseer task
- do not bypass current issue create flow

Current grounded seam:
- `server/internal/handler/issue.go`

## 6.3 Pipeline routing
Overseer and pipeline routing should complement each other.

Recommended role split:
- **pipeline routing** = deterministic stage progression
- **Overseer** = policy decisions around when/why to route or escalate

Examples:
- pipeline decides next stage is Review
- Overseer decides blocked issue should go to triage
- Overseer decides new issue must be decomposed before Builder stage

## 6.4 Blocked / stale / failed execution
Overseer becomes most valuable when workflows go off the happy path.

The Overseer should be able to react to:
- issue moved to blocked
- builder task failed
- no completion within stale threshold
- repeated reopen loops

This should be integrated through backend events/hooks, not UI polling hacks.

---

## 7. First implementation slice (recommended)

Keep the first Overseer slice intentionally small.

## Phase O1 — Project-scoped Overseer config + blocked/stale escalation

### Scope
1. Add `project_control_settings`
2. Allow selecting `overseer_agent_id` per project
3. Add `automation_mode`
4. Add `auto_escalate_blocked`
5. When an issue in such a project becomes `blocked`, enqueue Overseer
6. When stale detection marks an issue stale, enqueue Overseer
7. Record an audit activity describing why Overseer was triggered

### Why start here
This first slice gives high leverage without requiring the Overseer to own the entire workflow.
It solves a painful operational gap first: recovery and escalation.

### Non-goals of first slice
- no generic orchestration DSL
- no attempt to make Overseer own all new issues
- no daemon transport redesign
- no hidden Project Lead semantics

---

## 8. Phase roadmap

## Phase O1 — Escalation and recovery
- project control settings
- explicit overseer agent
- blocked escalation
- stale escalation
- activity/audit logging

## Phase O2 — Triage and readiness
- auto-triage on new issues
- readiness checks before Builder handoff
- optional decomposition request flow

## Phase O3 — Review orchestration
- route review requests to designated reviewer/oracle
- allow Overseer to reopen or send back to Builder with structured reasons
- integrate quality policy

## Phase O4 — Strategic project supervision
- monitor project-level bottlenecks
- capacity-aware escalation
- repeated-failure / repeated-block detection
- manual intervention recommendations in UI

---

## 9. Suggested UI model

### 9.1 Project settings panel
Add a dedicated **Control Plane** or **Automation** section.

Fields:
- Overseer
- Automation mode: `manual | assisted | autonomous`
- Default pipeline
- Auto-escalate blocked items
- Stale threshold
- Review policy summary

### 9.2 Issue detail
Show control-plane metadata when active:
- Overseer assigned
- current control status
- why Overseer was triggered
- last escalation / reconciliation timestamp
- manual “Send to Overseer” action

### 9.3 Board signals
Lightweight badges only, for example:
- `Escalated`
- `Stale`
- `Waiting on Overseer`

Do not clutter the board with excessive control-plane UI.

---

## 10. Audit and observability requirements

Every Overseer-triggered action should leave a trace.

Minimum evidence:
- why Overseer was invoked
- which project policy caused it
- related issue ID
- related stage / task if any
- created task ID
- timestamps

Recommended activity examples:
- `overseer_requested_for_blocked_issue`
- `overseer_requested_for_stale_issue`
- `overseer_triage_requested`
- `overseer_reroute_applied`

This is critical for operator trust.

---

## 11. Failure handling

### 11.1 If Overseer agent is missing or archived
Behavior:
- do not hard-fail issue mutation
- record warning activity / log
- surface configuration problem in project settings

### 11.2 If Overseer task enqueue fails
Behavior:
- keep business state change intact
- mark escalation attempt failure in activity/log
- allow manual retry

### 11.3 If multiple escalation triggers occur rapidly
Behavior:
- deduplicate while a pending/running Overseer task already exists for the same issue
- avoid spam-enqueue loops

---

## 12. Merge-safe implementation strategy

### Prefer
- new tables for settings/state
- a new `overseer.go` service
- thin handler hooks into authoritative flows
- UI additions in project detail/settings surfaces

### Avoid
- redefining Project Lead
- embedding Overseer policy into generic project CRUD fields
- spreading control-plane logic across many UI components
- daemon changes before control-plane behavior is validated

---

## 13. Suggested file plan

### Likely new files
- `server/internal/service/overseer.go`
- `server/internal/handler/project_control.go` or project handler extensions
- `server/pkg/db/queries/project_control.sql`
- `server/pkg/db/queries/issue_control.sql`
- migration file(s) for control tables
- UI components under project settings/control-plane area

### Existing seams likely to be touched
- `server/internal/handler/project.go`
- `server/internal/handler/issue.go`
- `server/internal/service/task.go`
- stale/reconciliation code once implemented

---

## 14. Final decision

The correct design is:

- keep **Project Lead** as ownership metadata
- introduce **Overseer** as an explicit, project-configured control-plane role
- let issue assignees and pipelines remain the primary execution mechanisms
- add Overseer first for **blocked/stale escalation**, then expand into triage/review orchestration

That gives us a clean path to elite autonomy without corrupting upstream semantics or overengineering the system.
