# Phase 2: Reliability

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Retry queue with exponential backoff, stall detection, reconciliation against GitHub, config hot-reload.

**Prerequisite:** Phase 1 complete (orchestrator dispatches agents).

---

### Task 2.1: Retry Queue with Exponential Backoff

**Files:**
- Create: `internal/orchestrator/retry.go`
- Test: `internal/orchestrator/retry_test.go`

**Step 1: Write the failing test**

```go
// internal/orchestrator/retry_test.go
package orchestrator

import (
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	maxBackoff := 300 * time.Second // 5 min cap

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 20 * time.Second},      // 10s * 2^1 = 20s
		{2, 40 * time.Second},      // 10s * 2^2 = 40s
		{3, 80 * time.Second},      // 10s * 2^3 = 80s
		{4, 160 * time.Second},     // 10s * 2^4 = 160s
		{5, 300 * time.Second},     // 10s * 2^5 = 320s → capped at 300s
		{10, 300 * time.Second},    // capped
	}

	for _, tt := range tests {
		got := BackoffDelay(tt.attempt, maxBackoff)
		if got != tt.want {
			t.Errorf("BackoffDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestRetryQueueEnqueueAndDue(t *testing.T) {
	q := NewRetryQueue()

	q.Enqueue(42, "o/r#42", 1, "agent crashed", 300*time.Second)
	q.Enqueue(43, "o/r#43", 2, "timeout", 300*time.Second)

	// Nothing should be due yet (backoff is at least 20s)
	due := q.DueEntries()
	if len(due) != 0 {
		t.Errorf("expected 0 due entries, got %d", len(due))
	}

	// Manually set DueAt to past
	q.mu.Lock()
	for _, e := range q.entries {
		e.DueAt = time.Now().Add(-1 * time.Second)
	}
	q.mu.Unlock()

	due = q.DueEntries()
	if len(due) != 2 {
		t.Errorf("expected 2 due entries, got %d", len(due))
	}
}

func TestRetryQueueRemove(t *testing.T) {
	q := NewRetryQueue()
	q.Enqueue(42, "o/r#42", 1, "err", 300*time.Second)
	q.Remove(42)

	if q.Has(42) {
		t.Error("entry should be removed")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/... -run TestBackoff`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/orchestrator/retry.go
package orchestrator

import (
	"math"
	"sync"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

const baseDelay = 10 * time.Second

// BackoffDelay calculates exponential backoff: min(10s * 2^attempt, maxBackoff).
func BackoffDelay(attempt int, maxBackoff time.Duration) time.Duration {
	delay := baseDelay * time.Duration(math.Pow(2, float64(attempt)))
	if delay > maxBackoff {
		delay = maxBackoff
	}
	return delay
}

// RetryQueue manages issues waiting for retry with backoff.
type RetryQueue struct {
	mu      sync.Mutex
	entries map[int]*domain.RetryEntry
}

// NewRetryQueue creates an empty retry queue.
func NewRetryQueue() *RetryQueue {
	return &RetryQueue{
		entries: make(map[int]*domain.RetryEntry),
	}
}

// Enqueue adds an issue to the retry queue with calculated backoff.
func (q *RetryQueue) Enqueue(issueID int, identifier string, attempt int, errMsg string, maxBackoff time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delay := BackoffDelay(attempt, maxBackoff)
	q.entries[issueID] = &domain.RetryEntry{
		IssueID:    issueID,
		Identifier: identifier,
		Attempt:    attempt,
		DueAt:      time.Now().Add(delay),
		Error:      errMsg,
	}
}

// DueEntries returns entries where DueAt <= now, removing them from the queue.
func (q *RetryQueue) DueEntries() []*domain.RetryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var due []*domain.RetryEntry
	for id, entry := range q.entries {
		if entry.DueAt.Before(now) || entry.DueAt.Equal(now) {
			due = append(due, entry)
			delete(q.entries, id)
		}
	}
	return due
}

// Remove removes an entry from the queue.
func (q *RetryQueue) Remove(issueID int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.entries, issueID)
}

// Has returns whether an issue is in the retry queue.
func (q *RetryQueue) Has(issueID int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.entries[issueID]
	return ok
}

// All returns all entries (for display).
func (q *RetryQueue) All() []*domain.RetryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := make([]*domain.RetryEntry, 0, len(q.entries))
	for _, e := range q.entries {
		entries = append(entries, e)
	}
	return entries
}

// Len returns the number of entries.
func (q *RetryQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/orchestrator/... -run TestBackoff`
Expected: PASS

Run: `go test -race ./internal/orchestrator/... -run TestRetryQueue`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/retry.go internal/orchestrator/retry_test.go
git commit -m "feat: retry queue with exponential backoff"
```

---

### Task 2.2: Integrate Retry into Orchestrator

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the failing test**

```go
// Append to internal/orchestrator/orchestrator_test.go

func TestOrchestratorRetryOnAgentFailure(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}

	// Agent that fails immediately
	failAgent := &mockFailAgent{}
	orch := NewOrchestrator(cfg, gh, failAgent)

	ctx := context.Background()
	orch.Tick(ctx)

	// Wait for agent to "fail"
	time.Sleep(100 * time.Millisecond)

	// Should be in retry queue
	if orch.retryQueue.Len() != 1 {
		t.Errorf("retry queue len = %d, want 1", orch.retryQueue.Len())
	}
}

type mockFailAgent struct{}

func (m *mockFailAgent) Name() string { return "mock-fail" }
func (m *mockFailAgent) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	done := make(chan struct{})
	close(done) // exit immediately
	return &agent.Session{
		ID: "fail-session", PID: 99999, Done: done,
		ExitCode: 1, ExitErr: fmt.Errorf("crashed"),
		Cancel: func() {},
	}, nil
}
func (m *mockFailAgent) Stop(sess *agent.Session) error { return nil }
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/... -run TestOrchestratorRetry`
Expected: FAIL — `retryQueue` field not on Orchestrator.

**Step 3: Update orchestrator to include retry queue**

Add to `Orchestrator` struct in `orchestrator.go`:
- `retryQueue *RetryQueue` field
- Initialize in `NewOrchestrator`: `retryQueue: NewRetryQueue()`
- In `Tick()`: process due retries before dispatching new candidates
- In `monitorAgent()`: on non-zero exit, enqueue for retry

Key changes to `orchestrator.go`:

```go
// In Orchestrator struct, add:
retryQueue *RetryQueue

// In NewOrchestrator, add:
retryQueue: NewRetryQueue(),

// In Tick(), after reconciliation, add retry processing:
// 2. PROCESS RETRY QUEUE
dueRetries := o.retryQueue.DueEntries()
for _, entry := range dueRetries {
    if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
        // Re-enqueue with same attempt (don't increment)
        o.retryQueue.Enqueue(entry.IssueID, entry.Identifier, entry.Attempt, entry.Error,
            time.Duration(o.cfg.Agent.MaxRetryBackoffMS)*time.Millisecond)
        break
    }
    // Re-fetch issue state
    issue, err := o.github.FetchIssueState(ctx, issueRepoFromIdentifier(entry.Identifier), entry.IssueID)
    if err != nil || issue == nil {
        slog.Warn("retry: could not fetch issue", "issue_id", entry.IssueID, "error", err)
        o.state.Release(entry.IssueID)
        continue
    }
    if !issue.IsEligible(o.cfg.GitHub.EligibleLabels, o.cfg.GitHub.ExcludedLabels) {
        slog.Info("retry: issue no longer eligible", "issue", entry.Identifier)
        o.state.Release(entry.IssueID)
        continue
    }
    o.dispatch(ctx, *issue, entry.Attempt)
}

// In monitorAgent(), on failure:
if sess.ExitCode != 0 {
    errMsg := "exit code " + strconv.Itoa(sess.ExitCode)
    if sess.ExitErr != nil {
        errMsg = sess.ExitErr.Error()
    }
    if entry.Attempt < o.cfg.Agent.MaxRetries {
        maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
        o.retryQueue.Enqueue(issue.ID, issue.Identifier(), entry.Attempt+1, errMsg, maxBackoff)
        log.Info("scheduled retry", "next_attempt", entry.Attempt+1)
    } else {
        o.handleMaxRetriesExceeded(issue, errMsg)
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat: integrate retry queue into orchestrator dispatch loop"
```

---

### Task 2.3: Stall Detection

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the failing test**

```go
// Append to orchestrator_test.go

func TestOrchestratorStallDetection(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000,
			StallTimeoutMS: 1, // 1ms — everything is stalled immediately
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, ag)

	ctx := context.Background()
	orch.Tick(ctx) // dispatch

	time.Sleep(50 * time.Millisecond) // let it become "stalled"

	orch.detectStalls(ctx) // should detect and kill

	if orch.state.RunningCount() != 0 {
		t.Errorf("running = %d, want 0 after stall detection", orch.state.RunningCount())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/... -run TestOrchestratorStall`
Expected: FAIL — `detectStalls` not defined.

**Step 3: Add stall detection method**

```go
// Add to orchestrator.go

func (o *Orchestrator) detectStalls(ctx context.Context) {
	timeout := o.cfg.StallTimeout()
	for _, entry := range o.state.AllRunning() {
		if entry.IsStalled(timeout) {
			log := slog.With("issue", entry.Issue.Identifier(), "session_id", entry.SessionID)
			log.Warn("agent stalled, killing", "last_event", entry.LastEventAt)

			// Kill via agent runner
			sess := o.findSession(entry)
			if sess != nil {
				o.agent.Stop(sess)
			}

			o.state.RemoveRunning(entry.Issue.ID)

			// Comment on issue
			duration := time.Since(entry.StartedAt).Round(time.Second)
			comment := fmt.Sprintf("Agent stalled after %s, retrying (attempt %d)", duration, entry.Attempt)
			o.github.AddComment(ctx, entry.Issue.Repo, entry.Issue.ID, comment)

			// Schedule retry
			if entry.Attempt < o.cfg.Agent.MaxRetries {
				maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
				o.retryQueue.Enqueue(entry.Issue.ID, entry.Issue.Identifier(), entry.Attempt+1, "stalled", maxBackoff)
			} else {
				o.handleMaxRetriesExceeded(entry.Issue, "stalled")
			}
		}
	}
}
```

Also add a `sessions` map to track `*agent.Session` per issue ID, and `handleMaxRetriesExceeded`:

```go
func (o *Orchestrator) handleMaxRetriesExceeded(issue domain.Issue, lastError string) {
	log := slog.With("issue", issue.Identifier())
	log.Error("max retries exceeded", "attempts", o.cfg.Agent.MaxRetries, "last_error", lastError)

	o.state.Release(issue.ID)

	comment := fmt.Sprintf("Gopilot failed after %d attempts. Last error: %s", o.cfg.Agent.MaxRetries, lastError)
	o.github.AddComment(context.Background(), issue.Repo, issue.ID, comment)
	o.github.AddLabel(context.Background(), issue.Repo, issue.ID, "gopilot-failed")
}
```

Call `o.detectStalls(ctx)` in `Tick()` after reconciliation.

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat: stall detection with retry scheduling"
```

---

### Task 2.4: Reconciliation Against GitHub State

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the failing test**

```go
// Append to orchestrator_test.go

func TestReconcileTerminalIssue(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	// Issue starts as Todo, dispatches, then becomes Done externally
	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, ag)

	ctx := context.Background()
	orch.Tick(ctx) // dispatch issue 1

	// Simulate external state change: issue moved to Done
	gh.issues[0].Status = "Done"

	orch.reconcile(ctx) // should detect terminal state

	// Agent should be stopped, not running
	if orch.state.RunningCount() != 0 {
		t.Errorf("running = %d, want 0 after reconciling terminal issue", orch.state.RunningCount())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/... -run TestReconcile`
Expected: FAIL — `reconcile` not defined.

**Step 3: Add reconciliation method**

```go
// Add to orchestrator.go

func (o *Orchestrator) reconcile(ctx context.Context) {
	for _, entry := range o.state.AllRunning() {
		issue, err := o.github.FetchIssueState(ctx, entry.Issue.Repo, entry.Issue.ID)
		if err != nil {
			slog.Warn("reconcile: fetch failed", "issue", entry.Issue.Identifier(), "error", err)
			continue
		}
		if issue == nil {
			continue
		}

		log := slog.With("issue", entry.Issue.Identifier())

		if issue.IsTerminal() {
			log.Info("reconcile: issue became terminal, stopping agent", "status", issue.Status)
			o.stopAndCleanup(ctx, entry, true) // cleanup workspace
			continue
		}

		if !issue.IsEligible(o.cfg.GitHub.EligibleLabels, o.cfg.GitHub.ExcludedLabels) {
			log.Info("reconcile: issue no longer eligible, stopping agent", "status", issue.Status, "labels", issue.Labels)
			o.stopAndCleanup(ctx, entry, false) // keep workspace
			continue
		}

		// Update local state with latest data
		entry.Issue = *issue
	}
}

func (o *Orchestrator) stopAndCleanup(ctx context.Context, entry *domain.RunEntry, removeWorkspace bool) {
	sess := o.findSession(entry)
	if sess != nil {
		o.agent.Stop(sess)
	}

	o.state.RemoveRunning(entry.Issue.ID)
	o.state.Release(entry.Issue.ID)

	o.workspace.RunHook(ctx, "after_run", o.workspace.Path(entry.Issue), entry.Issue)

	if removeWorkspace {
		o.workspace.Cleanup(ctx, entry.Issue)
	}
}
```

Call `o.reconcile(ctx)` at the start of `Tick()`.

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat: reconciliation against GitHub state — stop agents for terminal/ineligible issues"
```

---

### Task 2.5: Config Hot-Reload

**Files:**
- Create: `internal/config/watcher.go`
- Test: `internal/config/watcher_test.go`

**Step 1: Write the failing test**

```go
// internal/config/watcher_test.go
package config

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")

	yaml := `
github:
  token: test-token
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
polling: {interval_ms: 30000}
`
	os.WriteFile(path, []byte(yaml), 0644)

	var reloadCount atomic.Int32
	w, err := Watch(path, func(cfg *Config, err error) {
		reloadCount.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(100 * time.Millisecond) // let watcher start

	// Modify the file
	newYaml := `
github:
  token: test-token
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
polling: {interval_ms: 15000}
`
	os.WriteFile(path, []byte(newYaml), 0644)

	time.Sleep(500 * time.Millisecond) // wait for event

	if reloadCount.Load() < 1 {
		t.Error("expected at least 1 reload callback")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/config/... -run TestWatcher`
Expected: FAIL — `Watch` not defined.

**Step 3: Write minimal implementation**

```go
// internal/config/watcher.go
package config

import (
	"log/slog"

	"github.com/fsnotify/fsnotify"
)

// ReloadCallback is called when config changes. err is non-nil if the new config is invalid.
type ReloadCallback func(cfg *Config, err error)

// Watcher watches a config file for changes and triggers reloads.
type Watcher struct {
	fsw *fsnotify.Watcher
}

// Watch starts watching the config file at path and calls cb on changes.
func Watch(path string, cb ReloadCallback) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := fsw.Add(path); err != nil {
		fsw.Close()
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-fsw.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					slog.Info("config file changed, reloading", "path", path)
					cfg, err := Load(path)
					cb(cfg, err)
				}
			case err, ok := <-fsw.Errors:
				if !ok {
					return
				}
				slog.Error("config watcher error", "error", err)
			}
		}
	}()

	return &Watcher{fsw: fsw}, nil
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	return w.fsw.Close()
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/config/... -run TestWatcher`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/watcher.go internal/config/watcher_test.go
git commit -m "feat: config file watcher with fsnotify hot-reload"
```

---

### Task 2.6: Integrate Hot-Reload into Orchestrator

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`

**Step 1: Add config reload handling to orchestrator**

In `Run()`, start the config watcher and apply safe field updates:

```go
// In Run(), after initial tick, start watcher:
watcher, err := config.Watch(configPath, func(newCfg *config.Config, err error) {
    if err != nil {
        slog.Error("config reload failed, keeping current config", "error", err)
        return
    }
    slog.Info("config reloaded successfully")
    // Apply safe fields only
    o.cfg.Polling.IntervalMS = newCfg.Polling.IntervalMS
    o.cfg.Polling.MaxConcurrentAgents = newCfg.Polling.MaxConcurrentAgents
    o.cfg.Agent.StallTimeoutMS = newCfg.Agent.StallTimeoutMS
    o.cfg.Agent.TurnTimeoutMS = newCfg.Agent.TurnTimeoutMS
    o.cfg.Agent.MaxRetries = newCfg.Agent.MaxRetries
    o.cfg.Agent.MaxRetryBackoffMS = newCfg.Agent.MaxRetryBackoffMS
    o.cfg.Agent.MaxAutopilotContinues = newCfg.Agent.MaxAutopilotContinues
    o.cfg.Skills = newCfg.Skills
    o.cfg.Prompt = newCfg.Prompt
    // Note: token, repos, workspace.root NOT reloaded — require restart
})
if watcher != nil {
    defer watcher.Close()
}
```

Add `configPath` parameter to `NewOrchestrator` or `Run`.

**Step 2: Verify compilation**

Run: `go build ./cmd/gopilot/`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add internal/orchestrator/ cmd/gopilot/
git commit -m "feat: integrate config hot-reload into orchestrator"
```

---

## Phase 2 Milestone

Run: `go test -race ./...` — all tests pass.

The orchestrator now:
- Retries failed agents with exponential backoff (10s * 2^attempt, capped)
- Detects stalled agents (no output for stall_timeout_ms) and kills them
- Reconciles running agents against GitHub state (stops on terminal/ineligible)
- Handles max retries (comments, labels with `gopilot-failed`)
- Hot-reloads config on file change (safe fields only)
