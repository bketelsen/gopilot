# Gopilot Full Rewrite — Design Document

Date: 2026-03-05
Status: Approved
Spec: `research/SPEC-DRAFT.md` (v0.1.0-draft)

---

## Overview

Clean slate rewrite of Gopilot following the full specification across all 6 phases. The existing codebase (Phases 1-3) serves as reference only. The spec is the sole source of truth.

## Decisions

- **Approach**: Phase-linear — follow the spec's 6-phase ordering sequentially
- **Existing code**: Clean slate — rewrite from scratch
- **Agent priority**: Copilot CLI first (Phase 1), Claude Code adapter in Phase 6
- **Dependencies**: chi for routing, go-github for REST, manual GraphQL (lightweight), fsnotify for config watching, templ + templUI for dashboard, yaml.v3 for config, log/slog for logging
- **Testing**: TDD per phase — tests first, then implementation, integration tests at phase boundaries

---

## Phase 1: Core Loop (MVP)

The foundation. Establishes the poll-dispatch-reconcile architecture and all core components.

### 1.1 Domain Model & Interfaces

Define core types from spec Section 4:
- `Issue` struct with full Projects v2 fields (Status, Priority, Iteration, Effort, hierarchy)
- `OrchestratorState`, `RunEntry`, `RetryEntry`, `TokenCounts`, `TokenTotals`
- `AgentRunner` interface (spec Section 8.1)
- GitHub client interface (for mocking)
- Workspace manager interface

Package layout matches spec Section 15:
```
internal/
├── config/        (config.go, watcher.go)
├── github/        (client.go, issues.go, projects.go)
├── orchestrator/  (orchestrator.go, state.go, dispatch.go, reconcile.go)
├── workspace/     (manager.go, hooks.go)
├── agent/         (runner.go, copilot.go, claude.go, process.go)
├── skills/        (loader.go, injector.go)
└── web/           (server.go, handlers.go, sse.go, templates/)
```

### 1.2 Config Parser

`gopilot.yaml` loading with:
- Environment variable resolution (`$GITHUB_TOKEN` -> env value)
- Validation (required fields, valid ranges)
- Sensible defaults (spec Section 11.4)
- `gopilot init` generates starter config

TDD: parsing, env resolution, validation errors, defaults.

### 1.3 GitHub REST Client

Interface-based client for:
- `FetchCandidateIssues()` — fetch issues, filter by eligible/excluded labels
- `FetchIssueState(id)` — single issue state for reconciliation
- `AddComment(issue, body)` — failure notifications
- Label management

Normalization: labels lowercase, priority integer mapping, timestamps to `time.Time`.

TDD: normalization, eligibility rules (spec Section 6.2).

### 1.4 GitHub GraphQL Client

Projects v2 integration:
- Discover project metadata (Status/Priority field IDs and option IDs)
- Enrich issues with project fields (Status, Priority, Iteration, Effort)
- `SetProjectField(issue, field, value)` mutation (Status -> "In Progress")
- `FetchIssueStates(ids)` batch query for reconciliation

Manual GraphQL queries (no githubv4 library — queries are simple and well-defined).

TDD: field parsing, priority normalization, mutation construction.

### 1.5 Workspace Manager

Per-issue workspace lifecycle:
- Path: `{root}/{repo_name}/issue-{issue_id}`
- `ensure(issue)` — create if missing, reuse if exists
- `cleanup(issue)` — run before_remove hook, delete directory
- Lifecycle hooks: `after_create`, `before_run`, `after_run`, `before_remove`
- Template variables: `{{repo}}`, `{{issue_id}}`, `{{branch}}`, `{{workspace}}`
- Hook timeout: configurable (default 60s)
- Path safety: reject traversal attempts, sanitize identifiers

TDD: path generation, sanitization, hook execution, timeout behavior.

### 1.6 Agent Runner + Copilot CLI Adapter

`AgentRunner` interface:
```go
type AgentRunner interface {
    Start(ctx context.Context, workspace string, prompt string, opts AgentOpts) (*Session, error)
    Stop(session *Session) error
    Name() string
}
```

Copilot CLI adapter:
- Launch: `copilot -p <prompt> --allow-all --no-ask-user --autopilot --max-autopilot-continues N --model M --share path -s`
- Environment: GITHUB_TOKEN, COPILOT_GITHUB_TOKEN, GH_TOKEN
- Timeout enforcement: turn timeout (30min default), SIGTERM -> 10s wait -> SIGKILL
- Event emission: agent_started, agent_output, agent_completed, agent_failed, agent_timeout
- Session tracking: PID, start time, context cancellation

TDD: process lifecycle, timeout behavior, event emission.

### 1.7 Prompt Renderer

Go template rendering with:
- Issue data (repo, ID, title, body, labels, priority)
- Attempt number (for retry context)
- Parent issue context (sub-issues)
- Workflow steps and rules (from spec Section 4.3)
- Skills injection point (placeholder for Phase 3)

TDD: template rendering with various issue shapes, missing fields, retry context.

### 1.8 Orchestrator

Core poll-dispatch-reconcile loop (spec Section 5.2):
```
on_tick():
  1. RECONCILE — check running agents against GitHub state
  2. PROCESS RETRY QUEUE — dispatch due retries
  3. FETCH CANDIDATES — query GitHub, filter, normalize
  4. SORT — priority (urgent first), then oldest first
  5. DISPATCH — claim, workspace, prompt, launch agent
  6. NOTIFY — push state to dashboard (SSE)
  7. SCHEDULE next tick
```

Thread-safe state management:
- `Running` map (issue ID -> RunEntry)
- `Claimed` map (issue ID -> bool)
- `RetryQueue` map (issue ID -> RetryEntry)

Dispatch flow (spec Section 5.3):
1. Ensure workspace
2. Render prompt
3. Start agent subprocess
4. Record in state
5. On exit: schedule continuation or retry

TDD: test each tick phase independently, state transitions, concurrency limits.

### 1.9 CLI

Commands:
- `gopilot` — start with `./gopilot.yaml`
- `gopilot --config /path/to.yaml` — explicit config
- `gopilot --port 8080` — override dashboard port
- `gopilot --dry-run` — validate config, list eligible issues, exit
- `gopilot --debug` — verbose logging
- `gopilot version` — print version
- `gopilot init` — create starter `gopilot.yaml` + `skills/`

Signal handling: SIGINT/SIGTERM -> graceful shutdown (stop agents, run hooks, exit).

### 1.10 Structured Logging

`log/slog` with context fields:
- `issue_id`, `issue` (owner/repo#N), `session_id`, `workspace`, `attempt`, `ts`
- Log to stderr
- Debug level gated behind `--debug` flag

### Phase 1 Milestone

`gopilot --dry-run` lists eligible issues from GitHub with Projects v2 enrichment. `gopilot` dispatches Copilot CLI agents to per-issue workspaces and tracks their lifecycle. No retry logic, no stall detection, no dashboard yet.

---

## Phase 2: Reliability

Adds fault tolerance to the core loop.

### 2.1 Retry Queue

Exponential backoff: `delay = min(10s * 2^attempt, max_retry_backoff_ms)`

Retry processing in tick loop:
- For each entry where `due_at <= now`: re-fetch issue, dispatch if eligible and slots available
- If issue no longer eligible: release claim
- If no slots: requeue with incremented backoff

TDD: backoff calculation, queue ordering, max retries, re-eligibility checks.

### 2.2 Stall Detection

Monitor `last_event_at` per running agent:
- If `now - last_event_at > stall_timeout_ms`: kill agent, schedule retry
- Add comment: "Agent stalled after {duration}, retrying"

TDD: stall detection timing, kill behavior, comment posting.

### 2.3 Reconciliation Against GitHub

Each tick, re-fetch state for running issues:
- **Terminal state** (Done/Closed/Canceled): kill agent, run after_run hook, remove workspace, release claim
- **Non-eligible** (label removed, status changed): kill agent, run after_run hook, keep workspace, release claim
- **Still active**: update local state with latest GitHub data

Kill sequence: SIGTERM -> wait 10s -> SIGKILL.

TDD: each reconciliation scenario, kill sequence timing.

### 2.4 Max Retries Handling

After `max_retries` exceeded:
1. Release claim
2. Set issue Status back to "Todo" via Projects v2 mutation
3. Add comment: "Gopilot failed after {N} attempts. Last error: {error}"
4. Add label: `gopilot-failed`

TDD: full failure flow end-to-end.

### 2.5 Config Hot-Reload

fsnotify watcher on `gopilot.yaml`:
- On change: re-parse, validate
- If valid: apply safe fields (polling interval, concurrency, agent settings, skills dirs)
- If invalid: keep last-known-good config, log error
- Never reload: GitHub token, repos (require restart)

TDD: reload with valid/invalid configs, safe field application.

### Phase 2 Milestone

Agents recover from crashes and stalls via retry with exponential backoff. Failed issues get labeled (`gopilot-failed`) and commented. Config changes apply without restart. Reconciliation detects externally-changed issues.

---

## Phase 3: Skills

Behavioral contracts for agent discipline.

### 3.1 Skill Loader

Discover `SKILL.md` files from `skills.dir` (max depth 3):
- Parse YAML frontmatter: `name`, `description`, `type` (rigid/flexible/technique)
- Load supporting files in skill directory
- Later directories override earlier ones (custom overrides defaults)

Skill directory structure:
```
skills/
├── tdd/SKILL.md
├── verification/SKILL.md
├── pr-workflow/SKILL.md
├── debugging/SKILL.md
└── code-review/SKILL.md
```

TDD: discovery, frontmatter parsing, override behavior, max depth.

### 3.2 Skill Injector

Prompt injection logic:
- Required skills (`skills.required`): always injected
- Optional skills (`skills.optional`): injected when description matches issue context
- Format: section markers with skill content
- All supporting files available for inclusion

TDD: injection with various skill/issue combinations, required vs optional.

### 3.3 Default Skills

Write 5 shipped skills following the superpowers pattern:

1. **tdd** (rigid) — Red-green-refactor. Iron law: never write implementation before failing test. Anti-rationalization table.
2. **verification** (rigid) — Never claim done without evidence. Full test suite, build, linter must pass.
3. **pr-workflow** (rigid) — Never push to main. Branch -> commit -> push -> PR -> comment -> set "In Review".
4. **debugging** (technique) — Reproduce -> isolate -> root cause -> fix -> verify -> regression check.
5. **code-review** (flexible) — Adversarial reviewer prompt for multi-agent setups.

### 3.4 Skill Hot-Reload

Re-discover skills when config reloads or skills directory changes.

TDD: skill changes picked up on reload.

### Phase 3 Milestone

Agent prompts include behavioral contracts. TDD and verification are enforced via prompt injection. Custom project-specific skills work.

---

## Phase 4: Dashboard

Web UI for orchestrator visibility.

### 4.1 Web Server Setup

- chi router with middleware (logging, recovery)
- templ rendering engine
- templUI component installation (Sidebar, Table, Card, Badge, etc.)
- Static asset serving (Tailwind CSS v4)
- Build chain: `templ generate` + `tailwindcss` CLI + `go build`

### 4.2 Base Layout

- Sidebar navigation (Dashboard, Sprint, Settings)
- Dark mode support via Tailwind
- Responsive layout
- Base templ template with HTMX SSE extension loaded

### 4.3 Dashboard Page (`/`)

- **Active Agents Table**: Issue, Repo, Status badge (Running/Stalled/Retrying), Duration, Turn Count, Last Activity, Tokens
- **Retry Queue Table**: Issue, Repo, Attempt, Next Retry, Error
- **Summary Cards**: Issues processed today, Active agents / max slots, Total tokens / estimated cost, Success rate

Live-updating via SSE.

### 4.4 Issue Detail Page (`/issues/{repo}/{id}`)

- Issue title, description, labels, priority
- Agent session history (all attempts)
- Per-attempt: duration, turns, tokens, exit status, error
- Live agent output (streaming via SSE when running)
- Workspace path and branch info
- Links to GitHub issue and PR

### 4.5 SSE Event Streaming

`/api/v1/events` endpoint:
- Render templ fragments for changed components
- HTMX SSE extension swaps HTML in place
- Event types: `agent-update`, `retry-update`, `stats-update`

TDD: event serialization, connection lifecycle, fragment rendering.

### 4.6 JSON API

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/state` | GET | Current orchestrator state |
| `/api/v1/issues/{repo}/{id}` | GET | Issue detail with session history |
| `/api/v1/sprint` | GET | Current sprint summary |
| `/api/v1/refresh` | POST | Trigger immediate poll+reconcile |
| `/api/v1/events` | GET (SSE) | Real-time event stream |

TDD: each endpoint response shape, error cases.

### Phase 4 Milestone

Running gopilot shows a live dashboard at `localhost:3000` with real-time agent status, retry queue, issue details, and summary stats. JSON API available for programmatic access.

---

## Phase 5: Sprint & Analytics

Cost visibility and sprint tracking.

### 5.1 Token Usage Tracking

- Capture token counts from agent session output (parse from Copilot CLI output)
- Store per-issue (`RunEntry.Tokens`) and aggregate (`OrchestratorState.TotalTokens`)
- Update after each agent session ends

### 5.2 Cost Estimation

Configurable pricing per model:
```
cost = input_tokens * input_price + output_tokens * output_price
```

Default prices:
- Claude Sonnet: $3/M input, $15/M output
- Claude Opus: $15/M input, $75/M output

Track per-issue, per-sprint, and aggregate.

### 5.3 Sprint View (`/sprint`)

- Current iteration from Projects v2 Iteration field
- Issues grouped by status: Todo, In Progress, In Review, Done
- Progress bar (done / total)
- Token cost for the sprint
- Burn-down or throughput chart (Chart.js via templUI)

### 5.4 Metrics Exposure

Structured metrics (spec Section 13.2):
- Issues dispatched, completed, failed (counters)
- Agent session duration (histogram)
- Token usage: input, output, total (counters)
- Estimated cost in USD (counter)
- GitHub API calls and rate limit remaining (gauges)
- Active agents / max slots (gauge)
- Retry queue depth (gauge)

### Phase 5 Milestone

Sprint view shows iteration progress with cost tracking. Token usage visible per-issue and in aggregate. Charts show throughput trends.

---

## Phase 6: Multi-Agent & Extensions

Expand agent support and orchestration sophistication.

### 6.1 Claude Code Adapter

Implement `AgentRunner` for Claude Code CLI:
```
claude --dangerously-skip-permissions --print .gopilot-prompt.md
```
- Write prompt to `.gopilot-prompt.md` in workspace
- Environment: GITHUB_TOKEN, GH_TOKEN
- Parse output for token usage

### 6.2 Agent Selection Config

Allow per-repo or per-label agent selection:
```yaml
agent:
  default: copilot
  overrides:
    - repos: [owner/repo-a]
      command: claude
    - labels: [use-claude]
      command: claude
```

### 6.3 Sub-Issue Hierarchy

- Parse parent/child relationships from GitHub sub-issues API
- Parse "blocked by #N" from issue body
- Respect `BlockedBy` in eligibility: all blockers must be in terminal state
- Dispatch children before parents (or skip blocked issues)

### 6.4 Settings Page (`/settings`)

- Current `gopilot.yaml` config (read-only display)
- Loaded skills list with type badges
- GitHub connection status (token valid, rate limit)
- Agent command validation (binary exists, version)

### Phase 6 Milestone

Multiple agent backends supported. Sub-issue dependencies respected. Settings page provides system health overview.

---

## Cross-Cutting Concerns

### Security (Spec Section 14)

- Agent subprocess runs as same OS user
- Agent CWD restricted to workspace directory
- GitHub token passed via env var, never logged
- Workspace paths sanitized, must stay under root
- Orchestrator never executes code from issues

### Restart Recovery (Spec Section 12.3)

- State is in-memory only (by design)
- On restart: clean up terminal workspaces, fresh poll, re-dispatch
- No retry timers restored (simplicity over durability)

### Operator Controls (Spec Section 12.4)

All control via GitHub:
- Stop agent: move issue to Done/Canceled
- Pause agent: remove gopilot label
- Retry: move Status back to Todo
- Block: add "blocked" label
- Emergency stop: kill process (clean restart)
