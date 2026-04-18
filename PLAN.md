# Multica Fork — Plan & Runbook

> **Project:** `~/.openclaw/workspace-coding/projects/multica-fork/`
> **Fork:** `https://github.com/igochev/multica-fork`
> **Upstream:** `https://github.com/multica-ai/multica`
> **Last updated:** 2026-04-18

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

1. **Skills compounding** — persistent reusable skills indexed semantically (pgvector)
2. **Proactive agent blocker reporting** — agents report blockers autonomously vs reactive callbacks only
3. **Mission Control integration layer** — tighter hooks into OpenClaw/Hermes beyond current daemon dispatch
4. **Elite UI/UX** — redesign dashboard, board, and agent panels to best-in-class
5. **Stronger DoR/DoD enforcement** — configurable checklist-driven gates before column transitions
6. **Execution reconciliation dashboard** — stale detection + admin reconciliation UI
7. **Bidirectional lifecycle loop** — full closed-loop: MC → agent → callback → MC state update

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

**Last session's work:** Setting up initial infrastructure (Docker + CLI + daemon). Next step is to run the server and connect agents.
