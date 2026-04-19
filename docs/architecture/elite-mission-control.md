# Elite Mission Control Constitution for the Multica Fork

> Status: adopted architecture direction for the fork  
> Scope: `~/.openclaw/workspace-coding/projects/multica-fork`  
> Last updated: 2026-04-19

---

## 1. Mission

This fork exists to evolve Multica into an **Elite Mission Control** for serious AI-assisted software delivery:

- highly automated where automation improves throughput and quality
- robust under real project pressure
- production-ready in operational behavior, not just demos
- optimized for best engineering outcomes, not maximum novelty
- able to absorb upstream Multica improvements without losing our control-plane vision

We are **not** continuing the old Mission Control app as the primary path. The Multica fork is now the main platform.

---

## 2. Non-negotiable principles

### 2.1 Quality over motion
Automation is only good if it raises delivery quality. The platform must prefer:
- correct routing over clever routing
- auditable state over hidden magic
- explicit control over heuristic guesswork
- recovery paths over optimistic assumptions

### 2.2 Merge-safe by default
Where practical, fork work must be designed so future upstream sync remains cheap.

Preferred change style:
- add tables rather than mutate core tables heavily
- add services/hooks rather than rewrite existing flows end-to-end
- add settings and feature flags rather than change defaults globally
- keep control-plane behavior isolated in explicit modules

Avoid when possible:
- overloading existing generic fields with hidden semantics
- daemon rewrites for low-value gains
- large invasive changes across unrelated upstream files
- “temporary” compatibility layers that become permanent debt

### 2.3 No overengineering
Every abstraction must earn its existence.

We do **not** build:
- generic orchestration engines before we need them
- speculative plugin systems
- multi-provider policy frameworks before first real workflows succeed
- heavy config surfaces for behavior we have not validated on actual projects

### 2.4 Human override is mandatory
Elite autonomy does **not** mean no human control.
The system must always support:
- manual reassignment
- manual reroute
- pause/cancel/retry
- inspection of why routing happened
- recovery from stale or failed execution

### 2.5 Production readiness means operational resilience
A feature is not complete when it “works once.” It is complete when it:
- handles retries safely
- avoids duplicate dispatch
- records enough evidence to debug failures
- degrades safely when agents, network, or services fail
- supports reconciliation after missed events or partial outages

---

## 3. Product position

The target product is:

> **A robust Kanban control plane for AI-native software execution**

It should coordinate planning, execution, review, escalation, and recovery across agents, while preserving a clear, inspectable workflow model.

This means Multica should become better at:
- project-level orchestration
- stage-based work routing
- agent role specialization
- review and escalation loops
- quality gates
- stale-run detection and recovery
- event/audit visibility

It should not try to become:
- a magical black-box autopilot with no explanation
- a generic “workflow builder” for every domain
- a replacement for source control, CI, or observability tools

---

## 4. Current-state assessment

### 4.1 What Multica already does well
Multica already gives us the right foundation:
- Go backend
- PostgreSQL + pgvector
- Next.js frontend
- real-time board via WebSocket
- issue-centric task model
- daemon-driven agent execution
- multiple agent runtime integrations

### 4.2 What is currently missing for Elite Mission Control
The missing layer is the **control plane**:
- autonomous stage handoff
- explicit overseer logic
- project automation policy
- escalation and stale recovery
- stronger quality gating
- better merge-safe extension seams for Mission Control behavior

### 4.3 Important finding: Project Lead is not Overseer
Current Multica `project.lead_type` / `project.lead_id` is only lightweight project metadata.

Grounding from current codebase:
- schema: `server/migrations/034_projects.up.sql`
- project CRUD: `server/internal/handler/project.go`
- frontend project lead selection/display:
  - `packages/views/modals/create-project.tsx`
  - `packages/views/projects/components/project-detail.tsx`
  - `packages/views/projects/components/projects-page.tsx`

There is **no backend orchestration behavior** tied to Project Lead today.

Real execution currently flows through issue assignment and task enqueue:
- issue trigger logic: `server/internal/handler/issue.go`
- queue creation: `server/internal/service/task.go`

Therefore:
- **Project Lead stays ownership metadata**
- **Overseer must be an explicit control-plane concept**

---

## 5. Target operating model

We will structure the system in **three clear layers**.

### 5.1 Ownership layer
Purpose: accountability and visibility.

Examples:
- project lead
- project status/priority
- project metadata

Rules:
- lightweight
- human-facing
- no hidden automation semantics

### 5.2 Execution layer
Purpose: actual work delivery.

Examples:
- issue assignee
- stage-based pipeline routing
- agent task queue
- builder/reviewer execution

Rules:
- deterministic
- auditable
- driven by issue and stage state

### 5.3 Control layer
Purpose: orchestration and recovery.

Examples:
- Overseer agent
- project automation settings
- triage policy
- blocked-item escalation
- stale detection and reconciliation
- quality gate enforcement

Rules:
- explicit configuration
- isolated implementation seams
- additive to upstream architecture

---

## 6. Architectural decisions

### Decision A — Stage-based routing is Priority 1
The first major enhancement remains stage-based pipeline routing.

Why:
- it removes human handoff bottlenecks
- it fits Multica’s current issue-assignee queue model
- it can be built additively
- it creates the backbone that Overseer policy can later drive

### Decision B — OpenClaw gateway mode is low priority
We will not optimize around daemon gateway mode now.

Reason:
- likely future role split is **OpenClaw = Overseer**, **Hermes = Builder/other execution roles**
- that means per-agent OpenClaw model routing is not a critical near-term need

### Decision C — Do not overload Project Lead
Project Lead will not secretly mean “Overseer runtime.”

Reason:
- current upstream semantics are lightweight metadata only
- overloading it would create surprising behavior
- it would increase future merge pain
- it mixes ownership concerns with orchestration policy

### Decision D — Overseer becomes an explicit extension layer
We will add Overseer as a separate, named control-plane concept with explicit config and hooks.

---

## 7. Mission Control maturity roadmap

## Phase 1 — Deterministic execution backbone
Goal: make the board route work automatically.

Includes:
- pipeline tables
- stage definitions
- issue-to-pipeline association
- next-stage enqueue hook
- pipeline management UI
- stage context injection

Success criteria:
- issues move across multi-agent workflow without manual reassignment
- no duplicate stage dispatch on retries
- pipeline-free issues keep existing behavior unchanged

## Phase 2 — Explicit Overseer control plane
Goal: introduce project-level orchestration policy.

Includes:
- overseer agent config per project/pipeline
- triage and readiness policy
- blocked-item escalation policy
- review routing policy
- manual override surface

Success criteria:
- project automation is explicit and inspectable
- Project Lead remains metadata-only
- Overseer decisions are auditable

## Phase 3 — Reliability and recovery
Goal: make autonomy safe in production.

Includes:
- stale execution detection
- duplicate-dispatch guards
- reconciliation jobs / admin recovery actions
- richer audit trail and activity records
- failure-handling policy by stage

Success criteria:
- operator can understand and recover any stuck workflow
- autonomy fails safe, not silent

## Phase 4 — Quality gates and elite review
Goal: raise result quality, not just automate movement.

Includes:
- stronger Definition of Ready / Done
- review requirements by project or pipeline
- evidence expectations for builder completion
- review/escalation loops

Success criteria:
- autonomy improves shipped quality, not just speed

---

## 8. Merge-safe fork policy

All major fork work should answer these questions before implementation:

1. Can this be additive instead of invasive?
2. Can this be isolated in a new table/service/module?
3. Can upstream adopt similar capability later without catastrophic conflicts?
4. Are we depending on an existing field behaving in a way upstream never intended?
5. Can we document the fork delta in one place for future syncs?

### Preferred implementation patterns
- backend hook called from existing handler
- new DB tables with narrow foreign keys
- new config objects tied to project/pipeline
- explicit feature enablement
- event logging around automated decisions

### Patterns to avoid
- implicit behavior attached to generic metadata fields
- branching logic spread across multiple handlers with no central policy seam
- core daemon changes for speculative futures
- UI-only automation that bypasses server truth

---

## 9. Required documentation discipline

Mission Control fork work must leave behind docs that reduce future merge and maintenance cost.

Recommended permanent docs set:
- `docs/architecture/elite-mission-control.md` — this constitution
- `docs/architecture/overseer-design-spec.md` — explicit Overseer design
- `docs/architecture/fork-delta-map.md` — what differs from upstream and why
- `docs/architecture/upstream-merge-playbook.md` — repeatable sync procedure

The last two should be created as soon as the first implementation slices land.

---

## 10. Practical guardrails for implementation

### Build new behavior around current seams
Current grounded seams worth extending:
- project CRUD: `server/internal/handler/project.go`
- issue lifecycle updates: `server/internal/handler/issue.go`
- task enqueue: `server/internal/service/task.go`
- autopilot service where useful: `server/internal/service/autopilot.go`

### Keep policy separate from transport
- routing policy should not live in UI components
- project ownership should not define automation policy
- daemon transport changes should not be prerequisite for control-plane progress

### Preserve a manual-first escape hatch
Any automated path must still support:
- manual status move
- manual assignee change
- manual reroute
- manual retry
- manual cancellation

---

## 11. Executive summary

The fork strategy is:

1. **use Multica as the foundation**
2. **keep upstream mergeability in mind at every step**
3. **build stage-routing first**
4. **introduce Overseer as an explicit control-plane concept, not as Project Lead**
5. **focus on robustness, auditability, and quality rather than flashy automation**

That is the path to an Elite Mission Control without compromising clarity or maintainability.
