# DEV-NOTES.md — Operational Knowledge for the Multica Fork

> This file is for hard lessons, stack quirks, and operational patterns.
> Updated after every significant discovery.

---

## AgentInstructions — The Most Important Discovery (2026-04-19)

**You do NOT need daemon changes to add Planning Agent behavior.**

The flow:
```
Settings → Agents → edit agent → Instructions field
    → stored as agents.instructions in DB
    → daemon writes to .agent_context/issue_context.md + CLAUDE.md at task start
    → Hermes reads CLAUDE.md → follows the instructions
```

**This means:**
- Builder planning = just edit the prompt in the UI
- Overseer CEO behavior = just edit the prompt in the UI
- Knowledge Manager = just edit the prompt in the UI
- No Go code changes, no daemon rebuilds, no new tables needed

**Evidence:** `server/internal/daemon/execenv/runtime_config.go` lines 56-62 show exactly how
`AgentInstructions` is injected. `TaskContextForEnv.AgentInstructions` → CLAUDE.md.

---

## Stage Routing — Pipeline Hook Is Already Wired (2026-04-19)

`server/internal/handler/issue.go` already has the pipeline hook:
```go
if statusChanged && h.PipelineService != nil {
    if _, err := h.PipelineService.MaybeAdvanceIssuePipeline(r.Context(), issue); err != nil {
        slog.Warn("advance issue pipeline failed", ...)
    }
}
```

The hook fires on any status change. The `PipelineService.MaybeAdvanceIssuePipeline` is the
right seam to extend for automatic stage routing.

---

## overseer.go Is Reactive Only (2026-04-19)

`server/internal/service/overseer.go` implements:
- `MaybeEscalateBlockedIssue` — fires when issue status = "blocked"
- `MaybeEscalateStaleIssue` — fires when latest task > stale_after_minutes

This is useful but NOT the strategic Overseer. The strategic Overseer (CEO) is defined in
`docs/architecture/overseer-strategic-ceo-spec.md`. It runs on a schedule and creates issues,
not just reacts to status changes.

---

## Planning Happens Inside `in_progress` (2026-04-19)

**Decision:** No new Kanban phase for PLAN.

Rationale: "In progress" semantically means "I am working on this" — which includes analyzing,
planning, and executing. The Builder agent follows this sequence inside `in_progress`:
1. Read issue description
2. Invoke `writing-plans` skill → create `docs/plans/{slug}.md`
3. Write plan to issue comment
4. Execute plan tasks
5. Invoke `requesting-code-review`
6. Fix deviations
7. Move to `in_review`

---

## Hermeses `writing-plans` + `requesting-code-review` + `systematic-debugging` Skills (2026-04-19)

These three Hermes skills are the backbone of the quality loop:
- `writing-plans` — creates detailed task-by-task implementation plans
- `systematic-debugging` — 4-phase root cause investigation, NO fixes without understanding
- `requesting-code-review` — pre-commit pipeline: static scan + quality gates + independent review

**They work well together.** The Reviewer validates against the plan. If the plan was good,
the Reviewer just checks compliance. If the plan was vague, the Reviewer flags it — and
the next iteration improves the Planning Agent's output.

---

## CLAUDE.md Injection — Provider-Specific Paths

The daemon writes context files differently per provider:
- **Hermes, OpenClaw, Codex, Cursor**: writes to `.agent_context/` in workdir + `CLAUDE.md`
- **Gemini**: intentionally NO CLAUDE.md written (gemini provider skips this in `writeContextFiles`)
- **Codex**: also sets up per-task `CODEX_HOME` from `~/.codex/`

Grounded in: `server/internal/daemon/execenv/execenv_test.go` — provider-specific test assertions.

---

## Multica Plans Folder — Source, Not Just Documentation

`docs/plans/*.md` in the upstream repo proves plans work. For our fork:
- **Builder** writes its own plans to `docs/plans/{issue-slug}.md`
- Plans are committed to the repo alongside code
- The Reviewer reads the plan file for compliance checking
- This makes plans version-controlled, diffable, and reviewable

### Builder → Reviewer contract conclusion (2026-04-19)

The review contract is operational, not magical:
- **Repo access is automatic** once the reviewer checks out the repo.
- **Plan path discovery is not automatic** and therefore requires an issue comment convention.
- **No daemon change is required** for reviewers to read the plan file.

Use this exact rule:
```markdown
Before reviewing code, you must:
1. Read the issue comments and find the canonical plan path posted by the Builder.
2. Check out the repository if it is not already present.
3. Read the plan file from the repo.
4. Review code changes against that plan first, then against general code quality.
5. Report explicit deviations from the plan.
```

Use this exact discovery convention in the Builder's issue comment:
```markdown
Plan path: docs/plans/{issue-slug}.md
Review contract: this file is canonical.
```

Canonical prompts and this reviewer contract now live in `docs/operations/elite-agent-instructions.md`
with a focused copy in `docs/operations/reviewer-plan-contract.md`.

---

## No New Agents Needed — Roles Are Prompts (2026-04-19)

The 5 roles (Overseer/CEO, Planning Agent, Builder, Reviewer, Knowledge Manager) are all
implemented as Hermes + OpenClaw sessions with configured prompts. No new bot identities needed.

This means:
- Switching from Hermes → Codex for the Builder just means changing the provider + keeping the prompt
- The behavior contract stays the same
- Multica's agent abstraction makes this seamless

---

## RAG Decision — Layer 1 Now, Layer 2 When Painful (2026-04-19)

**Layer 1 (today):** Keep ARCHITECTURE.md, DECISIONS.md, DEV-NOTES.md, PLAN.md, API.md
in the repo. They ARE the context. Read them directly.

**Layer 2 (future):** `memory-lancedb` + Mini PC Ollama (`nomic-embed-text`, 24/7) — embed only
ARCHITECTURE.md, DECISIONS.md, DEV-NOTES.md. NOT source code. NOT test files.

**Trigger Layer 2 when:**
- 5+ active projects simultaneously
- Knowledge Manager starts missing relevant context
- >2000 meaningful files in a project
- Overseer reports "couldn't find relevant architecture context"

Mini PC already has nomic-embed-text installed. When Layer 2 is needed, it's ~10 min of config.

---

## Stale Detection — The `stale_after_minutes` Config (2026-04-19)

In `project_control_settings`:
- `stale_after_minutes INT` — threshold for stale issue detection
- `overseer_requested_for_stale_issue` — activity log action when triggered
- `overseer.go` uses `latestInFlightTask` to find the most recent queued/dispatched/running task
- Issues in `done` or `cancelled` status are excluded

---

## Multica Upstream Plans Folder — Manual Planning for Complex Features (2026-04-19)

The upstream multica-ai team uses `docs/plans/*.md` manually for complex features. ~18 commits
per release cycle. This proves:
1. Detailed planning is necessary for complex features
2. Plans are human-written and stored as markdown
3. Builders reference them during implementation

For our fork: Plans are written by the **Planning Agent** (Hermes `writing-plans`) automatically,
not manually. Same artifact format, faster creation.

---

_Updated: 2026-04-19 07:00 GMT+3_
