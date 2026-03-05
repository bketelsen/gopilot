# Gopilot Gap Closure — Design Document

Date: 2026-03-05
Status: Approved
Baseline: Full rewrite (phases 0-6) merged to main

---

## Overview

Close all gaps between the original implementation plan and the current codebase. The rewrite completed backend infrastructure solidly but left significant gaps in the dashboard UI, integration wiring, and metrics.

## Approach: Foundation-First

1. De-risk the templ/HTMX toolchain first (largest unknown)
2. Wire backend gaps (agent selection, blocking, GraphQL enrichment)
3. Build dashboard pages with real data
4. Fill metrics gaps last

## Decisions

- **Dashboard rendering:** templ + templUI + HTMX + Tailwind CSS v4 (per original plan)
- **templ toolchain:** `go tool templ generate` via Go 1.25+ tool dependencies (no global install)
- **Tailwind CSS:** Standalone CLI binary downloaded to local `bin/`
- **Sub-issue blocking:** Body-text parsing only (`ParseBlockedBy`), no GitHub sub-issues API
- **Token parsing:** Deferred — `TokenTracker` infrastructure stays but no agent stdout parsing
- **Sprint view:** Status grouping + progress bar, no Chart.js charts
- **Agent selection:** Runner registry (`map[string]agent.Runner`) with `AgentCommandForIssue()` dispatch

---

## Section 1: Templ/HTMX Toolchain & Dashboard Foundation

### 1.1 Tool Dependencies

- Add templ as Go tool dependency: `go get -tool github.com/a-h/templ/cmd/templ@latest`
- Download `tailwindcss` standalone CLI binary to `bin/tailwindcss`
- Run `templui init` or configure `.templui.json` for component installation
- Install templUI components: Sidebar, Table, Card, Badge, Button

### 1.2 Build Chain

Wire `Taskfile.yml`:
1. `go tool templ generate` — compile .templ to .go
2. `bin/tailwindcss -i input.css -o static/css/styles.css` — compile CSS
3. `go build` — compile binary

### 1.3 Base Layout

Create `internal/web/templates/layouts/base.templ`:
- HTML shell with HTMX + SSE extension loaded via CDN
- Tailwind CSS v4 stylesheet link
- Sidebar navigation: Dashboard, Sprint, Settings
- Dark mode support via Tailwind `dark:` classes
- Main content area slot

### 1.4 Smoke Test

- Route `/` renders base layout with placeholder content
- Verify full build chain: `task build` produces binary that serves styled page

---

## Section 2: Backend Wiring Gaps

### 2a. Multi-Agent Selection (Critical)

- Orchestrator holds `map[string]agent.Runner` keyed by command name
- At dispatch time, call `cfg.AgentCommandForIssue(repo, labels)` to select runner
- `NewOrchestrator` accepts runner registry instead of single runner
- Falls back to default agent if override not found

### 2b. Sub-Issue Blocking Enforcement (Critical)

- In `Tick()` candidate filtering, check `issue.IsBlocked(resolvedMap)`
- Build `resolvedMap` from issues known to be in terminal state
- Blocked issues skipped silently (picked up when blockers resolve)

### 2c. Projects v2 Enrichment (Medium)

- Add `EnrichIssues()` to GraphQL client: batch query for Priority, Iteration, Effort
- Call after REST fetch in `FetchCandidateIssues` flow
- Priority and Iteration fields populated for sorting and sprint view

### 2d. Max Retries Reset Status (Medium)

- In `handleMaxRetriesExceeded()`, call `o.github.SetProjectStatus(ctx, issue, "Todo")`

### 2e. Retry Eligibility Re-check (Low)

- After fetching fresh issue state in retry processing, verify `issue.IsEligible()` before dispatch
- If not eligible, release claim and skip

### 2f. CLI `--port` Flag (Medium)

- Add `--port` flag to CLI that overrides `cfg.Dashboard.Addr`

### 2g. Fix Retry Empty Repo (Low)

- Add `Repo` field to `RetryEntry`
- Use it in `FetchIssueState` call during retry processing

---

## Section 3: Dashboard Pages

### 3a. Dashboard Page (`/`)

- **Active Agents Table:** Issue, Repo, Status badge (Running/Stalled/Retrying), Duration, Turn Count, Last Activity, Tokens
- **Retry Queue Table:** Issue, Repo, Attempt, Next Retry, Error
- **Summary Cards:** Issues processed today, Active agents / max slots, Total tokens / estimated cost, Success rate
- Live-updating via HTMX SSE extension — events trigger fragment swaps

### 3b. Issue Detail Page (`/issues/{repo}/{id}`)

- Issue title, description, labels, priority
- Agent session history (all attempts)
- Per-attempt: duration, turns, tokens, exit status, error
- Workspace path and branch info
- Links to GitHub issue
- Add session history tracking to state (track completed runs, not just current)

### 3c. Sprint Page (`/sprint`)

- Issues grouped by status: Todo, In Progress, In Review, Done
- Progress bar (done / total)
- Token cost for sprint
- No charts (deferred)
- Requires Projects v2 Iteration enrichment (Section 2c)

### 3d. Settings Page (`/settings`)

- Current `gopilot.yaml` config (read-only display)
- Loaded skills list with type badges
- GitHub connection status (token valid, rate limit remaining)
- Agent command validation (binary exists in PATH)

### 3e. SSE Fragment Rendering

- Render templ components as HTML fragments in SSE broadcast
- HTMX SSE extension swaps fragments by target ID
- Event types: `agent-update`, `retry-update`, `stats-update`

### 3f. JSON API Additions

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/issues/{repo}/{id}` | GET | Issue detail with session history |
| `/api/v1/sprint` | GET | Current sprint summary |
| `/api/v1/refresh` | POST | Trigger immediate poll+reconcile |

---

## Section 4: Metrics & Analytics

### 4a. Session Duration Tracking

- Record duration in `RunEntry` when agent completes
- Track min/max/avg session duration in metrics (simple stats, not histogram)

### 4b. GitHub API Rate Limit Tracking

- Parse `X-RateLimit-Remaining` and `X-RateLimit-Reset` from REST response headers
- Expose as metrics
- Display on Settings page

### 4c. Metrics in Dashboard

- Wire all metrics into Summary Cards (dashboard page) and Settings page
- Metrics: dispatched, completed, failed, active/max, retry depth, rate limit

---

## Gap Coverage Matrix

| Gap | Severity | Section |
|-----|----------|---------|
| Dashboard UI (templ/HTMX/CSS) | Critical | 1, 3 |
| Multi-agent selection wiring | Critical | 2a |
| Sub-issue blocking enforcement | Critical | 2b |
| Projects v2 Priority/Iteration enrichment | Medium | 2c |
| --port CLI flag | Medium | 2f |
| Max retries Status reset | Medium | 2d |
| Dashboard pages (all 4) | Medium | 3a-3d |
| Sprint view | Medium | 3c |
| Settings page | Medium | 3d |
| SSE fragment rendering | Medium | 3e |
| JSON API additions | Medium | 3f |
| Parent issue context in prompt | Low | Not planned (minimal value) |
| Retry empty repo string | Low | 2g |
| Retry eligibility re-check | Low | 2e |
| Session duration metric | Low | 4a |
| GitHub API rate limit tracking | Low | 4b |
| Token parsing from agent output | Deferred | — |
| Chart.js burn-down charts | Deferred | — |
