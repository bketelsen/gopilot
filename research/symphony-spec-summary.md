# OpenAI Symphony Specification Summary

Researched: 2026-03-05
Source: https://github.com/openai/symphony/blob/main/SPEC.md (2110 lines)

## What It Is

Symphony is a **long-running daemon/service** that polls an issue tracker, dispatches coding agents (Codex app-server instances) to work on issues in isolated per-issue workspaces, and manages retry/reconciliation. Think of it as a scheduler that turns issue tracker tickets into automated coding sessions.

**Key difference from Superpowers**: Superpowers is prompt engineering (Markdown behavioral contracts injected into an AI's context). Symphony is **infrastructure** — a running process with state machines, polling loops, workspace management, and subprocess orchestration.

---

## Architecture: 8 Components

| Component | Purpose |
|-----------|---------|
| **Workflow Loader** | Reads `WORKFLOW.md` (YAML frontmatter + prompt template), watches for changes, hot-reloads |
| **Config Layer** | Typed config with defaults, `$VAR` env resolution, `~` path expansion |
| **Issue Tracker Client** | Polls tracker for candidate issues, refreshes states, normalizes data |
| **Orchestrator** | Single-authority state machine: polling → dispatch → reconciliation loop |
| **Workspace Manager** | Per-issue isolated directories with lifecycle hooks |
| **Agent Runner** | Wraps workspace + prompt + Codex app-server subprocess |
| **Status Surface** | Optional dashboard/API for observability |
| **Logging** | Structured logs with issue/session context |

---

## Domain Model

### Issue (normalized from tracker)
- `id`, `identifier` (human-readable like "MT-649"), `title`, `description`
- `state` (mapped to active/terminal categories)
- `priority` (integer), `labels` (lowercase strings)
- `blocked_by` (issue IDs blocking this one)
- `created_at`, `updated_at`

### Workflow Definition (`WORKFLOW.md`)
YAML frontmatter + Markdown prompt template:

```yaml
---
tracker:
  kind: linear          # → for gopilot: "github"
  api_key: $LINEAR_KEY  # → $GITHUB_TOKEN
  project_slug: my-proj # → owner/repo or project number
  active_states: [Todo, "In Progress"]
  terminal_states: [Done, Canceled]
polling:
  interval_ms: 30000
  max_concurrent_agents: 5
workspace:
  root: /tmp/workspaces
  hooks:
    after_create: "git clone ... ."
    before_run: "git pull && npm install"
    after_run: "echo done"
    before_remove: "echo cleanup"
agent:
  max_turns: 10
  max_retry_backoff_ms: 300000
codex:
  command: "codex app-server"
  read_timeout_ms: 15000
  turn_timeout_ms: 1800000
  stall_timeout_ms: 300000
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}
{{ issue.description }}
{% if attempt %}This is retry attempt {{ attempt }}.{% endif %}
```

### Orchestrator State Machine

```
Unclaimed → Claimed → Running → (normal exit) → RetryQueued (continuation)
                        ↓                              ↓
                   (error/timeout)              (re-dispatched)
                        ↓
                   RetryQueued (backoff)
                        ↓
                   Released (if issue no longer active)
```

---

## Core Loop: Poll → Dispatch → Reconcile

### Each Tick:
1. **Reconcile** running issues against tracker state
   - Terminal state → kill agent + cleanup workspace
   - Non-active state → kill agent (keep workspace)
   - Still active → update local state
   - Stalled (no events within `stall_timeout_ms`) → kill + retry
2. **Validate** dispatch config (tracker creds, codex binary, etc.)
3. **Fetch** candidate issues from tracker (active states, not blocked)
4. **Sort** by priority (highest first), then oldest `created_at`
5. **Dispatch** to available slots (up to `max_concurrent_agents`)
6. **Schedule** next tick after `poll_interval_ms`

### Dispatch Per Issue:
1. Create/reuse workspace directory
2. Run `before_run` hook
3. Start Codex app-server subprocess
4. Build prompt from template + issue data + attempt number
5. Run turns in a loop (up to `max_turns`)
6. After each turn, re-check issue state in tracker
7. If issue moved out of active state, stop
8. Run `after_run` hook
9. On normal exit → schedule continuation retry (attempt 1, short delay)
10. On error → schedule retry with exponential backoff (10s base, capped at `max_retry_backoff_ms`)

---

## Workspace Management

- **Deterministic path**: `{workspace_root}/{sanitized_issue_identifier}`
- **Persistent**: Workspaces survive across retries and restarts (intentional — preserves git state, build artifacts)
- **Lifecycle hooks** (shell scripts from WORKFLOW.md):
  - `after_create` — run once on first creation (e.g., `git clone`)
  - `before_run` — before each agent attempt (e.g., `git pull`)
  - `after_run` — after each attempt (best-effort, failures logged not fatal)
  - `before_remove` — before cleanup (best-effort)
- **Safety**: path must stay under workspace root, sanitized identifiers, agent CWD locked to workspace

---

## Agent Runner Protocol

Symphony communicates with Codex via a JSON-line protocol over stdin/stdout:

1. **Launch**: `bash -lc "codex app-server"` in workspace directory
2. **Handshake**: `initialize` → `initialized` → `thread/start` → `turn/start`
3. **Turn streaming**: Agent emits events (`turn_completed`, `turn_failed`, `notification`, etc.)
4. **Approval policy**: Implementation-defined (auto-approve commands/file changes, or surface to operator)
5. **User input**: Hard failure — if agent requests user input, fail the run
6. **Timeouts**: Read timeout, turn timeout, stall timeout — all configurable

### Emitted Events:
`session_started`, `turn_completed`, `turn_failed`, `turn_cancelled`, `turn_input_required`, `approval_auto_approved`, `notification`, etc.

---

## Issue Tracker Integration

Currently Linear-only, but designed for adapter pattern:

### Required Operations:
1. `fetch_candidate_issues()` — active issues for configured project
2. `fetch_issues_by_states(states)` — for startup cleanup
3. `fetch_issue_states_by_ids(ids)` — for reconciliation

### Normalization:
- Labels → lowercase
- Blockers → derived from inverse "blocks" relations
- Priority → integer only
- Timestamps → ISO-8601

### Important Boundary:
**Symphony does NOT write to the tracker.** It only reads. The coding agent itself handles state transitions, comments, and PR metadata via tools in its prompt. Symphony remains a scheduler/runner + tracker reader.

---

## Failure Model

### Failure Classes:
1. **Config failures** — bad WORKFLOW.md, missing creds → fail startup or skip dispatch
2. **Workspace failures** — can't create/populate → fail attempt, retry
3. **Agent failures** — startup, turn, timeout, stall → retry with backoff
4. **Tracker failures** — API errors → skip this tick, try next tick
5. **Observability failures** — never crash the orchestrator

### Recovery:
- **In-memory only** — no persistent state across restarts
- On restart: cleanup terminal workspaces, fresh poll, re-dispatch eligible work
- Retry queue: exponential backoff (10s × 2^attempt, capped)
- Normal completion → continuation retry (attempt=1, short delay) to re-check and continue

---

## Optional Extensions

### HTTP Server (`--port` or `server.port`)
- `GET /` — Human-readable dashboard
- `GET /api/v1/state` — JSON: running sessions, retry queue, token totals, rate limits
- `GET /api/v1/<issue_identifier>` — Per-issue debug details
- `POST /api/v1/refresh` — Trigger immediate poll+reconcile

### Client-Side Tools
- `linear_graphql` — agent can execute GraphQL against Linear using Symphony's auth
- Unsupported tool calls → fail gracefully, don't stall

---

## Adaptation Notes for Gopilot

### What to Keep (Core Patterns):
1. **Poll-dispatch-reconcile loop** — maps directly to watching GitHub issues
2. **Per-issue workspace isolation** — clone per issue, persistent across retries
3. **Workspace lifecycle hooks** — `after_create` (clone), `before_run` (pull/install), `after_run`, `before_remove`
4. **WORKFLOW.md as config** — single file defines tracker connection, workspace setup, agent behavior, prompt template
5. **State machine** (Unclaimed→Claimed→Running→RetryQueued→Released) — clean lifecycle management
6. **Retry with backoff** — exponential for errors, continuation for normal completion
7. **Reconciliation** — regularly check if issues are still active, kill orphaned agents
8. **Stall detection** — timeout inactive agents
9. **Tracker is read-only for the orchestrator** — agent handles mutations (comments, state transitions, PRs)
10. **Hot-reload of WORKFLOW.md** — change config without restart

### What to Replace:

| Symphony (Linear) | Gopilot (GitHub) |
|-------------------|------------------|
| `tracker.kind: linear` | `tracker.kind: github` |
| Linear GraphQL API | GitHub REST API + `gh` CLI |
| `project_slug` | `owner/repo` + project number |
| `active_states: [Todo, "In Progress"]` | GitHub issue states + labels (e.g., `open` + `label:ready`) |
| `terminal_states: [Done, Canceled]` | `closed` state |
| Linear issue identifiers (`MT-649`) | GitHub issue numbers (`#42`) |
| `linear_graphql` tool extension | `github_graphql` or just `gh` CLI in agent prompt |
| Priority integer from Linear | GitHub Projects v2 Priority field or labels |
| Blocker relations from Linear | GitHub sub-issues / "blocked by #X" in description |
| Codex app-server (JSON-line protocol) | Claude Code / Copilot CLI (different invocation model) |

### What to Add (Gaps in Symphony):

1. **GitHub Projects v2 integration** — use iterations for sprints, custom fields for status/priority, board views
2. **Sub-issue hierarchy** — epic → story → task breakdown using GitHub sub-issues
3. **PR lifecycle** — Symphony doesn't manage PRs; gopilot should create branches, open PRs, request reviews
4. **Superpowers-style workflow skills** — wrap agent invocations with behavioral contracts (TDD, code review, verification)
5. **Multi-stage review** (from Superpowers) — implementer → spec reviewer → code quality reviewer
6. **Anti-rationalization patterns** — iron laws, red flags, common rationalizations table
7. **Sprint planning** — use GitHub Projects iterations, auto-assign issues to sprints
8. **Cross-repo orchestration** — GitHub Projects v2 spans repos; symphony is single-project
9. **Webhook-driven dispatch** — instead of polling, react to GitHub webhook events (issue created/labeled/assigned)
10. **Cost tracking** — token usage per issue, per sprint, per project

### Suggested Gopilot Architecture:

```
gopilot/
├── WORKFLOW.md                    # Config: GitHub connection, workspace, agent, prompt
├── skills/                        # Superpowers-style behavioral contracts
│   ├── tdd/SKILL.md
│   ├── code-review/SKILL.md
│   ├── verification/SKILL.md
│   └── debugging/SKILL.md
├── src/
│   ├── orchestrator.go            # Poll-dispatch-reconcile loop
│   ├── tracker/
│   │   └── github.go              # GitHub Issues + Projects v2 adapter
│   ├── workspace/
│   │   └── manager.go             # Per-issue workspace with hooks
│   ├── agent/
│   │   └── runner.go              # Claude Code / Copilot invocation
│   ├── config/
│   │   └── workflow.go            # WORKFLOW.md parser + hot-reload
│   └── observability/
│       ├── logger.go
│       └── server.go              # Optional HTTP dashboard
└── docs/
    └── plans/                     # Sprint plans, design docs
```

### Key Design Decision:
Symphony's approach of **orchestrator reads tracker, agent writes tracker** is elegant and should be preserved. The gopilot orchestrator watches GitHub for eligible issues, dispatches agents with workspace + prompt, and the agent itself creates branches, opens PRs, adds comments, and transitions issue state via `gh` CLI.
