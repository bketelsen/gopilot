# Section 3: Dashboard Pages

---

### Task 1: Expand StateProvider and web Server for dashboard data

**Files:**
- Modify: `internal/web/server.go` (expand StateProvider, add RetryProvider)
- Modify: `internal/orchestrator/state.go` (ensure AllRetries satisfies interface)
- Test: `internal/web/server_test.go`

**Step 1: Expand the StateProvider interface**

In `internal/web/server.go`, expand the interfaces to expose all data the dashboard needs:

```go
type StateProvider interface {
	AllRunning() []*domain.RunEntry
	AllRetries() []*domain.RetryEntry
	RunningCount() int
}

type MetricsProvider interface {
	All() map[string]int64
	Get(name string) int64
}

type ConfigProvider interface {
	GetConfig() *config.Config
}

type SkillsProvider interface {
	LoadedSkills() []SkillInfo
}

type SkillInfo struct {
	Name        string
	Type        string
	Description string
}
```

Update `Server` struct:

```go
type Server struct {
	router   chi.Router
	state    StateProvider
	cfg      *config.Config
	sseHub   *SSEHub
	metrics  MetricsProvider
	retries  RetryProvider
}
```

Note: `orchestrator.State` already has `AllRetries()` — but it returns `[]*domain.RetryEntry` which needs to be importable. Since `web` already imports `domain`, this works.

Alternatively, keep it simple: just add `AllRetries()` to `StateProvider`. The orchestrator `State` already implements it.

Also add `RetryQueue` access. The orchestrator needs to expose retry queue data to the web server. The simplest approach: add a `RetryQueueProvider` interface or pass the retry queue data through the existing `StateProvider`.

**Step 2: Add RetryEntry import**

The `StateProvider` interface already uses `*domain.RunEntry`. Add `AllRetries() []*domain.RetryEntry` to it. The `orchestrator.State` already implements this method, so no changes needed there.

Actually, looking at the existing code, `State.AllRetries()` returns `[]*domain.RetryEntry` which is the retry entries tracked in state, but the actual `RetryQueue` is separate. We need to expose the `RetryQueue.All()` data too.

The cleanest approach: have the orchestrator pass both state and retry queue to the web server. Update `NewServer`:

```go
type RetryProvider interface {
	All() []*domain.RetryEntry
	Len() int
}
```

Update `NewServer` to accept the retry provider:

```go
func NewServer(state StateProvider, cfg *config.Config, metrics MetricsProvider, retries RetryProvider) *Server {
```

**Step 3: Update orchestrator to pass retry queue to web server**

In `internal/orchestrator/orchestrator.go`, update the web server creation:

```go
webSrv := web.NewServer(o.state, o.cfg, o.metrics, o.retryQueue)
```

This works because `RetryQueue` already has `All()` and `Len()` methods.

**Step 4: Run tests and fix compilation**

Run: `go test -race ./...`
Fix any compilation errors from the updated interfaces.

**Step 5: Commit**

```bash
git add internal/web/server.go internal/orchestrator/orchestrator.go internal/web/server_test.go
git commit -m "feat: expand web server interfaces for full dashboard data access"
```

---

### Task 2: Dashboard page templ template

**Files:**
- Create: `internal/web/templates/pages/dashboard.templ`
- Modify: `internal/web/server.go` (handler)

**Step 1: Create dashboard data types**

In `internal/web/server.go`, add view model types:

```go
type DashboardData struct {
	Running     []*domain.RunEntry
	Retries     []*domain.RetryEntry
	Metrics     map[string]int64
	MaxAgents   int
}
```

**Step 2: Create dashboard.templ**

```go
package pages

import (
	"fmt"
	"time"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

templ Dashboard(running []*domain.RunEntry, retries []*domain.RetryEntry, metrics map[string]int64, maxAgents int) {
	@layouts.Base("Dashboard") {
		<div hx-ext="sse" sse-connect="/api/v1/events" sse-swap="agent-update">
			<!-- Summary Cards -->
			<div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6" id="summary-cards">
				@SummaryCard("Active Agents", fmt.Sprintf("%d / %d", len(running), maxAgents), "blue")
				@SummaryCard("Dispatched", fmt.Sprintf("%d", metrics["issues_dispatched"]), "green")
				@SummaryCard("Completed", fmt.Sprintf("%d", metrics["issues_completed"]), "emerald")
				@SummaryCard("Failed", fmt.Sprintf("%d", metrics["issues_failed"]), "red")
			</div>

			<!-- Active Agents Table -->
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow mb-6">
				<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700">
					<h2 class="text-lg font-semibold">Active Agents</h2>
				</div>
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead class="bg-gray-50 dark:bg-gray-700">
							<tr>
								<th class="px-4 py-2 text-left">Issue</th>
								<th class="px-4 py-2 text-left">Repo</th>
								<th class="px-4 py-2 text-left">Status</th>
								<th class="px-4 py-2 text-left">Duration</th>
								<th class="px-4 py-2 text-left">Attempt</th>
								<th class="px-4 py-2 text-left">Last Activity</th>
							</tr>
						</thead>
						<tbody>
							if len(running) == 0 {
								<tr>
									<td colspan="6" class="px-4 py-8 text-center text-gray-500">No active agents</td>
								</tr>
							}
							for _, entry := range running {
								<tr class="border-t border-gray-200 dark:border-gray-700">
									<td class="px-4 py-2">
										<a href={ templ.SafeURL(fmt.Sprintf("/issues/%s/%d", entry.Issue.Repo, entry.Issue.ID)) } class="text-blue-500 hover:underline">
											{ entry.Issue.Identifier() }
										</a>
									</td>
									<td class="px-4 py-2">{ entry.Issue.Repo }</td>
									<td class="px-4 py-2">
										@StatusBadge("Running")
									</td>
									<td class="px-4 py-2">{ formatDuration(entry.Duration()) }</td>
									<td class="px-4 py-2">{ fmt.Sprintf("%d", entry.Attempt) }</td>
									<td class="px-4 py-2">{ entry.LastEventAt.Format("15:04:05") }</td>
								</tr>
							}
						</tbody>
					</table>
				</div>
			</div>

			<!-- Retry Queue Table -->
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow">
				<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700">
					<h2 class="text-lg font-semibold">Retry Queue</h2>
				</div>
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead class="bg-gray-50 dark:bg-gray-700">
							<tr>
								<th class="px-4 py-2 text-left">Issue</th>
								<th class="px-4 py-2 text-left">Attempt</th>
								<th class="px-4 py-2 text-left">Next Retry</th>
								<th class="px-4 py-2 text-left">Error</th>
							</tr>
						</thead>
						<tbody>
							if len(retries) == 0 {
								<tr>
									<td colspan="4" class="px-4 py-8 text-center text-gray-500">No retries queued</td>
								</tr>
							}
							for _, entry := range retries {
								<tr class="border-t border-gray-200 dark:border-gray-700">
									<td class="px-4 py-2">{ entry.Identifier }</td>
									<td class="px-4 py-2">{ fmt.Sprintf("%d", entry.Attempt) }</td>
									<td class="px-4 py-2">{ entry.DueAt.Format("15:04:05") }</td>
									<td class="px-4 py-2 text-red-500">{ entry.Error }</td>
								</tr>
							}
						</tbody>
					</table>
				</div>
			</div>
		</div>
	}
}

templ SummaryCard(title, value, color string) {
	<div class={ "bg-white dark:bg-gray-800 rounded-lg shadow p-4 border-l-4 border-" + color + "-500" }>
		<div class="text-sm text-gray-500 dark:text-gray-400">{ title }</div>
		<div class="text-2xl font-bold mt-1">{ value }</div>
	</div>
}

templ StatusBadge(status string) {
	switch status {
		case "Running":
			<span class="px-2 py-1 text-xs font-medium rounded-full bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">Running</span>
		case "Stalled":
			<span class="px-2 py-1 text-xs font-medium rounded-full bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">Stalled</span>
		case "Retrying":
			<span class="px-2 py-1 text-xs font-medium rounded-full bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">Retrying</span>
		default:
			<span class="px-2 py-1 text-xs font-medium rounded-full bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200">{ status }</span>
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
```

**Step 3: Update handleDashboardPage handler**

In `internal/web/server.go`:

```go
func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	var retries []*domain.RetryEntry
	if s.retries != nil {
		retries = s.retries.All()
	}
	m := map[string]int64{}
	if s.metrics != nil {
		m = s.metrics.All()
	}

	component := pages.Dashboard(running, retries, m, s.cfg.Polling.MaxConcurrentAgents)
	component.Render(r.Context(), w)
}
```

Import: `"github.com/bketelsen/gopilot/internal/web/templates/pages"`

**Step 4: Run templ generate and build**

Run: `go tool templ generate && go build ./cmd/gopilot/`
Expected: Compiles

**Step 5: Run tests**

Run: `go test -race ./...`
Expected: All pass

**Step 6: Commit**

```bash
git add internal/web/templates/pages/ internal/web/server.go
git commit -m "feat: dashboard page with active agents table, retry queue, summary cards"
```

---

### Task 3: Session history tracking for issue detail

**Files:**
- Modify: `internal/orchestrator/state.go` (add history tracking)
- Modify: `internal/domain/types.go` (add CompletedRun type)
- Test: `internal/orchestrator/state_test.go`

**Step 1: Add CompletedRun type**

In `internal/domain/types.go`:

```go
// CompletedRun records a finished agent session for history.
type CompletedRun struct {
	SessionID  string
	Attempt    int
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	ExitCode   int
	Error      string
	Tokens     TokenCounts
}
```

**Step 2: Add history tracking to State**

In `internal/orchestrator/state.go`, add a `history` map and methods:

```go
type State struct {
	mu      sync.RWMutex
	running map[int]*domain.RunEntry
	claimed map[int]bool
	retry   map[int]*domain.RetryEntry
	history map[int][]domain.CompletedRun // issue ID -> completed runs
	totals  domain.TokenTotals
}
```

Update `NewState`:
```go
func NewState() *State {
	return &State{
		running: make(map[int]*domain.RunEntry),
		claimed: make(map[int]bool),
		retry:   make(map[int]*domain.RetryEntry),
		history: make(map[int][]domain.CompletedRun),
	}
}
```

Add methods:
```go
func (s *State) AddHistory(issueID int, run domain.CompletedRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[issueID] = append(s.history[issueID], run)
}

func (s *State) GetHistory(issueID int) []domain.CompletedRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.history[issueID]
}
```

**Step 3: Write test**

In `internal/orchestrator/state_test.go`:

```go
func TestStateHistory(t *testing.T) {
	s := NewState()
	s.AddHistory(1, domain.CompletedRun{SessionID: "s1", Attempt: 1, ExitCode: 0})
	s.AddHistory(1, domain.CompletedRun{SessionID: "s2", Attempt: 2, ExitCode: 1, Error: "crashed"})

	history := s.GetHistory(1)
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].SessionID != "s1" {
		t.Errorf("first run session = %q, want s1", history[0].SessionID)
	}
}
```

**Step 4: Record history in monitorAgent**

In `internal/orchestrator/orchestrator.go`, in `monitorAgent()`, before releasing the claim, record the completed run:

```go
func (o *Orchestrator) monitorAgent(issue domain.Issue, sess *agent.Session, entry *domain.RunEntry) {
	<-sess.Done
	// ... existing code ...

	finishedAt := time.Now()
	errMsg := ""
	if sess.ExitErr != nil {
		errMsg = sess.ExitErr.Error()
	}
	o.state.AddHistory(issue.ID, domain.CompletedRun{
		SessionID:  sess.ID,
		Attempt:    entry.Attempt,
		StartedAt:  entry.StartedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(entry.StartedAt),
		ExitCode:   sess.ExitCode,
		Error:      errMsg,
		Tokens:     entry.Tokens,
	})

	// ... rest of existing code (success/retry handling) ...
}
```

**Step 5: Run tests**

Run: `go test -race ./...`
Expected: All pass

**Step 6: Commit**

```bash
git add internal/domain/types.go internal/orchestrator/state.go internal/orchestrator/state_test.go internal/orchestrator/orchestrator.go
git commit -m "feat: track completed session history per issue"
```

---

### Task 4: Issue detail page

**Files:**
- Create: `internal/web/templates/pages/issue_detail.templ`
- Modify: `internal/web/server.go` (add route + handler)

**Step 1: Expand StateProvider for history**

In `internal/web/server.go`, add to `StateProvider`:

```go
type StateProvider interface {
	AllRunning() []*domain.RunEntry
	AllRetries() []*domain.RetryEntry
	RunningCount() int
	GetRunning(issueID int) *domain.RunEntry
	GetHistory(issueID int) []domain.CompletedRun
}
```

**Step 2: Create issue_detail.templ**

```go
package pages

import (
	"fmt"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

templ IssueDetail(issue *domain.RunEntry, history []domain.CompletedRun, issueID int, repo string) {
	@layouts.Base(fmt.Sprintf("Issue %s#%d", repo, issueID)) {
		<div class="space-y-6">
			<!-- Issue Header -->
			if issue != nil {
				<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
					<h1 class="text-2xl font-bold">{ issue.Issue.Title }</h1>
					<div class="mt-2 text-gray-500">{ issue.Issue.Identifier() }</div>
					<div class="mt-4 flex gap-2">
						for _, label := range issue.Issue.Labels {
							<span class="px-2 py-1 text-xs rounded-full bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">{ label }</span>
						}
					</div>
					<div class="mt-4 grid grid-cols-3 gap-4 text-sm">
						<div>
							<span class="text-gray-500">Status:</span>
							@StatusBadge("Running")
						</div>
						<div>
							<span class="text-gray-500">Priority:</span> { fmt.Sprintf("%d", issue.Issue.Priority) }
						</div>
						<div>
							<span class="text-gray-500">Attempt:</span> { fmt.Sprintf("%d", issue.Attempt) }
						</div>
					</div>
				</div>
			} else {
				<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
					<h1 class="text-2xl font-bold">{ fmt.Sprintf("%s#%d", repo, issueID) }</h1>
					<div class="mt-2 text-gray-500">Not currently running</div>
				</div>
			}

			<!-- Session History -->
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow">
				<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700">
					<h2 class="text-lg font-semibold">Session History</h2>
				</div>
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead class="bg-gray-50 dark:bg-gray-700">
							<tr>
								<th class="px-4 py-2 text-left">Session</th>
								<th class="px-4 py-2 text-left">Attempt</th>
								<th class="px-4 py-2 text-left">Duration</th>
								<th class="px-4 py-2 text-left">Exit Code</th>
								<th class="px-4 py-2 text-left">Error</th>
							</tr>
						</thead>
						<tbody>
							if len(history) == 0 {
								<tr>
									<td colspan="5" class="px-4 py-8 text-center text-gray-500">No session history</td>
								</tr>
							}
							for _, run := range history {
								<tr class="border-t border-gray-200 dark:border-gray-700">
									<td class="px-4 py-2 font-mono text-xs">{ run.SessionID }</td>
									<td class="px-4 py-2">{ fmt.Sprintf("%d", run.Attempt) }</td>
									<td class="px-4 py-2">{ formatDuration(run.Duration) }</td>
									<td class="px-4 py-2">
										if run.ExitCode == 0 {
											<span class="text-green-600">0</span>
										} else {
											<span class="text-red-600">{ fmt.Sprintf("%d", run.ExitCode) }</span>
										}
									</td>
									<td class="px-4 py-2 text-red-500">{ run.Error }</td>
								</tr>
							}
						</tbody>
					</table>
				</div>
			</div>
		</div>
	}
}
```

**Step 3: Add route and handler**

In `internal/web/server.go`, in `buildRouter()`:

```go
r.Get("/issues/{owner}/{repo}/{id}", s.handleIssueDetail)
```

Handler:
```go
func (s *Server) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid issue ID", http.StatusBadRequest)
		return
	}
	fullRepo := owner + "/" + repo
	running := s.state.GetRunning(id)
	history := s.state.GetHistory(id)

	component := pages.IssueDetail(running, history, id, fullRepo)
	component.Render(r.Context(), w)
}
```

Add `"strconv"` import.

**Step 4: Add JSON API endpoint**

In `buildRouter()`:

```go
r.Get("/issues/{owner}/{repo}/{id}", s.handleIssueDetailAPI)
```

Wait — this conflicts with the page route. Use the `/api/v1/` prefix:

```go
r.Route("/api/v1", func(r chi.Router) {
	// ... existing routes ...
	r.Get("/issues/{owner}/{repo}/{id}", s.handleIssueDetailAPI)
})
```

Handler:
```go
func (s *Server) handleIssueDetailAPI(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid issue ID", http.StatusBadRequest)
		return
	}
	_ = owner + "/" + repo

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running": s.state.GetRunning(id),
		"history": s.state.GetHistory(id),
	})
}
```

**Step 5: Run templ generate and tests**

Run: `go tool templ generate && go test -race ./...`
Expected: All pass

**Step 6: Commit**

```bash
git add internal/web/templates/pages/issue_detail.templ internal/web/server.go
git commit -m "feat: issue detail page with session history"
```

---

### Task 5: Sprint page

**Files:**
- Create: `internal/web/templates/pages/sprint.templ`
- Modify: `internal/web/server.go`

**Step 1: Create sprint.templ**

```go
package pages

import (
	"fmt"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

type SprintData struct {
	Iteration string
	ByStatus  map[string][]domain.Issue
	Total     int
	Done      int
}

templ Sprint(data SprintData) {
	@layouts.Base("Sprint") {
		<div class="space-y-6">
			<h1 class="text-2xl font-bold">
				if data.Iteration != "" {
					Sprint: { data.Iteration }
				} else {
					Current Sprint
				}
			</h1>

			<!-- Progress Bar -->
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
				<div class="flex justify-between text-sm mb-2">
					<span>Progress</span>
					<span>{ fmt.Sprintf("%d / %d", data.Done, data.Total) }</span>
				</div>
				<div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-3">
					if data.Total > 0 {
						<div class="bg-green-500 h-3 rounded-full" style={ fmt.Sprintf("width: %d%%", data.Done*100/data.Total) }></div>
					}
				</div>
			</div>

			<!-- Status Groups -->
			for _, status := range []string{"Todo", "In Progress", "In Review", "Done"} {
				<div class="bg-white dark:bg-gray-800 rounded-lg shadow">
					<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700 flex justify-between">
						<h2 class="text-lg font-semibold">{ status }</h2>
						<span class="text-gray-500">{ fmt.Sprintf("%d", len(data.ByStatus[status])) }</span>
					</div>
					<ul class="divide-y divide-gray-200 dark:divide-gray-700">
						if len(data.ByStatus[status]) == 0 {
							<li class="px-4 py-3 text-gray-500 text-sm">No issues</li>
						}
						for _, issue := range data.ByStatus[status] {
							<li class="px-4 py-3 flex justify-between items-center">
								<div>
									<a href={ templ.SafeURL(fmt.Sprintf("/issues/%s/%d", issue.Repo, issue.ID)) } class="text-blue-500 hover:underline">
										{ issue.Identifier() }
									</a>
									<span class="ml-2 text-sm">{ issue.Title }</span>
								</div>
								if issue.Priority > 0 {
									<span class="text-xs text-gray-500">P{ fmt.Sprintf("%d", issue.Priority) }</span>
								}
							</li>
						}
					</ul>
				</div>
			}
		</div>
	}
}
```

**Step 2: Add route and handler**

In `internal/web/server.go`, in `buildRouter()`:

```go
r.Get("/sprint", s.handleSprintPage)
```

Handler:
```go
func (s *Server) handleSprintPage(w http.ResponseWriter, r *http.Request) {
	// Build sprint data from running issues
	running := s.state.AllRunning()
	byStatus := map[string][]domain.Issue{
		"Todo": {}, "In Progress": {}, "In Review": {}, "Done": {},
	}
	iteration := ""
	for _, entry := range running {
		status := entry.Issue.Status
		if _, ok := byStatus[status]; !ok {
			status = "In Progress" // running agents are in progress
		}
		byStatus[status] = append(byStatus[status], entry.Issue)
		if entry.Issue.Iteration != "" {
			iteration = entry.Issue.Iteration
		}
	}

	total := 0
	for _, issues := range byStatus {
		total += len(issues)
	}

	data := pages.SprintData{
		Iteration: iteration,
		ByStatus:  byStatus,
		Total:     total,
		Done:      len(byStatus["Done"]),
	}

	component := pages.Sprint(data)
	component.Render(r.Context(), w)
}
```

Also add JSON API:

```go
// In /api/v1 route group:
r.Get("/sprint", s.handleSprintAPI)
```

```go
func (s *Server) handleSprintAPI(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running_count": len(running),
		"running":       running,
	})
}
```

**Step 3: Run templ generate and tests**

Run: `go tool templ generate && go test -race ./...`

**Step 4: Commit**

```bash
git add internal/web/templates/pages/sprint.templ internal/web/server.go
git commit -m "feat: sprint page with status grouping and progress bar"
```

---

### Task 6: Settings page

**Files:**
- Create: `internal/web/templates/pages/settings.templ`
- Modify: `internal/web/server.go`

**Step 1: Create settings.templ**

```go
package pages

import (
	"fmt"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

type SettingsData struct {
	Config      *config.Config
	Skills      []SkillDisplay
	RateLimit   RateLimitInfo
	AgentValid  map[string]bool
}

type SkillDisplay struct {
	Name        string
	Type        string
	Description string
}

type RateLimitInfo struct {
	Remaining int
	Reset     string
}

templ Settings(data SettingsData) {
	@layouts.Base("Settings") {
		<div class="space-y-6">
			<h1 class="text-2xl font-bold">Settings</h1>

			<!-- Config Display -->
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow">
				<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700">
					<h2 class="text-lg font-semibold">Configuration</h2>
				</div>
				<div class="p-4 space-y-3 text-sm">
					<div class="grid grid-cols-2 gap-2">
						<span class="text-gray-500">Repos:</span>
						<span>
							for i, repo := range data.Config.GitHub.Repos {
								if i > 0 {
									,
								}
								{ repo }
							}
						</span>
						<span class="text-gray-500">Poll Interval:</span>
						<span>{ fmt.Sprintf("%dms", data.Config.Polling.IntervalMS) }</span>
						<span class="text-gray-500">Max Concurrent Agents:</span>
						<span>{ fmt.Sprintf("%d", data.Config.Polling.MaxConcurrentAgents) }</span>
						<span class="text-gray-500">Agent Command:</span>
						<span class="font-mono">{ data.Config.Agent.Command }</span>
						<span class="text-gray-500">Max Retries:</span>
						<span>{ fmt.Sprintf("%d", data.Config.Agent.MaxRetries) }</span>
						<span class="text-gray-500">Stall Timeout:</span>
						<span>{ fmt.Sprintf("%dms", data.Config.Agent.StallTimeoutMS) }</span>
						<span class="text-gray-500">Workspace Root:</span>
						<span class="font-mono">{ data.Config.Workspace.Root }</span>
					</div>
				</div>
			</div>

			<!-- Skills -->
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow">
				<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700">
					<h2 class="text-lg font-semibold">Loaded Skills</h2>
				</div>
				<ul class="divide-y divide-gray-200 dark:divide-gray-700">
					if len(data.Skills) == 0 {
						<li class="px-4 py-3 text-gray-500 text-sm">No skills loaded</li>
					}
					for _, skill := range data.Skills {
						<li class="px-4 py-3 flex items-center gap-3">
							<span class="font-medium">{ skill.Name }</span>
							@SkillTypeBadge(skill.Type)
							<span class="text-sm text-gray-500">{ skill.Description }</span>
						</li>
					}
				</ul>
			</div>

			<!-- GitHub Status -->
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow">
				<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700">
					<h2 class="text-lg font-semibold">GitHub Connection</h2>
				</div>
				<div class="p-4 space-y-2 text-sm">
					<div class="flex items-center gap-2">
						<span class="text-gray-500">Token:</span>
						if data.Config.GitHub.Token != "" {
							<span class="text-green-600">Configured</span>
						} else {
							<span class="text-red-600">Missing</span>
						}
					</div>
					if data.RateLimit.Remaining > 0 {
						<div class="flex items-center gap-2">
							<span class="text-gray-500">Rate Limit Remaining:</span>
							<span>{ fmt.Sprintf("%d", data.RateLimit.Remaining) }</span>
						</div>
					}
				</div>
			</div>

			<!-- Agent Validation -->
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow">
				<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700">
					<h2 class="text-lg font-semibold">Agent Commands</h2>
				</div>
				<ul class="divide-y divide-gray-200 dark:divide-gray-700">
					for cmd, valid := range data.AgentValid {
						<li class="px-4 py-3 flex items-center gap-3">
							<span class="font-mono">{ cmd }</span>
							if valid {
								<span class="text-green-600 text-sm">Found in PATH</span>
							} else {
								<span class="text-red-600 text-sm">Not found</span>
							}
						</li>
					}
				</ul>
			</div>
		</div>
	}
}

templ SkillTypeBadge(skillType string) {
	switch skillType {
		case "rigid":
			<span class="px-2 py-0.5 text-xs rounded bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200">rigid</span>
		case "flexible":
			<span class="px-2 py-0.5 text-xs rounded bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">flexible</span>
		case "technique":
			<span class="px-2 py-0.5 text-xs rounded bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200">technique</span>
		default:
			<span class="px-2 py-0.5 text-xs rounded bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200">{ skillType }</span>
	}
}
```

**Step 2: Add route and handler**

In `buildRouter()`:
```go
r.Get("/settings", s.handleSettingsPage)
```

Handler:
```go
func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	// Validate agent commands
	agentValid := map[string]bool{}
	agentValid[s.cfg.Agent.Command] = isCommandAvailable(s.cfg.Agent.Command)
	for _, override := range s.cfg.Agent.Overrides {
		agentValid[override.Command] = isCommandAvailable(override.Command)
	}

	// Build skills list from loaded skills
	var skillDisplays []pages.SkillDisplay
	if s.skills != nil {
		for _, info := range s.skills.LoadedSkills() {
			skillDisplays = append(skillDisplays, pages.SkillDisplay{
				Name:        info.Name,
				Type:        info.Type,
				Description: info.Description,
			})
		}
	}

	data := pages.SettingsData{
		Config:     s.cfg,
		Skills:     skillDisplays,
		AgentValid: agentValid,
	}

	component := pages.Settings(data)
	component.Render(r.Context(), w)
}

func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
```

Add `"os/exec"` import.

Also add `skills SkillsProvider` to the Server struct and `NewServer` params. In the orchestrator, create a simple adapter that wraps the loaded skills slice.

**Step 3: Run templ generate and tests**

Run: `go tool templ generate && go test -race ./...`

**Step 4: Commit**

```bash
git add internal/web/templates/pages/settings.templ internal/web/server.go
git commit -m "feat: settings page with config, skills, GitHub status, agent validation"
```

---

### Task 7: SSE fragment rendering

**Files:**
- Modify: `internal/web/sse.go`
- Create: `internal/web/templates/fragments/dashboard_fragments.templ`
- Modify: `internal/orchestrator/orchestrator.go`

**Step 1: Create dashboard fragment templates**

In `internal/web/templates/fragments/dashboard_fragments.templ`:

```go
package fragments

import (
	"fmt"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/web/templates/pages"
)

templ AgentTable(running []*domain.RunEntry) {
	<tbody id="agent-table-body">
		if len(running) == 0 {
			<tr>
				<td colspan="6" class="px-4 py-8 text-center text-gray-500">No active agents</td>
			</tr>
		}
		for _, entry := range running {
			<tr class="border-t border-gray-200 dark:border-gray-700">
				<td class="px-4 py-2">{ entry.Issue.Identifier() }</td>
				<td class="px-4 py-2">{ entry.Issue.Repo }</td>
				<td class="px-4 py-2">
					@pages.StatusBadge("Running")
				</td>
				<td class="px-4 py-2">{ pages.FormatDuration(entry.Duration()) }</td>
				<td class="px-4 py-2">{ fmt.Sprintf("%d", entry.Attempt) }</td>
				<td class="px-4 py-2">{ entry.LastEventAt.Format("15:04:05") }</td>
			</tr>
		}
	</tbody>
}
```

Note: The `formatDuration` function from `pages` needs to be exported as `FormatDuration` for the fragment package to use it. Update it in `dashboard.templ`.

**Step 2: Update SSEHub to support HTML fragment broadcasting**

In `internal/web/sse.go`, add a method to broadcast rendered templ components:

```go
import (
	"bytes"
	"context"

	"github.com/a-h/templ"
)

func (h *SSEHub) BroadcastComponent(eventType string, component templ.Component) {
	var buf bytes.Buffer
	component.Render(context.Background(), &buf)
	h.Broadcast(eventType, buf.String())
}
```

**Step 3: Update orchestrator to broadcast fragments after tick**

In `internal/orchestrator/orchestrator.go`, after each tick where SSE is broadcast, render the actual fragment:

```go
if o.sseHub != nil {
	// For now, broadcast a simple refresh signal
	// The dashboard uses SSE to trigger HTMX swaps
	o.sseHub.Broadcast("agent-update", "refresh")
}
```

The HTMX SSE extension on the client will receive this event and can trigger a swap. The simplest approach: use `hx-trigger="sse:agent-update"` on the dashboard div to refetch the page content.

**Step 4: Commit**

```bash
git add internal/web/sse.go internal/web/templates/fragments/
git commit -m "feat: SSE fragment rendering support for live dashboard updates"
```

---

### Task 8: POST /api/v1/refresh endpoint

**Files:**
- Modify: `internal/web/server.go`
- Modify: `internal/orchestrator/orchestrator.go`

**Step 1: Add TriggerFunc to Server**

In `internal/web/server.go`:

```go
type Server struct {
	// ... existing fields ...
	triggerRefresh func()
}
```

Update `NewServer` to accept an optional refresh callback:

```go
func NewServer(state StateProvider, cfg *config.Config, metrics MetricsProvider, retries RetryProvider, opts ...ServerOption) *Server {
```

Or simpler, add a `SetRefreshFunc` method:

```go
func (s *Server) SetRefreshFunc(fn func()) {
	s.triggerRefresh = fn
}
```

**Step 2: Add the endpoint**

In `buildRouter()`:
```go
r.Post("/refresh", s.handleRefresh)
```

Handler:
```go
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if s.triggerRefresh != nil {
		s.triggerRefresh()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
}
```

**Step 3: Wire in orchestrator**

In `internal/orchestrator/orchestrator.go`, after creating the web server:

```go
webSrv.SetRefreshFunc(func() {
	go o.Tick(ctx)
})
```

**Step 4: Run tests**

Run: `go test -race ./...`

**Step 5: Commit**

```bash
git add internal/web/server.go internal/orchestrator/orchestrator.go
git commit -m "feat: POST /api/v1/refresh triggers immediate poll+reconcile"
```

This completes Section 3. All dashboard pages and API endpoints are implemented.
