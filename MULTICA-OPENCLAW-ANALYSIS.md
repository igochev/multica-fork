# Multica + OpenClaw Integration Analysis
_Last updated: 2026-04-18_

## What We Now Know

### How Multica Calls OpenClaw

The Multica daemon spawns: `openclaw agent --local --json --session-id <nanotime> --message <prompt>`

**`--local` is critical here** — it runs OpenClaw in embedded mode, completely bypassing the Gateway. This means:

- The session ends up in `~/.openclaw/agents/main/sessions/` (the main workspace)
- The model used is the **OpenClaw workspace default** (`defaults.model.primary`), NOT a daemon env var
- `--model` flag is **silently ignored** in `--local` mode (confirmed via live test)

### Model Used in Test

Session `multica-1776542696111629088` had a `model_change` event:
```json
{ "provider": "openai-codex", "modelId": "gpt-5.4" }
```

This is the main workspace's primary model. The daemon is NOT hardcoding `MULTICA_OPENCLAW_MODEL=gpt-5.4` — it's simply falling through to OpenClaw's own workspace default.

**To change the model Multica uses for OpenClaw**: Change `agents.defaults.model.primary` in `~/.openclaw/openclaw.json`, OR fork Multica to use Gateway mode.

---

## Two Architectures for Per-Agent Model Selection

### Architecture A: Current (Local Mode, No Fork)
```
multica daemon → openclaw agent --local
  → uses OpenClaw workspace default model
  → all OpenClaw agents use the same model
  → per-agent model selection: IMPOSSIBLE without fork
```

### Architecture B: Gateway Mode (Small Fork)
```
multica daemon → openclaw agent --agent <id> --json
  → routes through OpenClaw Gateway (ws://localhost:18789)
  → Gateway resolves <id> → loads that agent's workspace config
  → uses THAT agent's configured model
  → per-agent model selection: WORKS
```

**The fix is ~5 lines** in `server/pkg/agent/openclaw.go`:
1. Remove `--local` from hardcoded args
2. Add `--agent` flag from `task.Agent.CustomEnv["OPENCLAW_AGENT_ID"]`
3. Add `OPENCLAW_GATEWAY_TOKEN` to spawned process env

---

## Proposed Role Design (Current Reality)

If we accept current limitations:

| Multica Agent | OpenClaw Agent | Model | Role |
|---|---|---|---|
| **Overseer** | OpenClaw `main` | `gpt-5.4` (workspace default) | Planning, orchestration, review |
| **Builder** | Hermes (separate provider) | Hermes own config | Execution, coding |
| **Review** | Hermes or manual | — | Could be Hermes if it has review skills |

**OpenClaw = OverSeer (gpt-5.4) is actually fine** — it's expensive but it's doing the thinking/planning work, which is what you want the expensive model for.

---

## Kanban Workflow Mechanics

### How Assignment Works in Multica
- Issue has ONE assignee at a time
- Assignee picks up the task, executes, reports back, updates status
- **No native stage-based routing** (e.g., no auto-assign to Review Agent when Done)

### Review Phase Workflow Options

**Option 1: Human conductor (simplest)**
1. Builder finishes → moves issue to "In Review"
2. Human reassigns to Review Agent
3. Review Agent picks it up

**Option 2: Builder creates follow-up issue**
1. Builder finishes → creates "Review: <original>" issue
2. Assigns review issue to Review Agent
3. Review Agent picks it up

**Option 3: Single agent does both**
- Builder agent has review skills embedded in instructions
- Completes work → self-reviews → moves to Done
- Simpler but less specialized

**Option 4: Fork for stage-based routing** (future)
- Add automation: when issue status → "In Review", auto-assign to Review Agent
- Requires custom code in Multica backend

### Status Workflow (confirmed working)
OpenClaw gpt-5.4 successfully:
1. Added comment to issue with results
2. Changed issue status to `done`

Required multica CLI commands used by the agent:
- `multica issue comment add <issue-id> --content "..."`
- `multica issue status <id> done`

---

## Next Steps Recommendation

### Test 1: Builder (Hermes) Kanban Flow
1. Create a coding issue (e.g., "Create a README for project X")
2. Assign to Hermes agent in Multica
3. Verify: does Hermes pick it up? Does it move through Todo → In Progress → Done?
4. Check: does Hermes add comments? Does it update status?

**If Hermes works**: Architecture is viable, just needs instructions tuning
**If Hermes fails**: May need different executor or instructions

### Test 2: Review Agent Assignment
1. After Builder moves issue to "In Review" column
2. Reassign to a separate Review Agent
3. Check: does Review Agent understand context from previous comments?

---

## Strategic Decision (2026-04-18)

**DECIDED: Fork Multica and build on top.**

Reasons:
- Multica's Go backend, WebSocket infrastructure, real-time board, Docker Compose, and multi-agent CLI abstraction are all built and working
- 2000+ forks of Multica suggest this is the community consensus approach
- Fork gives us: continuous upstream improvements, CLI/provider support, community contributions
- Mission Control (our prior project) would require rebuilding all infrastructure Multica already has
- Multi-provider support (Claude, Codex, OpenClaw, Hermes, OpenCode, Gemini, Pi, Cursor) is already there

## Fork Strategy

See `SPEC-STAGE-ROUTING.md` for detailed scope of first feature addition.

### Phase 1: Stage-Based Pipeline Routing (Priority 1)
- New tables: `pipelines`, `pipeline_stages`, `issue_pipelines`
- Backend hook fires on issue status change → auto-enqueues next stage's agent
- Frontend: pipeline management UI + project/issue assignment
- Est: ~740 lines across 13 files
- Merge-safe: new tables only, hooks over core changes, feature-gated by pipeline presence

### Phase 2: Daemon Gateway Mode (Priority 2)
- Change `openclaw agent --local` → `openclaw agent --agent <id>` in daemon
- Enables per-agent model selection through OpenClaw's workspace system
- Est: ~10 lines in `server/pkg/agent/openclaw.go`
- Required for true OverSeer (expensive) / Builder (cheap) separation

### Phase 3: Hermes Builder Integration (Priority 3)
- Create Hermes workspace agent in OpenClaw with fast/cheap model
- Configure Hermes CustomEnv with OpenCode Go API key (already working standalone)
- Test full Kanban cycle: Build stage → Hermes picks up → moves through columns → Done
- Instructions: include `multica issue status` + `multica issue comment` commands

## Current Known Gaps in Multica
1. **Stage-based routing** → Phase 1 addresses this
2. **Per-agent model selection** → Phase 2 addresses this
3. **No WIP limits on Kanban** → future phase
4. **No dependency tracking (blocked by upstream issue)** → future phase

## Open Questions

1. **Default pipeline behavior** — auto-assign project pipeline to new issues, or require explicit assignment?
2. **Stage instructions UX** — textarea or skill reference?
3. **Hermes Multica config** — does CustomEnv work for passing OpenCode Go API key to Hermes spawned by Multica daemon?

---

## File Locations
- Multica fork: `~/.openclaw/workspace-coding/projects/multica-fork/`
- OpenClaw config: `~/.openclaw/openclaw.json`
- OpenClaw agents: `~/.openclaw/agents/<id>/`
- Agent sessions: `~/.openclaw/agents/<id>/sessions/`
- Daemon config: `~/.multica/config.json`
