# Strategic Overseer — CEO Role for Elite Mission Control

> **Status:** Proposed v1 — for review before implementation  
> **Supersedes:** `overseer-design-spec.md` (reactive Overseer)  
> **Scope:** Multica fork, Elite Mission Control architecture  
> **Last updated:** 2026-04-19

---

## Executive Summary

The Overseer is not a "watchdog" or "cron for timeouts." It is the **CEO of the project** — a strategic agent that continuously monitors the project, identifies opportunities, and generates the work that the execution team should be doing.

The execution layer (Builder, Reviewer) already exists in our design. The missing piece is the **strategic layer** that creates the right work.

This spec defines:
1. What the Overseer does
2. How it creates vs. acquires work
3. How the handoff to Planning → Builder → Reviewer works
4. Per-project Overseer configuration
5. The relationship with the Knowledge Manager (Documentation Agent)
6. The RAG/content strategy that feeds both Overseer and Knowledge Manager

---

## 1. Core Definition

### 1.1 The Overseer Role

The Overseer is a **scheduled AI agent** that reads the project state and creates meaningful work items.

**Not:** a reactive alert system, a stale-detector, a monitor, or a supervisor.

**Is:** a proactive product/technical advisor that finds what the project needs.

### 1.2 Overseer vs. Existing Roles

| Role | Responsibility | Origin |
|---|---|---|
| **Overseer (CEO)** | Strategic scanning, opportunity identification, work item creation | This spec |
| **Planning Agent** | Converts user story → detailed implementation plan | Hermes `writing-plans` |
| **Builder** | Executes the plan | Hermes (default) |
| **Reviewer** | Validates builder output against the plan | Hermes `requesting-code-review` |
| **Knowledge Manager** | Keeps project documentation current and accurate | New role (weekly cron) |

### 1.3 Mental Model

```
     ┌─────────────────────────────────────────────┐
     │            PROJECT (codebase + board)        │
     └──────────┬───────────────────┬───────────────┘
                │                   │
     ┌──────────▼────────┐  ┌───────▼────────────────┐
     │   Overseer (CEO)  │  │  Knowledge Manager      │
     │   Scans + Creates │  │  Maintains Docs        │
     │   Work Items      │  │  Feeds Overseer        │
     └────────┬─────────┘  └───────┬────────────────┘
              │                    │
              ▼                    ▼
     ┌────────────────┐    ┌──────────────────────┐
     │ Issue Created │    │ Updated Docs/ARCH    │
     │ (user story   │    │ (Overseer can read   │
     │  level)        │    │  these for context)  │
     └───────┬────────┘    └──────────────────────┘
             │
     ┌───────▼─────────┐
     │ Planning Agent │  ← Hermes `writing-plans`
     │ Creates PLAN   │
     └───────┬─────────┘
             │
      ┌──────▼──────┐
      │   Builder   │  ← Hermes executes
      │   Builds    │
      └──────┬──────┘
             │
      ┌──────▼──────┐
      │  Reviewer   │  ← Hermes `requesting-code-review`
      │  Validates  │      Compares against PLAN
      └──────┬──────┘
             │
        [loop if deviations]
```

---

## 2. Overseer Responsibilities

### 2.1 Scan Scope (per trigger)

On each scheduled run, the Overseer reads:

| Source | What it looks for |
|---|---|
| **Codebase delta** (since last run) | New files, changed files, new dependencies, dead code |
| **Board state** | Items stalled in a column, backlog bloat, missing stages |
| **Existing issues** | Gaps between issue descriptions and actual implementation |
| **Documentation** | Outdated, contradictory, or missing docs |
| **Security** | New CVEs in dependencies, hardcoded secrets, unsafe patterns |
| **Test coverage** | Large files with zero tests, declining coverage |
| **Product feedback** | (if available) Feature requests, bug reports, user complaints |
| **Architecture** | Deviating from ARCHITECTURE.md, missing abstractions |

### 2.2 What It Creates

The Overseer creates issues at **user story level**, not implementation level.

**Good issue from Overseer:**
> "The `auth/` package has 3 helpers with overlapping functionality. Consolidating them into one well-tested module would reduce future bugs and speed up feature work on auth. Estimated: 2-3h. Priority: medium."

**Not a good issue from Overseer:**
> "Refactor auth/helpers.go" (too vague — let the Planning Agent decide the approach)

### 2.3 Cadence

- **Production projects:** Every 6-12 hours via Multica Autonomy
- **Stable projects:** Every 24 hours
- **Overseer can be paused** — operator discretion via project automation settings

### 2.4 What It Does NOT Do

- Does not write code
- Does not create detailed plans (Planning Agent does this)
- Does not execute tasks
- Does not manage the board manually
- Does not replace the human product owner — it suggests, the board owner approves

---

## 3. Planning Agent — The Handoff

### 3.1 The Gap

**Current Multica:** Issues (1-2 sentences) go directly to a Builder agent. The Builder decides how to approach the work.

**Our desired flow:** Issues go to the Planning Agent first → detailed plan created → Builder follows the plan.

### 3.2 Does Multica Have This?

Multica does **not** have a native Planning Agent role. The current flow is:

```
Issue Created → Builder Agent receives it → executes
```

The plan (if any) lives in the Builder's head or scratch space, not as a first-class artifact.

### 3.3 Our Implementation

For our Elite Mission Control, the handoff is:

```
Overseer creates issue (user story level)
         ↓
Issue enters board in "Inbox" or "Consider" column
         ↓
[Trigger 1: Human assigns to Builder] OR [Trigger 2: Autonomy cron assigns]
         ↓
PLANNING PHASE begins:
  → Hermes `writing-plans` skill is invoked
  → Creates: PLAN.md sub-issue or issue description update
  → Plan contains: tasks, file paths, test approach, review criteria
         ↓
[Plan is attached to the issue as description/comments]
         ↓
BUILD PHASE begins:
  → Builder (Hermes) executes the plan
  → Each task checked off as completed
         ↓
REVIEW PHASE:
  → Reviewer (Hermes `requesting-code-review`) reads the plan
  → Validates: "did you follow the plan?"
  → Reports deviations
         ↓
[If deviations: back to BUILD PHASE with specific feedback]
```

### 3.4 Multica Pipeline Stage Integration

The natural way to implement this in our pipeline architecture:

| Stage | Agent | Output |
|---|---|---|
| `PLAN` | Hermes `writing-plans` | Detailed plan attached to issue |
| `BUILD` | Hermes (execution) | Code + tests |
| `REVIEW` | Hermes `requesting-code-review` | Pass/fail + deviation report |
| `DONE` | — | Plan + implementation merged |

**The plan IS the contract for review.** Reviewer says "you deviated from step 3" — Builder goes back to step 3.

---

## 4. Knowledge Manager — Documentation Agent

### 4.1 Role Definition

The Knowledge Manager is a **scheduled documentation maintenance agent** — not a writer, but an editor/auditor.

It runs weekly and ensures the project's documentation remains accurate and useful.

### 4.2 What It Checks

| Check | Action if failed |
|---|---|
| `ARCHITECTURE.md` matches actual code structure | Flag as issue |
| `PLAN.md` is current (no stale completed items) | Archive and clean up |
| New public APIs have doc comments or `docs/API.md` entries | Create documentation stub issue |
| README reflects current project state | Update if >7 days stale |
| Inline comments contradict code | Flag for cleanup |
| CHANGELOG updated since last release | Prompt for release notes |
| Dependency list matches `go.mod` / `package.json` | Flag drift |

### 4.3 Relationship with Overseer

```
Knowledge Manager → Updates docs → Overseer reads updated docs → Better context → Better issues
```

The Knowledge Manager feeds the Overseer. The better the docs, the smarter the Overseer.

### 4.4 Cadence

- **Weekly cron** via Multica Autonomy
- **Runtime:** OpenClaw or Hermes with `documentation-maintenance` skill

### 4.5 Agent Identity

Does not need to be a separate bot. It is a **role**, not a new agent:

- OpenClaw can run it as a scheduled session
- Hermes can run it as a scheduled task with documentation skill
- Both read/write the same project workspace

---

## 5. Per-Project Overseer Configuration

### 5.1 Schema Extension

Add to `project_control_settings` table:

```sql
ALTER TABLE project_control_settings ADD COLUMN overseer_config JSONB;
```

### 5.2 Config Schema

```json
{
  "enabled": true,
  "scan_interval_hours": 24,
  "scan_focus": [
    "security",
    "test_coverage",
    "code_quality",
    "documentation",
    "architecture"
  ],
  "product_context": "Internal employee portal for workflow automation, React + Next.js",
  "quality_bar": ["accessibility", "performance", "test_coverage_above_70"],
  "priority_weights": {
    "security": 10,
    "performance": 7,
    "documentation": 5,
    "features": 3
  },
  "max_issues_per_run": 3,
  "require_approval_before_adding_to_board": true
}
```

### 5.3 Scan Focus Options

| Focus | What the Overseer looks for |
|---|---|
| `security` | CVEs, hardcoded secrets, unsafe patterns, outdated deps |
| `test_coverage` | Files >300 lines with <50% coverage, missing integration tests |
| `code_quality` | Duplication, long functions, missing error handling |
| `documentation` | Undocumented public APIs, outdated README, missing CHANGELOG |
| `architecture` | Violations of ARCHITECTURE.md, circular dependencies, god files |
| `ux_improvements` | Missing inline help, accessibility gaps, confusing flows |

---

## 6. Content / RAG Strategy

### 6.1 The Problem

Both Overseer and Knowledge Manager need to read the codebase. At scale (1000+ files), glob + read becomes slow. RAG can help.

### 6.2 Layered Approach (Don't Over-Engineer)

**Layer 1: Project Wiki (Markdown) — Always, from day 1**
```
docs/
├── ARCHITECTURE.md     ← Overseer + all agents read for strategic context
├── PLAN.md             ← Builder + Reviewer follow this
├── API.md              ← All agents reference as canonical API docs
├── DEV-NOTES.md        ← Project-specific operational knowledge
└── DECISIONS.md        ← Architecture decision records (ADRs)
```

These files are the **primary corpus**. They travel with the project, are version-controlled, and are always current because of the Knowledge Manager.

**Layer 2: Targeted RAG Embeddings — Enable when painful**

Trigger conditions:
- 5+ active projects simultaneously
- Knowledge Manager starts missing relevant context
- Overseer reports "couldn't find relevant architecture context"
- >2000 meaningful files in a single project

Implementation:
- `memory-lancedb` + Mini PC Ollama (`nomic-embed-text`, 24/7)
- Embed only: `ARCHITECTURE.md`, `DECISIONS.md`, `DEV-NOTES.md`, key plan docs
- **Do NOT** embed the full codebase — too noisy, too expensive

**Layer 3: Direct File Reads — Always available**
- Hermes/OpenClaw can always glob + read specific files
- Fast, precise, no index lag
- Used for: implementation tasks, code review, targeted queries

### 6.3 Embedding Strategy

| Document | Embedded? | Why |
|---|---|---|
| `ARCHITECTURE.md` | ✅ Always | Strategic context, long-lived |
| `DECISIONS.md` (ADRs) | ✅ Always | Design rationale, frequently consulted |
| `DEV-NOTES.md` | ✅ Always | Operational knowledge |
| `PLAN.md` (active) | ✅ When active | Current focus |
| Full source code | ❌ No | Too noisy; direct reads suffice |
| Test files | ❌ No | Direct reads for targeted work |
| `node_modules` / vendor | ❌ No | Never |

---

## 7. Implementation Roadmap

### Phase O1 (Current — Already Done)
- [x] Reactive Overseer (blocked escalation, stale detection)
- [x] `project_control_settings` table
- [x] Overseer agent selection per project
- [x] Backend API for project control settings

### Phase O2 (Recommended Next)
- [ ] Add `overseer_config` JSONB column to `project_control_settings`
- [ ] Define Overseer prompt template with scan scope
- [ ] Wire Multica Autonomy → Overseer agent execution
- [ ] Overseer creates first test issues on real projects

### Phase O3
- [ ] Planning Agent integration: Hermes `writing-plans` triggered on issue assignment
- [ ] Plan attachment to issue as first-class artifact
- [ ] Reviewer reads plan as review contract

### Phase O4
- [ ] Knowledge Manager weekly cron
- [ ] Documentation audit checks
- [ ] RAG enablement (when pain threshold reached)

---

## 8. Key Design Decisions

| Decision | Rationale |
|---|---|
| Overseer = CEO, not watchdog | Reactive monitoring is already covered by Autonomy crons. The value add is strategic opportunity identification. |
| Planning Agent is Hermes `writing-plans`, not a new agent | Hermes already has this skill. No new runtime needed. |
| Plan = review contract | The Reviewer validates against the plan, not against "best effort." This is what makes quality enforceable. |
| Knowledge Manager is a role, not a new bot | OpenClaw or Hermes can run it. No separate identity needed. |
| RAG is Layer 2, not Layer 1 | Markdown docs are the primary corpus. RAG is acceleration, not foundation. |
| Per-project Overseer config via JSONB | Flexible, no schema migration, operators can tune per project need |

---

## 9. Relation to Existing Docs

| Doc | Status |
|---|---|
| `overseer-design-spec.md` | **Superseded by this spec** — keep for reference, mark deprecated |
| `elite-mission-control.md` | **Companion** — defines the 3-layer model (Ownership / Execution / Control), this spec refines the Control layer |
| `elite-mission-control-stage-routing-overseer-o1.md` | **Completed** — the reactive Overseer implementation that this spec builds upon |
| `SPEC-STAGE-ROUTING.md` | **Referenced** — pipeline stages are the execution backbone that Planning → Build → Review flow through |

---

## 10. Questions for Review

1. Should the Overseer require human approval before issues enter the board, or add them directly?
2. Should the Planning Agent run automatically when any issue enters the PLAN stage, or only when explicitly triggered?
3. How many Overseer-created issues per run is appropriate before it becomes noise? (currently set to 3 max)
4. Should Knowledge Manager be OpenClaw or Hermes? (affects cost model)

---

_This spec defines the Strategic Overseer. It is a living document — update as the implementation reveals what actually works._
