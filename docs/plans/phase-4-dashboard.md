# Phase 4: Dashboard

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Web dashboard with real-time agent status, issue details, SSE updates, and JSON API.

**Prerequisite:** Phase 3 complete.

---

### Task 4.1: Web Server Setup with Chi

**Files:**
- Create: `internal/web/server.go`
- Test: `internal/web/server_test.go`

**Step 1: Write the failing test**

```go
// internal/web/server_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/gopilot/internal/orchestrator"
)

func TestHealthEndpoint(t *testing.T) {
	state := orchestrator.NewState()
	srv := NewServer(state, nil)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/web/...`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/web/server.go
package web

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/orchestrator"
)

// Server serves the web dashboard and JSON API.
type Server struct {
	router chi.Router
	state  *orchestrator.State
	cfg    *config.Config
}

// NewServer creates a new web server.
func NewServer(state *orchestrator.State, cfg *config.Config) *Server {
	s := &Server{
		state: state,
		cfg:   cfg,
	}
	s.router = s.buildRouter()
	return s
}

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// JSON API
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/state", s.handleState)
		r.Get("/issues/{owner}/{repo}/{id}", s.handleIssueDetail)
		r.Post("/refresh", s.handleRefresh)
		r.Get("/events", s.handleSSE)
	})

	// Dashboard pages (Phase 4.2+)
	r.Get("/", s.handleDashboardPage)
	r.Get("/issues/{owner}/{repo}/{id}", s.handleIssueDetailPage)

	return r
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running_count": len(running),
		"running":       running,
	})
}

func (s *Server) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	// Placeholder — will be fleshed out
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "not implemented"})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	// Trigger immediate tick — requires orchestrator reference (added later)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "refresh queued"})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// SSE implementation in Task 4.5
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	// templ rendering — Task 4.3
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html><body><h1>Gopilot Dashboard</h1><p>Coming soon.</p></body></html>"))
}

func (s *Server) handleIssueDetailPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html><body><h1>Issue Detail</h1><p>Coming soon.</p></body></html>"))
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/web/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/web/
git commit -m "feat: web server skeleton with chi router and JSON API stubs"
```

---

### Task 4.2: JSON API — State Endpoint

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/web/server_test.go`

**Step 1: Write the failing test**

```go
// Append to server_test.go

func TestStateEndpoint(t *testing.T) {
	state := orchestrator.NewState()
	state.AddRunning(42, &domain.RunEntry{
		Issue:     domain.Issue{ID: 42, Repo: "o/r", Title: "Fix bug"},
		SessionID: "sess-1",
		StartedAt: time.Now(),
		Attempt:   1,
	})

	srv := NewServer(state, nil)
	req := httptest.NewRequest("GET", "/api/v1/state", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["running_count"].(float64) != 1 {
		t.Errorf("running_count = %v, want 1", resp["running_count"])
	}
}
```

**Step 2: Run and verify — should already pass with current implementation**

Run: `go test -race ./internal/web/...`
Expected: PASS (handleState already returns running entries)

**Step 3: Commit if tests pass, otherwise fix**

```bash
git add internal/web/
git commit -m "feat: JSON API state endpoint with running agents"
```

---

### Task 4.3: Templ Base Layout

**Files:**
- Create: `internal/web/templates/layouts/base.templ`
- Create: `internal/web/templates/input.css`
- Create: `internal/web/static/.gitkeep`

**Step 1: Create the base layout template**

```go
// internal/web/templates/layouts/base.templ
package layouts

templ Base(title string) {
	<!DOCTYPE html>
	<html lang="en" class="dark">
	<head>
		<meta charset="UTF-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
		<title>{ title } — Gopilot</title>
		<link rel="stylesheet" href="/static/styles.css"/>
		<script src="https://unpkg.com/htmx.org@2.0.4"></script>
		<script src="https://unpkg.com/htmx-ext-sse@2.2.2/sse.js"></script>
	</head>
	<body class="bg-background text-foreground min-h-screen">
		<div class="flex min-h-screen">
			@Sidebar()
			<main class="flex-1 p-6">
				{ children... }
			</main>
		</div>
	</body>
	</html>
}

templ Sidebar() {
	<aside class="w-64 border-r border-border bg-card p-4">
		<div class="mb-8">
			<h1 class="text-xl font-bold">Gopilot</h1>
		</div>
		<nav class="space-y-2">
			<a href="/" class="block px-3 py-2 rounded-md hover:bg-accent">Dashboard</a>
			<a href="/sprint" class="block px-3 py-2 rounded-md hover:bg-accent">Sprint</a>
			<a href="/settings" class="block px-3 py-2 rounded-md hover:bg-accent">Settings</a>
		</nav>
	</aside>
}
```

Tailwind input CSS:

```css
/* internal/web/templates/input.css */
@import "tailwindcss";
```

**Step 2: Generate templ files**

Run: `templ generate`
Expected: generates `*_templ.go` files

**Step 3: Commit**

```bash
git add internal/web/templates/ internal/web/static/
git commit -m "feat: templ base layout with sidebar navigation"
```

---

### Task 4.4: Dashboard Page Template

**Files:**
- Create: `internal/web/templates/pages/dashboard.templ`

**Step 1: Create the dashboard template**

```go
// internal/web/templates/pages/dashboard.templ
package pages

import (
	"fmt"
	"time"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

templ Dashboard(running []*domain.RunEntry, retries []*domain.RetryEntry, stats DashboardStats) {
	@layouts.Base("Dashboard") {
		<div class="space-y-6">
			<!-- Summary Cards -->
			<div class="grid grid-cols-4 gap-4">
				@SummaryCard("Active Agents", fmt.Sprintf("%d / %d", stats.ActiveAgents, stats.MaxSlots))
				@SummaryCard("Issues Today", fmt.Sprintf("%d", stats.IssuesToday))
				@SummaryCard("Success Rate", fmt.Sprintf("%.0f%%", stats.SuccessRate*100))
				@SummaryCard("Total Tokens", fmt.Sprintf("%dk", stats.TotalTokens/1000))
			</div>

			<!-- Active Agents Table -->
			<div class="rounded-lg border border-border bg-card" hx-ext="sse" sse-connect="/api/v1/events">
				<div class="p-4 border-b border-border">
					<h2 class="text-lg font-semibold">Active Agents</h2>
				</div>
				<div sse-swap="agent-update">
					@AgentTable(running)
				</div>
			</div>

			<!-- Retry Queue -->
			if len(retries) > 0 {
				<div class="rounded-lg border border-border bg-card">
					<div class="p-4 border-b border-border">
						<h2 class="text-lg font-semibold">Retry Queue</h2>
					</div>
					@RetryTable(retries)
				</div>
			}
		</div>
	}
}

type DashboardStats struct {
	ActiveAgents int
	MaxSlots     int
	IssuesToday  int
	SuccessRate  float64
	TotalTokens  int64
}

templ SummaryCard(title string, value string) {
	<div class="rounded-lg border border-border bg-card p-4">
		<p class="text-sm text-muted-foreground">{ title }</p>
		<p class="text-2xl font-bold">{ value }</p>
	</div>
}

templ AgentTable(entries []*domain.RunEntry) {
	<table class="w-full">
		<thead>
			<tr class="border-b border-border text-left text-sm text-muted-foreground">
				<th class="p-3">Issue</th>
				<th class="p-3">Repo</th>
				<th class="p-3">Status</th>
				<th class="p-3">Duration</th>
				<th class="p-3">Turns</th>
				<th class="p-3">Last Activity</th>
			</tr>
		</thead>
		<tbody>
			for _, entry := range entries {
				<tr class="border-b border-border">
					<td class="p-3">
						<a href={ templ.URL(fmt.Sprintf("/issues/%s/%d", entry.Issue.Repo, entry.Issue.ID)) } class="text-primary hover:underline">
							#{ fmt.Sprintf("%d", entry.Issue.ID) }
						</a>
						<span class="ml-2 text-sm">{ entry.Issue.Title }</span>
					</td>
					<td class="p-3 text-sm">{ entry.Issue.Repo }</td>
					<td class="p-3">
						<span class="inline-flex items-center rounded-full bg-green-500/10 px-2 py-1 text-xs text-green-500">
							Running
						</span>
					</td>
					<td class="p-3 text-sm">{ formatDuration(entry.Duration()) }</td>
					<td class="p-3 text-sm">{ fmt.Sprintf("%d", entry.TurnCount) }</td>
					<td class="p-3 text-sm">{ formatTimeAgo(entry.LastEventAt) }</td>
				</tr>
			}
			if len(entries) == 0 {
				<tr>
					<td colspan="6" class="p-6 text-center text-muted-foreground">No active agents</td>
				</tr>
			}
		</tbody>
	</table>
}

templ RetryTable(entries []*domain.RetryEntry) {
	<table class="w-full">
		<thead>
			<tr class="border-b border-border text-left text-sm text-muted-foreground">
				<th class="p-3">Issue</th>
				<th class="p-3">Attempt</th>
				<th class="p-3">Next Retry</th>
				<th class="p-3">Error</th>
			</tr>
		</thead>
		<tbody>
			for _, entry := range entries {
				<tr class="border-b border-border">
					<td class="p-3">{ entry.Identifier }</td>
					<td class="p-3">{ fmt.Sprintf("%d", entry.Attempt) }</td>
					<td class="p-3 text-sm">{ formatTimeAgo(entry.DueAt) }</td>
					<td class="p-3 text-sm text-destructive">{ entry.Error }</td>
				</tr>
			}
		</tbody>
	</table>
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm ago", int(d.Minutes()))
}
```

**Step 2: Generate and build**

Run: `templ generate && go build ./...`
Expected: SUCCESS

**Step 3: Wire up the handler in server.go**

Update `handleDashboardPage` to render the templ component using `s.state.AllRunning()` and stats.

**Step 4: Commit**

```bash
git add internal/web/
git commit -m "feat: dashboard page with agent table, retry queue, and summary cards"
```

---

### Task 4.5: SSE Event Streaming

**Files:**
- Create: `internal/web/sse.go`
- Test: `internal/web/sse_test.go`

**Step 1: Write the failing test**

```go
// internal/web/sse_test.go
package web

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/orchestrator"
)

func TestSSEStream(t *testing.T) {
	state := orchestrator.NewState()
	srv := NewServer(state, nil)
	hub := srv.sseHub

	// Start SSE request
	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.ServeHTTP(w, req)
		close(done)
	}()

	// Give it time to connect
	time.Sleep(50 * time.Millisecond)

	// Broadcast an event
	hub.Broadcast("agent-update", "<tr>updated</tr>")

	time.Sleep(50 * time.Millisecond)

	// Check output contains SSE format
	body := w.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))
	foundEvent := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "event: agent-update") {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Errorf("expected SSE event in output: %q", body)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/web/... -run TestSSE`
Expected: FAIL

**Step 3: Write SSE hub implementation**

```go
// internal/web/sse.go
package web

import (
	"fmt"
	"net/http"
	"sync"
)

// SSEHub manages Server-Sent Event connections.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

// SSEEvent is a single event to send.
type SSEEvent struct {
	Type string
	Data string
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan SSEEvent]struct{}),
	}
}

// Broadcast sends an event to all connected clients.
func (h *SSEHub) Broadcast(eventType string, data string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	event := SSEEvent{Type: eventType, Data: data}
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
			// Client too slow, skip
		}
	}
}

// Subscribe adds a client. Returns a channel and cleanup function.
func (h *SSEHub) Subscribe() (chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
		close(ch)
	}
}

// HandleSSE is the HTTP handler for SSE connections.
func (h *SSEHub) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cleanup := h.Subscribe()
	defer cleanup()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			flusher.Flush()
		}
	}
}
```

Wire `sseHub` into `Server` struct and use `h.sseHub.HandleSSE` for the `/api/v1/events` handler.

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/web/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/web/
git commit -m "feat: SSE hub for real-time dashboard updates"
```

---

### Task 4.6: Integrate Dashboard into Orchestrator Startup

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `cmd/gopilot/main.go`

**Step 1: Start web server in orchestrator.Run()**

In `Run()`, if dashboard is enabled, start HTTP server:

```go
if o.cfg.Dashboard.Enabled {
    webSrv := web.NewServer(o.state, o.cfg)
    go func() {
        slog.Info("dashboard starting", "addr", o.cfg.Dashboard.Addr)
        if err := http.ListenAndServe(o.cfg.Dashboard.Addr, webSrv); err != nil {
            slog.Error("dashboard server error", "error", err)
        }
    }()
}
```

After each tick, broadcast SSE update:

```go
if o.sseHub != nil {
    o.sseHub.Broadcast("agent-update", "refresh")
    o.sseHub.Broadcast("stats-update", "refresh")
}
```

**Step 2: Verify build**

Run: `go build ./cmd/gopilot/`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add internal/orchestrator/ internal/web/ cmd/gopilot/
git commit -m "feat: integrate web dashboard into orchestrator startup"
```

---

## Phase 4 Milestone

Run: `go test -race ./...` — all tests pass.
Run: `go build -o gopilot ./cmd/gopilot/` — builds.

Dashboard:
- Chi router with JSON API endpoints
- Dashboard page with active agents table, retry queue, summary cards
- SSE event streaming for real-time updates
- HTMX integration for partial page updates
- Templ templates with Tailwind CSS dark mode
