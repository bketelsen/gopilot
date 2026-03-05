# Phase 5: Sprint & Analytics

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Token usage tracking, cost estimation, sprint view, metrics.

**Prerequisite:** Phase 4 complete.

---

### Task 5.1: Token Usage Tracking

**Files:**
- Create: `internal/metrics/tokens.go`
- Test: `internal/metrics/tokens_test.go`

**Step 1: Write the failing test**

```go
// internal/metrics/tokens_test.go
package metrics

import "testing"

func TestTokenTrackerRecord(t *testing.T) {
	tracker := NewTokenTracker()

	tracker.Record(42, TokenUsage{InputTokens: 1000, OutputTokens: 500})
	tracker.Record(42, TokenUsage{InputTokens: 2000, OutputTokens: 1000})
	tracker.Record(43, TokenUsage{InputTokens: 500, OutputTokens: 250})

	// Per-issue totals
	issue42 := tracker.ForIssue(42)
	if issue42.InputTokens != 3000 {
		t.Errorf("issue 42 input = %d, want 3000", issue42.InputTokens)
	}
	if issue42.OutputTokens != 1500 {
		t.Errorf("issue 42 output = %d, want 1500", issue42.OutputTokens)
	}

	// Aggregate totals
	totals := tracker.Totals()
	if totals.InputTokens != 3500 {
		t.Errorf("total input = %d, want 3500", totals.InputTokens)
	}
}

func TestCostEstimation(t *testing.T) {
	pricing := ModelPricing{
		InputPricePerMillion:  3.0,  // $3/M input
		OutputPricePerMillion: 15.0, // $15/M output
	}

	usage := TokenUsage{InputTokens: 1_000_000, OutputTokens: 100_000}
	cost := usage.EstimateCost(pricing)

	// $3 for input + $1.5 for output = $4.5
	if cost < 4.4 || cost > 4.6 {
		t.Errorf("cost = %f, want ~4.5", cost)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/metrics/...`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/metrics/tokens.go
package metrics

import "sync"

// TokenUsage records token consumption for a single session.
type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
}

// EstimateCost returns estimated USD cost using the given pricing.
func (t TokenUsage) EstimateCost(pricing ModelPricing) float64 {
	input := float64(t.InputTokens) / 1_000_000 * pricing.InputPricePerMillion
	output := float64(t.OutputTokens) / 1_000_000 * pricing.OutputPricePerMillion
	return input + output
}

// ModelPricing defines per-million-token prices.
type ModelPricing struct {
	InputPricePerMillion  float64
	OutputPricePerMillion float64
}

// DefaultPricing returns pricing for known models.
var DefaultPricing = map[string]ModelPricing{
	"claude-sonnet-4.6": {InputPricePerMillion: 3.0, OutputPricePerMillion: 15.0},
	"claude-opus-4.6":   {InputPricePerMillion: 15.0, OutputPricePerMillion: 75.0},
}

// TokenTracker accumulates token usage per-issue and in aggregate.
type TokenTracker struct {
	mu       sync.Mutex
	perIssue map[int]TokenUsage
	total    TokenUsage
}

// NewTokenTracker creates an empty tracker.
func NewTokenTracker() *TokenTracker {
	return &TokenTracker{
		perIssue: make(map[int]TokenUsage),
	}
}

// Record adds token usage for an issue.
func (t *TokenTracker) Record(issueID int, usage TokenUsage) {
	t.mu.Lock()
	defer t.mu.Unlock()

	existing := t.perIssue[issueID]
	existing.InputTokens += usage.InputTokens
	existing.OutputTokens += usage.OutputTokens
	t.perIssue[issueID] = existing

	t.total.InputTokens += usage.InputTokens
	t.total.OutputTokens += usage.OutputTokens
}

// ForIssue returns accumulated usage for a specific issue.
func (t *TokenTracker) ForIssue(issueID int) TokenUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.perIssue[issueID]
}

// Totals returns aggregate usage across all issues.
func (t *TokenTracker) Totals() TokenUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.total
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/metrics/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/metrics/
git commit -m "feat: token usage tracking with cost estimation"
```

---

### Task 5.2: Integrate Token Tracking into Orchestrator

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`

**Step 1: Add token tracker to orchestrator**

Add `tokenTracker *metrics.TokenTracker` field to `Orchestrator`.
Initialize in `NewOrchestrator`.
In `monitorAgent`, after agent exits, parse token usage from session output and call `tracker.Record()`.

Token parsing from Copilot CLI output (look for summary lines like `Tokens used: 1234 input, 567 output`):

```go
func parseTokenUsage(output string) metrics.TokenUsage {
	// Parse from agent output — format depends on agent adapter
	// Copilot CLI outputs a summary at the end
	var usage metrics.TokenUsage
	// Simple regex or string parsing for token counts
	// This is adapter-specific and may need refinement
	return usage
}
```

**Step 2: Verify build**

Run: `go build ./cmd/gopilot/`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat: integrate token tracking into agent lifecycle"
```

---

### Task 5.3: Sprint View Page

**Files:**
- Create: `internal/web/templates/pages/sprint.templ`
- Modify: `internal/web/server.go`

**Step 1: Create sprint template**

```go
// internal/web/templates/pages/sprint.templ
package pages

import (
	"fmt"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

type SprintData struct {
	Name          string
	Todo          []domain.Issue
	InProgress    []domain.Issue
	InReview      []domain.Issue
	Done          []domain.Issue
	TotalIssues   int
	DoneCount     int
	TokenCost     float64
}

templ Sprint(data SprintData) {
	@layouts.Base("Sprint") {
		<div class="space-y-6">
			<h2 class="text-2xl font-bold">{ data.Name }</h2>

			<!-- Progress bar -->
			<div class="w-full bg-secondary rounded-full h-4">
				<div class="bg-primary h-4 rounded-full" style={ fmt.Sprintf("width: %.0f%%", float64(data.DoneCount)/float64(max(data.TotalIssues, 1))*100) }></div>
			</div>
			<p class="text-sm text-muted-foreground">
				{ fmt.Sprintf("%d / %d issues complete", data.DoneCount, data.TotalIssues) }
				{ fmt.Sprintf(" — $%.2f token cost", data.TokenCost) }
			</p>

			<!-- Issue columns -->
			<div class="grid grid-cols-4 gap-4">
				@SprintColumn("Todo", data.Todo)
				@SprintColumn("In Progress", data.InProgress)
				@SprintColumn("In Review", data.InReview)
				@SprintColumn("Done", data.Done)
			</div>
		</div>
	}
}

templ SprintColumn(title string, issues []domain.Issue) {
	<div class="rounded-lg border border-border bg-card">
		<div class="p-3 border-b border-border">
			<h3 class="font-semibold">{ title } <span class="text-muted-foreground">{ fmt.Sprintf("(%d)", len(issues)) }</span></h3>
		</div>
		<div class="p-2 space-y-2">
			for _, issue := range issues {
				<div class="p-2 rounded border border-border text-sm">
					<span class="font-mono text-xs text-muted-foreground">#{ fmt.Sprintf("%d", issue.ID) }</span>
					<span class="ml-1">{ issue.Title }</span>
				</div>
			}
		</div>
	</div>
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

**Step 2: Add sprint route handler**

Add `GET /sprint` handler that fetches iteration data and renders the template.

**Step 3: Generate and build**

Run: `templ generate && go build ./...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add internal/web/
git commit -m "feat: sprint view page with progress bar and issue columns"
```

---

### Task 5.4: Metrics Counters

**Files:**
- Create: `internal/metrics/counters.go`
- Test: `internal/metrics/counters_test.go`

**Step 1: Write the failing test**

```go
// internal/metrics/counters_test.go
package metrics

import "testing"

func TestCounters(t *testing.T) {
	c := NewCounters()

	c.Increment("issues_dispatched")
	c.Increment("issues_dispatched")
	c.Increment("issues_completed")

	if c.Get("issues_dispatched") != 2 {
		t.Errorf("dispatched = %d, want 2", c.Get("issues_dispatched"))
	}
	if c.Get("issues_completed") != 1 {
		t.Errorf("completed = %d, want 1", c.Get("issues_completed"))
	}
	if c.Get("nonexistent") != 0 {
		t.Error("nonexistent counter should be 0")
	}

	all := c.All()
	if len(all) != 2 {
		t.Errorf("All() len = %d, want 2", len(all))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/metrics/... -run TestCounters`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/metrics/counters.go
package metrics

import "sync"

// Counters provides thread-safe named counters.
type Counters struct {
	mu   sync.RWMutex
	vals map[string]int64
}

// NewCounters creates an empty counter set.
func NewCounters() *Counters {
	return &Counters{vals: make(map[string]int64)}
}

// Increment adds 1 to a named counter.
func (c *Counters) Increment(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[name]++
}

// Add adds a value to a named counter.
func (c *Counters) Add(name string, val int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[name] += val
}

// Get returns the current value of a counter.
func (c *Counters) Get(name string) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.vals[name]
}

// All returns a snapshot of all counters.
func (c *Counters) All() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]int64, len(c.vals))
	for k, v := range c.vals {
		out[k] = v
	}
	return out
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/metrics/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/metrics/
git commit -m "feat: thread-safe metrics counters"
```

---

### Task 5.5: Wire Metrics into Orchestrator and Dashboard

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `internal/web/server.go`

**Step 1: Add counters to orchestrator**

Add `metrics *metrics.Counters` field. Increment on dispatch, completion, failure:

```go
o.metrics.Increment("issues_dispatched")
o.metrics.Increment("issues_completed")  // on success
o.metrics.Increment("issues_failed")     // on max retries
```

**Step 2: Add `/api/v1/metrics` endpoint**

```go
r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(s.metrics.All())
})
```

**Step 3: Update dashboard stats to use real metrics**

Pass counters into `DashboardStats` when rendering the dashboard page.

**Step 4: Verify build and tests**

Run: `go test -race ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/ internal/web/ internal/metrics/
git commit -m "feat: wire metrics counters into orchestrator and dashboard"
```

---

## Phase 5 Milestone

Run: `go test -race ./...` — all tests pass.

Analytics:
- Token usage tracked per-issue and in aggregate
- Cost estimation with configurable model pricing
- Sprint view with progress bar, issue columns, token cost
- Metrics counters for dispatched/completed/failed issues
- Metrics exposed via JSON API
