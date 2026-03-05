# Phase 0: Clean Slate Scaffold

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove existing implementation, set up fresh package structure, define domain model and interfaces.

**Prerequisite:** None — this is the first phase.

---

### Task 0.1: Clean Existing Implementation

**Files:**
- Delete: `internal/config/`
- Delete: `internal/orchestrator/`
- Delete: `internal/github/`
- Delete: `internal/agent/`
- Delete: `internal/workspace/`
- Delete: `internal/skill/`
- Delete: `internal/web/`
- Delete: `components/`
- Delete: `utils/`
- Keep: `cmd/gopilot/main.go` (will be rewritten in Phase 1)
- Keep: `go.mod`, `go.sum` (will be updated)
- Keep: `Taskfile.yml`
- Keep: `research/`, `docs/`, `skills/`

**Step 1: Remove old internal packages**

```bash
rm -rf internal/config internal/orchestrator internal/github internal/agent internal/workspace internal/skill internal/web components utils
```

**Step 2: Create new package directories**

```bash
mkdir -p internal/{config,github,orchestrator,workspace,agent,skills,web/templates/{layouts,pages,components}}
```

**Step 3: Verify clean state**

Run: `go vet ./cmd/gopilot/...` — Expected: compilation errors (imports removed packages). That's fine, we'll fix main.go in Phase 1.

**Step 4: Commit**

```bash
git add -A
git commit -m "chore: clean slate for full rewrite

Remove all existing internal packages. Fresh package structure
created per spec Section 15. Existing code preserved in git history."
```

---

### Task 0.2: Domain Model — Issue Type

**Files:**
- Create: `internal/domain/types.go`
- Test: `internal/domain/types_test.go`

**Step 1: Write the failing test**

```go
// internal/domain/types_test.go
package domain

import (
	"testing"
	"time"
)

func TestIssueIdentifier(t *testing.T) {
	issue := Issue{
		ID:   42,
		Repo: "owner/repo",
	}
	if got := issue.Identifier(); got != "owner/repo#42" {
		t.Errorf("Identifier() = %q, want %q", got, "owner/repo#42")
	}
}

func TestIssueIsTerminal(t *testing.T) {
	tests := []struct {
		status   string
		terminal bool
	}{
		{"Todo", false},
		{"In Progress", false},
		{"In Review", false},
		{"Done", true},
		{"Closed", true},
		{"Canceled", true},
	}
	for _, tt := range tests {
		issue := Issue{Status: tt.status}
		if got := issue.IsTerminal(); got != tt.terminal {
			t.Errorf("IsTerminal() for status %q = %v, want %v", tt.status, got, tt.terminal)
		}
	}
}

func TestIssueIsEligible(t *testing.T) {
	issue := Issue{
		ID:     1,
		Repo:   "owner/repo",
		Labels: []string{"gopilot"},
		Status: "Todo",
	}
	eligible := []string{"gopilot", "autopilot"}
	excluded := []string{"blocked", "needs-design"}

	if !issue.IsEligible(eligible, excluded) {
		t.Error("expected issue to be eligible")
	}

	// No eligible label
	issue.Labels = []string{"other"}
	if issue.IsEligible(eligible, excluded) {
		t.Error("expected issue without eligible label to be ineligible")
	}

	// Has excluded label
	issue.Labels = []string{"gopilot", "blocked"}
	if issue.IsEligible(eligible, excluded) {
		t.Error("expected issue with excluded label to be ineligible")
	}

	// Wrong status
	issue.Labels = []string{"gopilot"}
	issue.Status = "In Progress"
	if issue.IsEligible(eligible, excluded) {
		t.Error("expected issue with non-Todo status to be ineligible")
	}
}

func TestPrioritySort(t *testing.T) {
	now := time.Now()
	issues := []Issue{
		{ID: 1, Priority: 4, CreatedAt: now},                       // low
		{ID: 2, Priority: 1, CreatedAt: now},                       // urgent
		{ID: 3, Priority: 0, CreatedAt: now},                       // none (last)
		{ID: 4, Priority: 1, CreatedAt: now.Add(-time.Hour)},       // urgent, older
	}
	SortByPriority(issues)

	expected := []int{4, 2, 1, 3} // urgent-older, urgent-newer, low, none
	for i, want := range expected {
		if issues[i].ID != want {
			t.Errorf("position %d: got ID %d, want %d", i, issues[i].ID, want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/domain/...`
Expected: FAIL — package doesn't exist yet.

**Step 3: Write minimal implementation**

```go
// internal/domain/types.go
package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Issue represents a normalized GitHub issue enriched with Projects v2 fields.
type Issue struct {
	// Identity
	ID     int    // GitHub issue number
	NodeID string // GitHub GraphQL node ID
	Repo   string // "owner/repo"
	URL    string // Full GitHub URL

	// Content
	Title     string
	Body      string
	Labels    []string // lowercase
	Assignees []string

	// Hierarchy
	ParentID  *int  // Parent issue number (sub-issues)
	ChildIDs  []int // Child issue numbers
	BlockedBy []int // Issues blocking this one

	// Project fields (from Projects v2)
	Status    string // Todo, In Progress, In Review, Done, Closed, Canceled
	Priority  int    // 0=none, 1=urgent, 2=high, 3=medium, 4=low
	Iteration string // Sprint/iteration name
	Effort    int    // Story points

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Identifier returns the "owner/repo#N" string for logging.
func (i Issue) Identifier() string {
	return fmt.Sprintf("%s#%d", i.Repo, i.ID)
}

// IsTerminal returns true if the issue is in a terminal state.
func (i Issue) IsTerminal() bool {
	switch i.Status {
	case "Done", "Closed", "Canceled":
		return true
	}
	return false
}

// IsEligible checks whether the issue can be dispatched.
// Requires at least one eligible label, no excluded labels, and Status "Todo".
func (i Issue) IsEligible(eligible, excluded []string) bool {
	if i.Status != "Todo" {
		return false
	}

	hasEligible := false
	for _, label := range i.Labels {
		for _, el := range eligible {
			if strings.EqualFold(label, el) {
				hasEligible = true
			}
		}
		for _, ex := range excluded {
			if strings.EqualFold(label, ex) {
				return false
			}
		}
	}
	return hasEligible
}

// SortByPriority sorts issues by priority (1=urgent first, 0=none last),
// then by CreatedAt (oldest first).
func SortByPriority(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		pi, pj := issues[i].Priority, issues[j].Priority
		// 0 means "none" — sort last
		if pi == 0 && pj != 0 {
			return false
		}
		if pi != 0 && pj == 0 {
			return true
		}
		if pi != pj {
			return pi < pj
		}
		return issues[i].CreatedAt.Before(issues[j].CreatedAt)
	})
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/domain/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/
git commit -m "feat: add domain model — Issue type with eligibility and sorting"
```

---

### Task 0.3: Domain Model — Runtime State Types

**Files:**
- Modify: `internal/domain/types.go`
- Test: `internal/domain/types_test.go`

**Step 1: Write the failing test**

```go
// Append to internal/domain/types_test.go

func TestRunEntryDuration(t *testing.T) {
	entry := RunEntry{
		StartedAt: time.Now().Add(-5 * time.Minute),
	}
	d := entry.Duration()
	if d < 4*time.Minute || d > 6*time.Minute {
		t.Errorf("Duration() = %v, want ~5m", d)
	}
}

func TestRunEntryIsStalled(t *testing.T) {
	timeout := 5 * time.Minute
	fresh := RunEntry{LastEventAt: time.Now()}
	if fresh.IsStalled(timeout) {
		t.Error("fresh entry should not be stalled")
	}

	stale := RunEntry{LastEventAt: time.Now().Add(-10 * time.Minute)}
	if !stale.IsStalled(timeout) {
		t.Error("stale entry should be stalled")
	}
}

func TestTokenCountsAdd(t *testing.T) {
	a := TokenCounts{InputTokens: 100, OutputTokens: 50}
	b := TokenCounts{InputTokens: 200, OutputTokens: 100}
	sum := a.Add(b)
	if sum.InputTokens != 300 || sum.OutputTokens != 150 || sum.TotalTokens != 450 {
		t.Errorf("Add() = %+v, want {300, 150, 450}", sum)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/domain/...`
Expected: FAIL — `RunEntry`, `TokenCounts` not defined.

**Step 3: Write minimal implementation**

```go
// Append to internal/domain/types.go

// RunEntry tracks an active agent session.
type RunEntry struct {
	Issue       Issue
	SessionID   string
	ProcessPID  int
	StartedAt   time.Time
	LastEventAt time.Time
	LastEvent   string
	LastMessage string
	TurnCount   int
	Attempt     int
	Tokens      TokenCounts
}

// Duration returns time since the agent started.
func (r RunEntry) Duration() time.Duration {
	return time.Since(r.StartedAt)
}

// IsStalled returns true if no events received within the timeout.
func (r RunEntry) IsStalled(timeout time.Duration) bool {
	return time.Since(r.LastEventAt) > timeout
}

// RetryEntry tracks an issue waiting for retry.
type RetryEntry struct {
	IssueID    int
	Identifier string // "owner/repo#42"
	Attempt    int
	DueAt      time.Time
	Error      string
}

// TokenCounts tracks token usage for a session.
type TokenCounts struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// Add returns the sum of two TokenCounts.
func (t TokenCounts) Add(other TokenCounts) TokenCounts {
	return TokenCounts{
		InputTokens:  t.InputTokens + other.InputTokens,
		OutputTokens: t.OutputTokens + other.OutputTokens,
		TotalTokens:  t.InputTokens + other.InputTokens + t.OutputTokens + other.OutputTokens,
	}
}

// TokenTotals extends TokenCounts with aggregate metrics.
type TokenTotals struct {
	TokenCounts
	SecondsRunning float64
	CostEstimate   float64 // estimated USD
}

// AgentEvent represents an event from a running agent.
type AgentEvent struct {
	Type      string // agent_started, agent_output, agent_completed, agent_failed, agent_timeout
	SessionID string
	IssueID   int
	Message   string
	Timestamp time.Time
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/domain/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/
git commit -m "feat: add runtime state types — RunEntry, RetryEntry, TokenCounts"
```

---

### Task 0.4: Interfaces — AgentRunner

**Files:**
- Create: `internal/agent/runner.go`

**Step 1: Write the interface** (no test needed — interfaces have no behavior)

```go
// internal/agent/runner.go
package agent

import "context"

// Session represents a running agent subprocess.
type Session struct {
	ID         string
	PID        int
	Cancel     context.CancelFunc
	Done       <-chan struct{} // closed when process exits
	ExitCode   int
	ExitErr    error
}

// AgentOpts configures an agent launch.
type AgentOpts struct {
	Model            string
	MaxContinuations int
	Env              []string
}

// Runner launches and manages agent subprocesses.
type Runner interface {
	// Start launches an agent in the workspace with the given prompt.
	Start(ctx context.Context, workspace string, prompt string, opts AgentOpts) (*Session, error)

	// Stop terminates a running session (SIGTERM → wait → SIGKILL).
	Stop(session *Session) error

	// Name returns the adapter name for logging.
	Name() string
}
```

**Step 2: Commit**

```bash
git add internal/agent/runner.go
git commit -m "feat: define AgentRunner interface"
```

---

### Task 0.5: Interfaces — GitHub Client

**Files:**
- Create: `internal/github/client.go`

**Step 1: Write the interface**

```go
// internal/github/client.go
package github

import (
	"context"

	"github.com/bketelsen/gopilot/internal/domain"
)

// Client defines the GitHub operations the orchestrator needs.
type Client interface {
	// FetchCandidateIssues returns open issues matching eligibility criteria.
	FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error)

	// FetchIssueState returns the current state of a single issue.
	FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error)

	// FetchIssueStates returns current state of multiple issues in batch.
	FetchIssueStates(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error)

	// SetProjectStatus sets the Status field on an issue in Projects v2.
	SetProjectStatus(ctx context.Context, issue domain.Issue, status string) error

	// AddComment posts a comment on an issue.
	AddComment(ctx context.Context, repo string, id int, body string) error

	// AddLabel adds a label to an issue.
	AddLabel(ctx context.Context, repo string, id int, label string) error
}
```

**Step 2: Commit**

```bash
git add internal/github/client.go
git commit -m "feat: define GitHub client interface"
```

---

### Task 0.6: Interfaces — Workspace Manager

**Files:**
- Create: `internal/workspace/manager.go`

**Step 1: Write the interface**

```go
// internal/workspace/manager.go
package workspace

import (
	"context"

	"github.com/bketelsen/gopilot/internal/domain"
)

// Manager handles per-issue workspace lifecycle.
type Manager interface {
	// Ensure creates the workspace if it doesn't exist, returns the path.
	Ensure(ctx context.Context, issue domain.Issue) (string, error)

	// RunHook executes a named lifecycle hook in the workspace directory.
	RunHook(ctx context.Context, hook string, workspacePath string, issue domain.Issue) error

	// Cleanup removes a workspace directory after running before_remove hook.
	Cleanup(ctx context.Context, issue domain.Issue) error

	// Path returns the workspace path for an issue (without creating it).
	Path(issue domain.Issue) string
}
```

**Step 2: Commit**

```bash
git add internal/workspace/manager.go
git commit -m "feat: define Workspace Manager interface"
```

---

### Task 0.7: Update go.mod with new dependencies

**Step 1: Add chi and fsnotify**

```bash
cd /home/debian/gopilot
go get github.com/go-chi/chi/v5@latest
go get github.com/fsnotify/fsnotify@latest
go mod tidy
```

**Step 2: Verify**

Run: `cat go.mod` — should show chi and fsnotify in require block.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add chi and fsnotify dependencies"
```

---

### Task 0.8: Update Taskfile for templ + tailwind build chain

**Files:**
- Modify: `Taskfile.yml`

**Step 1: Update Taskfile**

```yaml
version: '3'

vars:
  VERSION:
    sh: git describe --tags --always --dirty 2>/dev/null || echo "dev"

tasks:
  generate:
    desc: Generate templ files
    cmds:
      - templ generate

  css:
    desc: Build Tailwind CSS
    cmds:
      - npx @tailwindcss/cli -i internal/web/templates/input.css -o internal/web/static/styles.css --minify

  build:
    desc: Build the gopilot binary
    deps: [generate]
    cmds:
      - go build -ldflags "-X main.Version={{.VERSION}}" -o gopilot ./cmd/gopilot

  test:
    desc: Run all tests
    cmds:
      - go test -race ./...

  lint:
    desc: Run go vet
    cmds:
      - go vet ./...

  clean:
    desc: Remove build artifacts
    cmds:
      - rm -f gopilot

  dev:
    desc: Build and run
    deps: [build]
    cmds:
      - ./gopilot {{.CLI_ARGS}}
```

**Step 2: Commit**

```bash
git add Taskfile.yml
git commit -m "chore: update Taskfile for templ + tailwind build chain"
```

---

## Phase 0 Milestone

Fresh package structure with:
- Domain types: `Issue`, `RunEntry`, `RetryEntry`, `TokenCounts` with tests
- Interfaces: `agent.Runner`, `github.Client`, `workspace.Manager`
- Dependencies: chi, fsnotify added
- Build chain: templ generate → go build

All tests pass. Ready for Phase 1.
