# SSE Dashboard Refresh Fix

## Problem

After each poll tick, the orchestrator broadcasts `Broadcast("agent-update", "refresh")`. The dashboard uses `sse-swap="agent-update"`, which tells HTMX to replace the div's innerHTML with the SSE event data — the literal string "refresh". The entire dashboard is replaced with the text "REFRESH".

## Solution

Use the SSE event as a trigger for an `hx-get` request instead of `sse-swap`. The event signals "something changed"; a separate HTTP endpoint returns the rendered HTML fragment.

## Design

### Template changes (`dashboard.templ`)

Extract the dashboard inner content into a `DashboardContent` component. The full `Dashboard` renders the layout wrapper + SSE div + initial content. `DashboardContent` renders just the cards and tables (the swappable fragment).

Change the SSE div from:
```html
<div hx-ext="sse" sse-connect="/api/v1/events" sse-swap="agent-update">
```
to:
```html
<div hx-ext="sse" sse-connect="/api/v1/events" hx-trigger="sse:agent-update" hx-get="/api/v1/dashboard">
```

### New endpoint (`server.go`)

Add `GET /api/v1/dashboard` that renders `DashboardContent` with current state data (running agents, retries, planning, metrics, max agents). Uses the same state/metrics/retry providers already wired into the server.

### No changes needed

- `sse.go` — unchanged
- `orchestrator.go` — broadcast calls remain as-is; the data payload is ignored by the client

## Files

| File | Change |
|------|--------|
| `internal/web/templates/pages/dashboard.templ` | Extract `DashboardContent` component, update SSE div attributes |
| `internal/web/server.go` | Add `GET /api/v1/dashboard` handler |
