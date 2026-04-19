# Multica Fork — Plan & Runbook

> **Project:** `~/.openclaw/workspace-coding/projects/multica-fork/`
> **Fork:** `https://github.com/igochev/multica-fork`
> **Upstream:** `https://github.com/multica-ai/multica`
> **Last updated:** 2026-04-19

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│  LAYER 1: DOCKER SERVER  (Mini PC — Docker Desktop)    │
│  ─────────────────────────────────────────────────────  │
│  - Go Backend (port ${PORT:-8080})                     │
│  - Next.js Frontend (port ${FRONTEND_PORT:-3000})      │
│  - PostgreSQL 17 + pgvector (port 5432)                │
│                                                         │
│  docker-compose builds from LOCAL Dockerfiles:          │
│    - backend:  Dockerfile (Go binary)                   │
│    - frontend: Dockerfile.web (Next.js)                 │
│  So our fork IS the server — push changes, rebuild.     │
└─────────────────────────────────────────────────────────┘
          ▲ REST + WebSocket (ws://localhost:8080)
          │ HTTP (http://localhost:3000)
          │
┌─────────────────────────────────────────────────────────┐
│  LAYER 2: MULTICA CLI + DAEMON  (WSL Ubuntu)           │
│  ─────────────────────────────────────────────────────  │
│  - `multica` binary — auth, workspace mgmt, daemonctl    │
│  - `multica daemon` — background process                │
│  - Auto-detects agent CLIs on PATH (openclaw,          │
│    hermes, codex, opencode, gemini, pi, cursor-agent)  │
│  - Installed via: curl script or Homebrew               │
│  - Config: ~/.multica/config.json                      │
│  - Profiles: ~/.multica/profiles/<name>/               │
│  - Logs: ~/.multica/daemon.log                         │
└─────────────────────────────────────────────────────────┘
          │ spawns agent CLI subprocesses when tasks assigned
          ▼
┌─────────────────────────────────────────────────────────┐
│  LAYER 3: AGENT CLIs  (WSL Ubuntu, existing setup)     │
│  ─────────────────────────────────────────────────────  │
│  - openclaw  — already installed, Discord-connected     │
│  - hermes    — already installed, Discord-connected     │
│  - codex     — to be installed                         │
│  - opencode  — optional                                │
│                                                         │
│  These are normal CLI processes spawned by the daemon.  │
│  Their own configs (OpenClaw Discord, Hermes API keys)  │
│  remain unchanged.                                      │
└─────────────────────────────────────────────────────────┘
```

**Key insight:** The daemon doesn't replace agents — it spawns them as subprocesses. Your existing
OpenClaw (Discord bot) and Hermes setups stay untouched. Multica adds the task queue + web UI.

### Elite Mission Control — 5 Role Model

The fork implements 5 distinct agent roles. Each role has a **prompt contract** configured via the
`AgentInstructions` field in Multica UI (Settings → Agents → edit agent → Instructions).
These prompts are injected into `CLAUDE.md` at task start — no daemon changes needed.

| Role | Multica Agent | What it does | Key Prompt Instruction |
|---|---|---|---|
| **Overseer (CEO)** | "Overseer Agent" | Strategic scanning, opportunity identification, creates user-story-level issues | `scan project for X, Y, Z; create issues` |
| **Planning Agent** | (same as Builder) | Converts issue → detailed plan before building | `invoke writing-plans skill first; write plan to docs/plans/` |
| **Builder** | "Hermes Builder" | Executes the plan from Planning Agent | `follow the plan; run TDD; test before commit` |
| **Reviewer** | (same as Builder) | Validates builder output against the plan | `requesting-code-review skill; compare against plan` |
| **Knowledge Manager** | "Documentation Agent" | Weekly doc maintenance, ARCHITECTURE.md hygiene | `audit docs weekly; flag inconsistencies as issues` |

**Planning happens inside `in_progress`** — no new Kanban phase needed. "In progress" semantically
means: analyze → plan → execute. The Builder does this in order. No UI/UX changes required.

**AgentInstructions flow:**
```
Multica UI (Settings → Agents → Instructions field)
    → stored in DB (agents.instructions)
    → daemon writes to .agent_context/issue_context.md + CLAUDE.md at task start
    → Hermes reads CLAUDE.md and follows the instructions
```

### Core Architecture Docs

- `docs/architecture/elite-mission-control.md` — constitution and redesign principles for the fork
- `docs/architecture/overseer-strategic-ceo-spec.md` — **canonical** strategic Overseer (CEO) design
- `docs/architecture/overseer-design-spec.md` — reactive Overseer (legacy, superseded by strategic spec)
- `SPEC-STAGE-ROUTING.md` — Phase 1 deterministic stage-routing backbone

---

## Branch Architecture

```
upstream/main ──────────────────────────────────────────── (original multica, read-only sync)
    │
    │  git fetch upstream && git merge upstream/main
    ▼
fork/main ──────────────────────────────────────────────── (GitHub fork main, synced from upstream)
                                                              │
                                                              │  git merge main
                                                              ▼
fork/multica-my-feats ←── stable feature branch (PR target) ──┐
                                                              │
                                                              │  PR / merge
                                                              ▼
fork/multica-my-feats-dev ←── active development ─────────────┘
    ▲
    │  (checked out — this is where we work)
```

- **`main`** — tracks `fork/main`, synced from upstream. Never develop directly.
- **`multica-my-feats`** — our stable feature branch. PR target for reviewed features.
- **`multica-my-feats-dev`** — our dev branch. All new work happens here.

---

## Remote URLs

| Remote | URL | Purpose |
|--------|-----|---------|
| `upstream` | `https://github.com/multica-ai/multica` | Original multica (read-only sync source) |
| `fork` | `https://github.com/igochev/multica-fork` | Our fork (push here) |

---

## Prerequisites Met

- [x] Fork cloned at `~/.openclaw/workspace-coding/projects/multica-fork/`
- [x] `upstream` remote added and all branches fetched
- [x] `multica-my-feats-dev` checked out
- [x] Local `multica-my-feats` branch created (tracking `fork/multica-my-feats`)
- [x] Dockerfiles confirmed local (Dockerfile + Dockerfile.web in repo root)
- [x] Docker Desktop on Windows Mini PC — **ENABLE WSL INTEGRATION before first run**
- [x] Confirmed: fork/main and upstream/main are at same commit (no drift since fork)

---

## Setup Instructions (Step by Step)

### Step 1: Enable Docker Desktop WSL Integration (Windows Side)

1. Open **Docker Desktop** → Settings → **Resources** → **WSL Integration**
2. Toggle **ON** for your Ubuntu distro
3. Click Apply & Restart
4. Verify in WSL: `docker ps` should work without error

### Step 2: Configure .env for Self-Hosting

```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork

# Copy example env
cp .env.example .env

# Edit .env — minimum changes:
# APP_ENV=development    # enables dev code 888888 (safe for private network)
# JWT_SECRET=<random>    # openssl rand -hex 32
# (everything else can stay as defaults)
```

**Why APP_ENV=development:** The selfhost stack defaults to production where 888888 is disabled.
Since this runs on your private LAN, dev mode is fine and saves needing Resend API key setup.

### Step 3: Start the Server

```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork

# Start all services (postgres + backend + frontend)
docker compose -f docker-compose.selfhost.yml up -d

# Watch for healthy startup
docker compose -f docker-compose.selfhost.yml logs --follow
# or check individually:
docker compose -f docker-compose.selfhost.yml ps
```

Expected after startup:
- **Frontend:** http://localhost:3000
- **Backend API:** http://localhost:8080
- **Backend health:** `curl http://localhost:8080/health` → `{"status":"ok"}`

### Step 4: Install Multica CLI + Daemon (WSL)

```bash
# Install the multica binary (no Docker needed — CLI only)
curl -fsSL https://raw.githubusercontent.com/multica-ai/multica/main/scripts/install.sh | bash

# Verify
multica version
```

### Step 5: Configure CLI to Point at Local Server

```bash
# Point CLI at our local server
multica config set server_url http://localhost:8080
multica config set app_url http://localhost:3000

# Check config
multica config show
```

### Step 6: Authenticate

```bash
# Headless auth (no browser needed):
# Send verification code request
curl -s -X POST "http://localhost:8080/auth/send-code" \
  -H "Content-Type: application/json" \
  -d '{"email": "dev@localhost"}'

# Check backend logs for the code:
docker compose -f docker-compose.selfhost.yml logs backend | grep "Verification code"

# Or use the CLI login (opens browser if available):
multica login --token
```

For development without browser, use the API directly:
```bash
# Get JWT token
JWT=$(curl -s -X POST "http://localhost:8080/auth/verify-code" \
  -H "Content-Type: application/json" \
  -d '{"email": "dev@localhost", "code": "888888"}' | jq -r '.token')

# Create a PAT
PAT=$(curl -s -X POST "http://localhost:8080/api/tokens" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name": "local-dev", "expires_in_days": 365}' | jq -r '.token')

# Create workspace
WS=$(curl -s -X POST "http://localhost:8080/api/workspaces" \
  -H "Authorization: Bearer $PAT" \
  -H "Content-Type: application/json" \
  -d '{"name": "Dev", "slug": "dev"}' | jq -r '.id')

# Write CLI config manually
mkdir -p ~/.multica
cat > ~/.multica/config.json << EOF
{
  "server_url": "http://localhost:8080",
  "app_url": "http://localhost:3000",
  "token": "$PAT",
  "workspace_id": "$WS",
  "watched_workspaces": [{"id": "$WS", "name": "Dev"}]
}
EOF
```

### Step 7: Start Daemon

```bash
# Start the agent daemon (auto-detects CLIs on PATH)
multica daemon start

# Verify it sees our agents
multica daemon status

# Check it registered runtimes
multica daemon status --output json | jq '.runtimes'
```

### Step 8: Verify in Web UI

1. Open http://localhost:3000
2. Login: `dev@localhost` / `888888`
3. Go to **Settings → Runtimes** — your machine should appear
4. Go to **Settings → Agents** — create agents:
   - Name: "OpenClaw Agent", Runtime: your machine, Provider: OpenClaw
   - Name: "Hermes Agent", Runtime: your machine, Provider: Hermes
   - Name: "Codex Agent", Runtime: your machine, Provider: Codex
5. Create an issue and assign to an agent → it should pick up the task

---

## Stopping Services

```bash
# Stop Docker server
cd ~/.openclaw/workspace-coding/projects/multica-fork
docker compose -f docker-compose.selfhost.yml down

# Stop daemon
multica daemon stop
```

---

## Upgrading

### Upgrade 1: Pull Official Upstream Updates (fork/main)

```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork

git checkout main
git fetch upstream
git merge upstream/main          # resolve any conflicts
git push fork main              # push merged state to our fork

# Sync into our feature branches:
git checkout multica-my-feats
git merge main
git push fork multica-my-feats

git checkout multica-my-feats-dev
git merge multica-my-feats
git push fork multica-my-feats-dev

# Rebuild server with new upstream code:
docker compose -f docker-compose.selfhost.yml up -d --build
```

Migrations run automatically on backend startup (idempotent).

### Upgrade 2: Deploy Our Custom Features

```bash
cd ~/.openclaw/workspace-coding/projects/multica-fork

# Work on dev branch:
git checkout multica-my-feats-dev
# ... implement features ...

git push fork multica-my-feats-dev

# Open PR on GitHub: multica-my-feats-dev → multica-my-feats
# Review and merge

# Deploy merged features:
git checkout multica-my-feats
git pull fork multica-my-feats
docker compose -f docker-compose.selfhost.yml up -d --build
```

### Upgrade 3: Multica CLI / Daemon

```bash
multica update       # auto-upgrades the binary
multica daemon restart
```

---

## Multica CLI Quick Reference

```bash
multica version                          # Show version
multica update                           # Upgrade CLI
multica config show                      # Show current config
multica config set server_url <url>      # Change server URL
multica config set app_url <url>        # Change frontend URL

multica daemon start                    # Start daemon (background)
multica daemon stop                      # Stop daemon
multica daemon status                   # Show status
multica daemon status --output json      # JSON output
multica daemon logs                      # Last 50 log lines
multica daemon logs -f                   # Follow logs

multica workspace list                   # List watched workspaces
multica workspace watch <id>             # Watch a workspace
multica workspace unwatch <id>           # Unwatch

multica issue list                       # List issues
multica issue list --status in_progress  # Filter by status
multica issue create --title "..."       # Create issue
multica issue get <id>                   # Get issue detail

multica agent list                       # List agents in workspace

multica autopilot list                   # List autopilots
multica autopilot create --title "..."   # Create autopilot

# Profiles (multiple daemon instances):
multica setup self-host --profile staging
multica daemon start --profile staging
```

---

## Known Differences from Upstream (Roadmap)

These are features we want to add or redesign in our fork:

1. ~~Proactive agent blocker reporting~~ — ✅ DONE in Task 9 (reactive blocked/stale escalation)
2. **Strategic Overseer (CEO)** — proactive opportunity scanning + issue creation (see `overseer-strategic-ceo-spec.md`)
3. **Planning Agent integration** — Builder uses `AgentInstructions` to call `writing-plans` before coding (no daemon change)
4. **Plan-as-review-contract** — Reviewer validates against the plan file, not just code quality
5. **Knowledge Manager role** — weekly documentation maintenance cron
6. **Skills compounding** — persistent reusable skills indexed semantically (pgvector) — future
7. **Elite UI/UX** — redesign dashboard, board, and agent panels to best-in-class — future
8. **Per-project Overseer config** — `overseer_config` JSONB column on `project_control_settings` — future
9. **Bidirectional lifecycle loop** — full closed-loop: MC → agent → callback → MC state update — future

### Key Verdicts (2026-04-19)

| Question | Decision |
|---|---|
| New Kanban phase for PLAN? | **No** — planning happens inside `in_progress`. "In progress" = analyze + plan + execute. |
| Daemon change needed for planning? | **No** — use `AgentInstructions` field. Written to CLAUDE.md at task start. |
| Overseer = watchdog or CEO? | **CEO** — strategic opportunity identification. Reactive stale/blocked detection is a bonus, not the core value. |
| RAG needed now? | **No** — Layer 1 (markdown docs) is sufficient. Layer 2 (lancedb + Ollama embeddings) when pain threshold reached. |
| New agents needed for roles? | **No** — roles are prompts/config, not new bots. Hermes + OpenClaw cover all 5 roles. |
| Multica plans/ folder for execution? | **Yes** — `docs/plans/{issue-slug}.md` is where Builder writes plans. Feeds the Reviewer. |

---

## Agent Instructions Configuration Guide

### How It Works

1. Go to Multica UI → Settings → Agents → edit any agent
2. Fill the **Instructions** field (text area)
3. The daemon writes this into `CLAUDE.md` in the task workdir at task start
4. The agent (Hermes, Codex, etc.) reads CLAUDE.md and follows the instructions

### Builder Agent Instructions (Canonical Prompt)

```markdown
You are the Builder agent for Elite Mission Control.

RULES (non-negotiable — every time):
1. When you receive an issue, IMMEDIATELY invoke the `writing-plans` skill.
2. Read the issue description carefully.
3. Create a detailed plan at `docs/plans/{issue-slug}.md`.
   Include: task breakdown, exact file paths, test approach, review criteria.
4. Write the plan back into the issue as a comment so the Reviewer can validate it.
5. Execute the plan task by task. Run tests after each task.
6. If tests fail: invoke `systematic-debugging` — NO fixes without root cause.
7. After all tasks pass: invoke `requesting-code-review`.
8. The Reviewer will compare your work against the plan file.
9. Fix any deviations the Reviewer identifies before marking complete.

Never start coding before the plan file exists and is written to the issue.
```

### Overseer Agent Instructions (Canonical Prompt)

```markdown
You are the Overseer (CEO) for this project.

EVERY 6-24 HOURS (per your schedule):
1. Read the codebase delta since last run (new/changed files, new dependencies).
2. Read the board state (backlog size, stalled items, blocked issues).
3. Read ARCHITECTURE.md and DECISIONS.md for current project direction.
4. Look for:
   - Security: new CVEs in dependencies, hardcoded secrets, unsafe patterns
   - Test coverage: files >300 lines with <50% coverage
   - Code quality: duplication, missing error handling, god files
   - Documentation: undocumented public APIs, outdated README
   - Architecture: violations of ARCHITECTURE.md
   - UX: missing inline help, accessibility gaps
5. Create user-story-level issues (not implementation-level) — include:
   - What the problem/opportunity is
   - Why it matters to the project
   - Estimated effort (rough)
6. Maximum 3 new issues per run. Prioritize by project `overseer_config` weights.
7. Do NOT write code. Do NOT create detailed plans.

Report what you found in a summary comment on the project.
```

### Knowledge Manager Instructions (Canonical Prompt)

```markdown
You are the Knowledge Manager for this project.

EVERY WEEK:
1. Check ARCHITECTURE.md — does it match actual code structure?
2. Check PLAN.md — archive completed items, flag stale gaps.
3. Check docs/API.md — new public handlers without API docs?
4. Check inline comments — any contradict the code?
5. Check CHANGELOG — updated since last release?
6. Flag all issues as documentation maintenance tickets (do NOT fix yourself).

Output a brief report of what needs attention.
```

---

## Session Resumption Guide

If you come back to this project after a break, check this file first.

**Was the server running?**
```bash
docker compose -f docker-compose.selfhost.yml ps
# If nothing running:
docker compose -f docker-compose.selfhost.yml up -d
```

**Was the daemon running?**
```bash
multica daemon status
# If stopped:
multica daemon start
```

**What branch are we on?**
```bash
git branch  # should show * multica-my-feats-dev
git log --oneline -3
```

**Last session's work (2026-04-19):** Completed Tasks 1-9 of elite-mission-control-stage-routing-overseer-o1 plan.
Pipeline routing + reactive Overseer (blocked/stale escalation) built and committed.
Major design decision: Overseer = strategic CEO role, not reactive cron.
New spec written: `docs/architecture/overseer-strategic-ceo-spec.md`.
AgentInstructions discovery: no daemon changes needed for Planning Agent — just configure the prompt.
Next step: configure Hermes Builder AgentInstructions in Multica UI, test with a 1-line issue.
