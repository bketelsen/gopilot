# PR-Aware Issue Filtering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prevent agents from working on issues that already have open pull requests, and stop running agents when a PR appears.

**Architecture:** Add PR enrichment to the orchestrator's `Tick` candidate loop (using existing `FetchLinkedPullRequests`) and a PR check to `reconcile()`. All domain types and GitHub client methods already exist — this wires them into the orchestrator.

**Tech Stack:** Go, existing `domain.HasOpenPR()`, existing `github.Client.FetchLinkedPullRequests()`

---

### Task 1: Add PR-aware mock to test infrastructure

**Files:**
- Modify: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the `mockGitHubLinkedPRs` struct**

Add a new mock that extends `mockGitHub` with configurable linked PR responses. Add this after the existing `mockGitHubPR` struct (around line 510):

```go
// mockGitHubLinkedPRs extends mockGitHub with linked PR support for issue filtering tests.
type mockGitHubLinkedPRs struct {
	mockGitHub
	linkedPRs map[int][]domain.PullRequest // issue number -> linked PRs
}

func (m *mockGitHubLinkedPRs) FetchLinkedPullRequests(ctx context.Context, repo string, issueNumber int) ([]domain.PullRequest, error) {
	if prs, ok := m.linkedPRs[issueNumber]; ok {
		return prs, nil
	}
	return nil, nil
}
```

**Step 2: Run tests to verify nothing breaks**

Run: `cd /home/debian/gopilot && go test -race ./internal/orchestrator/...`
Expected: All existing tests PASS

**Step 3: Commit**

```bash
git add internal/orchestrator/orchestrator_test.go
git commit -m "test: add mockGitHubLinkedPRs for PR-aware filtering tests"
```

---

### Task 2: Write failing test for candidate filtering

**Files:**
- Modify: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the test**

Add this test after the existing `TestBlockedIssueNotDispatched` test:

```go
func TestIssueWithOpenPRNotDispatched(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 5},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work on {{ .Issue.Title }}",
	}

	gh := &mockGitHubLinkedPRs{
		mockGitHub: mockGitHub{
			issues: []domain.Issue{
				{ID: 1, Repo: "o/r", Title: "Has PR", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
				{ID: 2, Repo: "o/r", Title: "No PR", Labels: []string{"gopilot"}, Status: "Todo", Priority: 2, CreatedAt: time.Now()},
			},
		},
		linkedPRs: map[int][]domain.PullRequest{
			1: {{Number: 10, State: "open", Repo: "o/r"}},
		},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	orch.Tick(ctx)

	// Only issue 2 should be dispatched; issue 1 has an open PR
	if ag.started != 1 {
		t.Errorf("started = %d, want 1 (issue with open PR should be skipped)", ag.started)
	}
	if orch.state.RunningCount() != 1 {
		t.Errorf("running = %d, want 1", orch.state.RunningCount())
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `cd /home/debian/gopilot && go test -race -run TestIssueWithOpenPRNotDispatched ./internal/orchestrator/...`
Expected: FAIL — both issues get dispatched (started=2, want 1)

**Step 3: Commit**

```bash
git add internal/orchestrator/orchestrator_test.go
git commit -m "test: add failing test for issue with open PR not dispatched"
```

---

### Task 3: Implement candidate filtering in Tick

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:237`

**Step 1: Add PR enrichment loop**

In the `Tick` method, after the `BlockedBy` parsing loop (line 237) and before the `resolved` map construction (line 239), add:

```go
	// Enrich candidates with linked PR data
	for i := range issues {
		prs, err := o.github.FetchLinkedPullRequests(ctx, issues[i].Repo, issues[i].ID)
		if err != nil {
			slog.Warn("failed to fetch linked PRs", "issue", issues[i].Identifier(), "error", err)
			continue
		}
		issues[i].LinkedPRs = prs
	}
```

**Step 2: Add HasOpenPR check in the filtering loop**

In the filtering loop (around line 252, after the `IsBlocked` check), add:

```go
		if issue.HasOpenPR() {
			slog.Info("skipping issue with open PR", "issue", issue.Identifier())
			continue
		}
```

The filtering loop should now look like:

```go
	var candidates []domain.Issue
	for _, issue := range issues {
		if o.state.IsCompleted(issue.ID) || o.state.IsClaimed(issue.ID) || o.state.GetRunning(issue.ID) != nil || o.state.IsInRetryQueue(issue.ID) || o.retryQueue.Has(issue.ID) {
			continue
		}
		if issue.IsBlocked(resolved) {
			slog.Debug("skipping blocked issue", "issue", issue.Identifier(), "blocked_by", issue.BlockedBy)
			continue
		}
		if issue.HasOpenPR() {
			slog.Info("skipping issue with open PR", "issue", issue.Identifier())
			continue
		}
		candidates = append(candidates, issue)
	}
```

**Step 3: Run the test to verify it passes**

Run: `cd /home/debian/gopilot && go test -race -run TestIssueWithOpenPRNotDispatched ./internal/orchestrator/...`
Expected: PASS

**Step 4: Run all orchestrator tests**

Run: `cd /home/debian/gopilot && go test -race ./internal/orchestrator/...`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/orchestrator.go
git commit -m "feat: skip candidate issues that have open pull requests"
```

---

### Task 4: Write failing test for reconciliation PR check

**Files:**
- Modify: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the `mockGitHubSplitLinkedPRs` struct**

We need a mock that combines `mockGitHubSplit` (separate candidate vs state control) with linked PR support. Add after the `mockGitHubLinkedPRs` struct:

```go
// mockGitHubSplitLinkedPRs extends mockGitHubSplit with linked PR support for reconciliation tests.
type mockGitHubSplitLinkedPRs struct {
	mockGitHubSplit
	linkedPRs map[int][]domain.PullRequest
}

func (m *mockGitHubSplitLinkedPRs) FetchLinkedPullRequests(ctx context.Context, repo string, issueNumber int) ([]domain.PullRequest, error) {
	if prs, ok := m.linkedPRs[issueNumber]; ok {
		return prs, nil
	}
	return nil, nil
}
```

**Step 2: Write the reconciliation test**

```go
func TestReconcileStopsAgentWhenPROpened(t *testing.T) {
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

	issue := domain.Issue{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1}
	gh := &mockGitHubSplitLinkedPRs{
		mockGitHubSplit: mockGitHubSplit{
			candidates: []domain.Issue{issue},
			stateMap:   map[int]*domain.Issue{1: &issue},
		},
		linkedPRs: map[int][]domain.PullRequest{},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	ctx := context.Background()
	orch.Tick(ctx) // dispatch issue 1

	if orch.state.RunningCount() != 1 {
		t.Fatalf("running = %d, want 1 after dispatch", orch.state.RunningCount())
	}

	// Simulate PR being opened externally
	gh.linkedPRs[1] = []domain.PullRequest{{Number: 42, State: "open", Repo: "o/r"}}

	orch.reconcile(ctx)

	if orch.state.RunningCount() != 0 {
		t.Errorf("running = %d, want 0 after reconcile with open PR", orch.state.RunningCount())
	}
	if !orch.state.IsCompleted(1) {
		t.Error("issue 1 should be marked completed after PR detected")
	}
}
```

**Step 3: Run the test to verify it fails**

Run: `cd /home/debian/gopilot && go test -race -run TestReconcileStopsAgentWhenPROpened ./internal/orchestrator/...`
Expected: FAIL — reconcile doesn't check PRs yet

**Step 4: Commit**

```bash
git add internal/orchestrator/orchestrator_test.go
git commit -m "test: add failing test for reconcile stopping agent when PR opened"
```

---

### Task 5: Implement reconciliation PR check

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:548`

**Step 1: Add PR check in reconcile**

In the `reconcile` method, after the eligibility check (line 548) and before `entry.Issue = *issue` (line 550), add the PR check:

```go
		// Check for linked PRs — if an open PR exists, stop the agent and mark completed
		prs, err := o.github.FetchLinkedPullRequests(ctx, entry.Issue.Repo, entry.Issue.ID)
		if err != nil {
			slog.Warn("reconcile: failed to fetch linked PRs", "issue", entry.Issue.Identifier(), "error", err)
		} else {
			entry.Issue.LinkedPRs = prs
			if entry.Issue.HasOpenPR() {
				slog.Info("reconcile: issue has open PR, stopping agent", "issue", entry.Issue.Identifier())
				o.stopAndCleanup(ctx, entry, true)
				o.state.MarkCompleted(entry.Issue.ID)
				continue
			}
		}
```

The full `reconcile` method should now look like:

```go
func (o *Orchestrator) reconcile(ctx context.Context) {
	for _, entry := range o.state.AllRunning() {
		issue, err := o.github.FetchIssueState(ctx, entry.Issue.Repo, entry.Issue.ID)
		if err != nil {
			if errors.Is(err, gh.ErrNotFound) {
				slog.Info("reconcile: issue not found, stopping agent", "issue", entry.Issue.Identifier())
				o.stopAndCleanup(ctx, entry, true)
			} else {
				slog.Warn("reconcile: fetch failed", "issue", entry.Issue.Identifier(), "error", err)
			}
			continue
		}
		if issue == nil {
			continue
		}

		if issue.IsTerminal() {
			slog.Info("reconcile: issue became terminal, stopping agent", "issue", entry.Issue.Identifier(), "status", issue.Status)
			o.stopAndCleanup(ctx, entry, true)
			continue
		}

		if !issue.IsEligible(o.cfg.GitHub.EligibleLabels, o.cfg.GitHub.ExcludedLabels) {
			slog.Info("reconcile: issue no longer eligible, stopping agent", "issue", entry.Issue.Identifier())
			o.stopAndCleanup(ctx, entry, false)
			continue
		}

		// Check for linked PRs — if an open PR exists, stop the agent and mark completed
		prs, err := o.github.FetchLinkedPullRequests(ctx, entry.Issue.Repo, entry.Issue.ID)
		if err != nil {
			slog.Warn("reconcile: failed to fetch linked PRs", "issue", entry.Issue.Identifier(), "error", err)
		} else {
			entry.Issue.LinkedPRs = prs
			if entry.Issue.HasOpenPR() {
				slog.Info("reconcile: issue has open PR, stopping agent", "issue", entry.Issue.Identifier())
				o.stopAndCleanup(ctx, entry, true)
				o.state.MarkCompleted(entry.Issue.ID)
				continue
			}
		}

		entry.Issue = *issue
	}
}
```

**Step 2: Run the reconciliation test**

Run: `cd /home/debian/gopilot && go test -race -run TestReconcileStopsAgentWhenPROpened ./internal/orchestrator/...`
Expected: PASS

**Step 3: Run all orchestrator tests**

Run: `cd /home/debian/gopilot && go test -race ./internal/orchestrator/...`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add internal/orchestrator/orchestrator.go
git commit -m "feat: reconcile stops agents when open PR detected on issue"
```

---

### Task 6: Write test for closed PR (should NOT block dispatch)

**Files:**
- Modify: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the test**

This verifies that closed/merged PRs don't prevent dispatch (only open PRs do):

```go
func TestIssueWithClosedPRStillDispatched(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 5},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	gh := &mockGitHubLinkedPRs{
		mockGitHub: mockGitHub{
			issues: []domain.Issue{
				{ID: 1, Repo: "o/r", Title: "Has closed PR", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
			},
		},
		linkedPRs: map[int][]domain.PullRequest{
			1: {{Number: 10, State: "closed", Merged: true, Repo: "o/r"}},
		},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Issue with only closed/merged PRs should still be dispatched
	if ag.started != 1 {
		t.Errorf("started = %d, want 1 (closed PR should not block dispatch)", ag.started)
	}
}
```

**Step 2: Run the test**

Run: `cd /home/debian/gopilot && go test -race -run TestIssueWithClosedPRStillDispatched ./internal/orchestrator/...`
Expected: PASS (already works since `HasOpenPR` only checks `State == "open"`)

**Step 3: Commit**

```bash
git add internal/orchestrator/orchestrator_test.go
git commit -m "test: verify closed PRs don't block dispatch"
```

---

### Task 7: Run full test suite and lint

**Step 1: Run all tests**

Run: `cd /home/debian/gopilot && go test -race ./...`
Expected: All tests PASS

**Step 2: Run linter**

Run: `cd /home/debian/gopilot && golangci-lint run ./...`
Expected: No new warnings

**Step 3: Final commit (if any lint fixes needed)**

```bash
git add -A
git commit -m "chore: lint fixes for PR-aware filtering"
```
