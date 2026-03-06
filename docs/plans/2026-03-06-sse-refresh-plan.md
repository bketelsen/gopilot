# SSE Dashboard Refresh Fix — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the dashboard SSE refresh so it swaps in rendered HTML instead of displaying the literal text "refresh".

**Architecture:** Change from `sse-swap` (which replaces innerHTML with event data) to `hx-trigger="sse:agent-update"` + `hx-get="/api/v1/dashboard"` (which triggers an HTTP fetch on SSE event). Extract dashboard inner content into a `DashboardContent` component so the fragment endpoint can render it independently.

**Tech Stack:** templ, HTMX SSE extension, chi router

---

### Task 1: Extract DashboardContent component

**Files:**
- Modify: `internal/web/templates/pages/dashboard.templ`

**Step 1: Edit the templ file**

Split the existing `Dashboard` component. Extract everything inside the `<div hx-ext="sse" ...>` into a new `DashboardContent` component. Update the SSE div to use `hx-trigger` + `hx-get` instead of `sse-swap`.

In `dashboard.templ`, the `Dashboard` component becomes:

```templ
templ Dashboard(running []*domain.RunEntry, retries []*domain.RetryEntry, planning []*domain.PlanningEntry, metrics map[string]int64, maxAgents int) {
	@layouts.Base("Dashboard") {
		<div hx-ext="sse" sse-connect="/api/v1/events" hx-trigger="sse:agent-update" hx-get="/api/v1/dashboard">
			@DashboardContent(running, retries, planning, metrics, maxAgents)
		</div>
	}
}
```

Add a new `DashboardContent` component containing everything that was previously inside the SSE div (summary cards, active agents table, planning table, retry queue table). Same signature, just the inner HTML.

**Step 2: Regenerate templ**

Run: `task generate`
Expected: Clean exit, `dashboard_templ.go` regenerated.

**Step 3: Verify it compiles**

Run: `go build ./internal/web/...`
Expected: No errors.

**Step 4: Commit**

```bash
git add internal/web/templates/pages/dashboard.templ internal/web/templates/pages/dashboard_templ.go
git commit -m "refactor: extract DashboardContent component for SSE fragment"
```

---

### Task 2: Add dashboard fragment endpoint

**Files:**
- Modify: `internal/web/server.go:72-80` (add route)
- Modify: `internal/web/server.go` (add handler method)

**Step 1: Add the route and handler**

In `buildRouter()`, add inside the `/api/v1` route group (after the existing `/events` line):

```go
r.Get("/dashboard", s.handleDashboardFragment)
```

Add the handler method. It mirrors `handleDashboardPage` but renders only `DashboardContent` (no layout wrapper):

```go
func (s *Server) handleDashboardFragment(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	var retries []*domain.RetryEntry
	if s.retries != nil {
		retries = s.retries.All()
	}
	var planningEntries []*domain.PlanningEntry
	if s.planning != nil {
		planningEntries = s.planning.AllPlanning()
	}
	m := map[string]int64{}
	if s.metrics != nil {
		m = s.metrics.All()
	}
	component := pages.DashboardContent(running, retries, planningEntries, m, s.cfg.Polling.MaxConcurrentAgents)
	component.Render(r.Context(), w)
}
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: No errors.

**Step 3: Commit**

```bash
git add internal/web/server.go
git commit -m "feat: add /api/v1/dashboard fragment endpoint for SSE refresh"
```

---

### Task 3: Smoke test

**Step 1: Run existing tests**

Run: `task test`
Expected: All tests pass.

**Step 2: Manual verification**

Run: `task dev`

1. Open `http://localhost:<port>/` in browser
2. Open browser DevTools → Network tab
3. Verify SSE connection to `/api/v1/events` is established
4. Wait for a poll tick
5. Verify: SSE event `agent-update` arrives, triggers GET `/api/v1/dashboard`, dashboard content swaps in without showing "REFRESH" text

**Step 3: Commit (if any fixes needed)**
