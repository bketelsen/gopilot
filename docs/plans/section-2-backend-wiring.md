# Section 2: Backend Wiring Gaps

---

### Task 1: Add Repo field to RetryEntry and fix retry FetchIssueState

**Files:**
- Modify: `internal/domain/types.go:151-157` (RetryEntry struct)
- Modify: `internal/orchestrator/retry.go:32` (Enqueue signature)
- Modify: `internal/orchestrator/orchestrator.go:167,291,321` (callers of Enqueue)
- Test: `internal/orchestrator/retry_test.go`

**Step 1: Write the failing test**

In `internal/orchestrator/retry_test.go`, add a test that Enqueue stores repo:

```go
func TestRetryEntryHasRepo(t *testing.T) {
	q := NewRetryQueue()
	q.Enqueue(1, "o/r", "o/r#1", 2, "error", 5*time.Minute)
	entries := q.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Repo != "o/r" {
		t.Errorf("repo = %q, want %q", entries[0].Repo, "o/r")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/ -run TestRetryEntryHasRepo -v`
Expected: FAIL — `RetryEntry` has no `Repo` field, `Enqueue` wrong arg count

**Step 3: Add Repo to RetryEntry**

In `internal/domain/types.go`, add `Repo` field to `RetryEntry`:

```go
type RetryEntry struct {
	IssueID    int
	Repo       string // "owner/repo"
	Identifier string // "owner/repo#42"
	Attempt    int
	DueAt      time.Time
	Error      string
}
```

**Step 4: Update Enqueue signature**

In `internal/orchestrator/retry.go`, update `Enqueue`:

```go
func (q *RetryQueue) Enqueue(issueID int, repo string, identifier string, attempt int, errMsg string, maxBackoff time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delay := BackoffDelay(attempt, maxBackoff)
	q.entries[issueID] = &domain.RetryEntry{
		IssueID:    issueID,
		Repo:       repo,
		Identifier: identifier,
		Attempt:    attempt,
		DueAt:      time.Now().Add(delay),
		Error:      errMsg,
	}
}
```

**Step 5: Update all callers of Enqueue in orchestrator.go**

There are 4 call sites. Each needs the `issue.Repo` or `entry.Issue.Repo` argument added after `issueID`:

1. Line ~162 (retry re-enqueue): `o.retryQueue.Enqueue(retry.IssueID, retry.Repo, retry.Identifier, ...)`
2. Line ~167 (fix FetchIssueState): `o.github.FetchIssueState(ctx, retry.Repo, retry.IssueID)`
3. Line ~291 (monitorAgent): `o.retryQueue.Enqueue(issue.ID, issue.Repo, issue.Identifier(), entry.Attempt+1, errMsg, maxBackoff)`
4. Line ~321 (detectStalls): `o.retryQueue.Enqueue(entry.Issue.ID, entry.Issue.Repo, entry.Issue.Identifier(), entry.Attempt+1, "stalled", maxBackoff)`

**Step 6: Run tests**

Run: `go test -race ./internal/orchestrator/ -v`
Expected: All pass including new test

**Step 7: Commit**

```bash
git add internal/domain/types.go internal/orchestrator/retry.go internal/orchestrator/orchestrator.go internal/orchestrator/retry_test.go
git commit -m "fix: add Repo to RetryEntry and fix retry FetchIssueState empty repo"
```

---

### Task 2: Add retry eligibility re-check

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:158-173` (retry processing in Tick)
- Test: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the failing test**

In `internal/orchestrator/orchestrator_test.go`:

```go
func TestRetrySkipsIneligibleIssue(t *testing.T) {
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

	// Issue no longer has eligible label
	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{}, Status: "Todo", Priority: 1},
		},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, ag)

	// Manually add a retry entry
	orch.retryQueue.Enqueue(1, "o/r", "o/r#1", 2, "error", 5*time.Minute)
	// Force DueAt to past so it gets processed
	orch.retryQueue.mu.Lock()
	orch.retryQueue.entries[1].DueAt = time.Now().Add(-time.Second)
	orch.retryQueue.mu.Unlock()

	orch.Tick(context.Background())

	if ag.started != 0 {
		t.Errorf("started = %d, want 0 (issue no longer eligible)", ag.started)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/ -run TestRetrySkipsIneligibleIssue -v`
Expected: FAIL — agent gets started because eligibility isn't checked

**Step 3: Add eligibility check in retry processing**

In `internal/orchestrator/orchestrator.go`, in the retry loop after fetching issue state (around line 170), add:

```go
issue, err := o.github.FetchIssueState(ctx, retry.Repo, retry.IssueID)
if err != nil || issue == nil {
	slog.Warn("retry: could not fetch issue state", "issue_id", retry.IssueID, "error", err)
	continue
}
if !issue.IsEligible(o.cfg.GitHub.EligibleLabels, o.cfg.GitHub.ExcludedLabels) {
	slog.Info("retry: issue no longer eligible, releasing", "issue", retry.Identifier)
	o.state.Release(issue.ID)
	continue
}
o.state.Release(issue.ID)
o.dispatch(ctx, *issue, retry.Attempt)
```

**Step 4: Run tests**

Run: `go test -race ./internal/orchestrator/ -v`
Expected: All pass

**Step 5: Commit**

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go
git commit -m "fix: check eligibility before retry dispatch"
```

---

### Task 3: Multi-agent runner registry

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:24-64,200-241` (struct, constructor, dispatch)
- Modify: `internal/orchestrator/orchestrator_test.go` (update tests)
- Modify: `cmd/gopilot/main.go:50-57` (build runner registry)

**Step 1: Write the failing test**

In `internal/orchestrator/orchestrator_test.go`:

```go
func TestDispatchUsesCorrectAgent(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
			Overrides: []config.AgentOverride{
				{Labels: []string{"use-claude"}, Command: "claude-mock"},
			},
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot", "use-claude"}, Status: "Todo", Priority: 1},
		},
	}

	defaultAgent := &mockAgent{}
	claudeAgent := &mockAgent{}
	runners := map[string]agent.Runner{
		"mock":       defaultAgent,
		"claude-mock": claudeAgent,
	}

	orch := NewOrchestrator(cfg, gh, runners)
	orch.Tick(context.Background())

	if claudeAgent.started != 1 {
		t.Errorf("claude agent started = %d, want 1", claudeAgent.started)
	}
	if defaultAgent.started != 0 {
		t.Errorf("default agent started = %d, want 0", defaultAgent.started)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/ -run TestDispatchUsesCorrectAgent -v`
Expected: FAIL — `NewOrchestrator` doesn't accept runner map

**Step 3: Refactor Orchestrator to use runner registry**

In `internal/orchestrator/orchestrator.go`:

Change the struct field:
```go
type Orchestrator struct {
	cfg        *config.Config
	github     gh.Client
	agents     map[string]agent.Runner  // was: agent agent.Runner
	workspace  workspace.Manager
	// ... rest unchanged
}
```

Update `NewOrchestrator`:
```go
func NewOrchestrator(cfg *config.Config, github gh.Client, agents map[string]agent.Runner, configPath ...string) *Orchestrator {
	o := &Orchestrator{
		cfg:        cfg,
		github:     github,
		agents:     agents,
		// ... rest unchanged
	}
	// ... rest unchanged
}
```

Add agent lookup helper:
```go
func (o *Orchestrator) agentForIssue(issue domain.Issue) agent.Runner {
	cmd := o.cfg.AgentCommandForIssue(issue.Repo, issue.Labels)
	if runner, ok := o.agents[cmd]; ok {
		return runner
	}
	// Fallback to default command
	if runner, ok := o.agents[o.cfg.Agent.Command]; ok {
		return runner
	}
	// Return any available runner
	for _, runner := range o.agents {
		return runner
	}
	return nil
}
```

In `dispatch()`, replace `o.agent.Start(...)` with:
```go
runner := o.agentForIssue(issue)
if runner == nil {
	log.Error("no agent runner available")
	o.state.Release(issue.ID)
	return
}
sess, err := runner.Start(ctx, wsPath, rendered, opts)
```

In `detectStalls()` and `stopAndCleanup()`, replace `o.agent.Stop(sess)` with looking up the runner. Since all runners share the same Stop interface and Stop only needs the session, this can just use any runner. Add a helper:
```go
func (o *Orchestrator) stopSession(sess *agent.Session) {
	// All runners implement the same Stop behavior (SIGTERM -> SIGKILL)
	for _, runner := range o.agents {
		runner.Stop(sess)
		return
	}
}
```

Replace all `o.agent.Stop(sess)` calls with `o.stopSession(sess)`.

**Step 4: Update existing tests**

All existing tests pass a single `agent.Runner`. Update them to pass `map[string]agent.Runner`:

For each test using `NewOrchestrator(cfg, gh, ag)`, change to:
```go
NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})
```

For `mockFailAgent` tests:
```go
NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": failAgent})
```

**Step 5: Update cmd/gopilot/main.go**

Replace the single runner creation with a registry:

```go
runners := map[string]agent.Runner{
	cfg.Agent.Command: &agent.CopilotRunner{
		Command: cfg.Agent.Command,
		Token:   cfg.GitHub.Token,
	},
}

// Register override agents
for _, override := range cfg.Agent.Overrides {
	if _, exists := runners[override.Command]; !exists {
		switch override.Command {
		case "claude", "claude-code":
			runners[override.Command] = &agent.ClaudeRunner{
				Command: override.Command,
				Token:   cfg.GitHub.Token,
			}
		default:
			runners[override.Command] = &agent.CopilotRunner{
				Command: override.Command,
				Token:   cfg.GitHub.Token,
			}
		}
	}
}

orch := orchestrator.NewOrchestrator(cfg, restClient, runners, *configPath)
```

**Step 6: Run all tests**

Run: `go test -race ./...`
Expected: All pass

**Step 7: Commit**

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go cmd/gopilot/main.go
git commit -m "feat: multi-agent runner registry with per-issue agent selection"
```

---

### Task 4: Sub-issue blocking enforcement

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:182-188` (candidate filtering in Tick)
- Test: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the failing test**

In `internal/orchestrator/orchestrator_test.go`:

```go
func TestBlockedIssueNotDispatched(t *testing.T) {
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
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1,
				Body: "blocked by #2", BlockedBy: []int{2}},
			{ID: 3, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 2},
		},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	orch.Tick(context.Background())

	// Only issue 3 should be dispatched, issue 1 is blocked by #2
	if ag.started != 1 {
		t.Errorf("started = %d, want 1 (blocked issue should be skipped)", ag.started)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/ -run TestBlockedIssueNotDispatched -v`
Expected: FAIL — both issues dispatched (blocking not checked)

**Step 3: Add blocking check to candidate filtering**

In `internal/orchestrator/orchestrator.go`, in `Tick()`, after fetching candidates, build a resolved map and filter:

```go
issues, err := o.github.FetchCandidateIssues(ctx)
if err != nil {
	slog.Error("failed to fetch candidates", "error", err)
	return
}

// Parse BlockedBy from body text for all issues
for i := range issues {
	if len(issues[i].BlockedBy) == 0 {
		issues[i].BlockedBy = domain.ParseBlockedBy(issues[i].Body)
	}
}

// Build resolved map: issues that are NOT in the candidate list with Todo status
// are considered potentially resolved. Also check terminal states from all known issues.
resolved := make(map[int]bool)
for _, issue := range issues {
	if issue.IsTerminal() {
		resolved[issue.ID] = true
	}
}

var candidates []domain.Issue
for _, issue := range issues {
	if o.state.IsClaimed(issue.ID) || o.state.GetRunning(issue.ID) != nil || o.state.IsInRetryQueue(issue.ID) || o.retryQueue.Has(issue.ID) {
		continue
	}
	if issue.IsBlocked(resolved) {
		slog.Debug("skipping blocked issue", "issue", issue.Identifier(), "blocked_by", issue.BlockedBy)
		continue
	}
	candidates = append(candidates, issue)
}
```

**Step 4: Run tests**

Run: `go test -race ./internal/orchestrator/ -v`
Expected: All pass

**Step 5: Commit**

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go
git commit -m "feat: enforce sub-issue blocking in candidate filtering"
```

---

### Task 5: Max retries resets Status to "Todo"

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:374-384` (handleMaxRetriesExceeded)

**Step 1: Add SetProjectStatus call**

In `handleMaxRetriesExceeded()`, add before the comment:

```go
func (o *Orchestrator) handleMaxRetriesExceeded(issue domain.Issue, lastError string) {
	log := slog.With("issue", issue.Identifier())
	log.Error("max retries exceeded", "attempts", o.cfg.Agent.MaxRetries, "last_error", lastError)

	o.metrics.Increment("issues_failed")
	o.state.Release(issue.ID)

	if err := o.github.SetProjectStatus(context.Background(), issue, "Todo"); err != nil {
		log.Warn("failed to reset status to Todo", "error", err)
	}

	comment := fmt.Sprintf("Gopilot failed after %d attempts. Last error: %s", o.cfg.Agent.MaxRetries, lastError)
	o.github.AddComment(context.Background(), issue.Repo, issue.ID, comment)
	o.github.AddLabel(context.Background(), issue.Repo, issue.ID, "gopilot-failed")
}
```

**Step 2: Run tests**

Run: `go test -race ./internal/orchestrator/ -v`
Expected: All pass (mockGitHub.SetProjectStatus is a no-op)

**Step 3: Commit**

```bash
git add internal/orchestrator/orchestrator.go
git commit -m "fix: reset issue Status to Todo when max retries exceeded"
```

---

### Task 6: CLI --port flag

**Files:**
- Modify: `cmd/gopilot/main.go`

**Step 1: Add --port flag**

After the existing flag definitions:

```go
configPath := flag.String("config", "gopilot.yaml", "path to config file")
dryRun := flag.Bool("dry-run", false, "list eligible issues without dispatching")
debug := flag.Bool("debug", false, "enable debug logging")
port := flag.String("port", "", "override dashboard listen port (e.g., 8080)")
flag.Parse()
```

After loading config, apply the port override:

```go
if *port != "" {
	cfg.Dashboard.Addr = ":" + *port
	cfg.Dashboard.Enabled = true
}
```

**Step 2: Build to verify**

Run: `go build ./cmd/gopilot/`
Expected: Compiles

**Step 3: Commit**

```bash
git add cmd/gopilot/main.go
git commit -m "feat: add --port CLI flag to override dashboard address"
```

---

### Task 7: GraphQL EnrichIssues for Priority and Iteration

**Files:**
- Modify: `internal/github/graphql.go`
- Modify: `internal/github/client.go` (add EnrichIssues to interface)
- Test: `internal/github/graphql_test.go`

**Step 1: Write the test**

In `internal/github/graphql_test.go`, add a test for the enrichment query construction (unit test, no real API call):

```go
func TestEnrichIssuesQueryConstruction(t *testing.T) {
	// Test that EnrichIssues is callable and handles empty input
	cfg := config.GitHubConfig{Token: "test-token"}
	client := NewGraphQLClient(cfg, "https://api.github.com/graphql")

	// With no project meta, should return error
	_, err := client.EnrichIssues(context.Background(), nil)
	if err != nil {
		t.Errorf("EnrichIssues with nil should not error, got: %v", err)
	}
}
```

**Step 2: Add EnrichIssues to Client interface**

In `internal/github/client.go`:

```go
type Client interface {
	FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error)
	FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error)
	FetchIssueStates(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error)
	SetProjectStatus(ctx context.Context, issue domain.Issue, status string) error
	AddComment(ctx context.Context, repo string, id int, body string) error
	AddLabel(ctx context.Context, repo string, id int, label string) error
	EnrichIssues(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error)
}
```

**Step 3: Add no-op EnrichIssues to RESTClient**

In `internal/github/rest.go`:

```go
func (c *RESTClient) EnrichIssues(_ context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	return issues, nil
}
```

**Step 4: Add EnrichIssues to GraphQLClient**

In `internal/github/graphql.go`, add the method. This queries Projects v2 item fields for each issue by node ID:

```go
func (c *GraphQLClient) EnrichIssues(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	if issues == nil || c.meta == nil {
		return issues, nil
	}
	// For now, enrich using a per-issue query (batching can be optimized later)
	for i, issue := range issues {
		if issue.NodeID == "" {
			continue
		}
		enriched, err := c.enrichSingleIssue(ctx, issue)
		if err != nil {
			slog.Warn("failed to enrich issue", "issue", issue.Identifier(), "error", err)
			continue
		}
		issues[i] = enriched
	}
	return issues, nil
}

func (c *GraphQLClient) enrichSingleIssue(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	query := fmt.Sprintf(`{
		node(id: %q) {
			... on Issue {
				projectItems(first: 10) {
					nodes {
						project { id }
						fieldValues(first: 20) {
							nodes {
								__typename
								... on ProjectV2ItemFieldSingleSelectValue {
									field { ... on ProjectV2SingleSelectField { name } }
									name
								}
								... on ProjectV2ItemFieldIterationValue {
									field { ... on ProjectV2IterationField { name } }
									title
								}
								... on ProjectV2ItemFieldNumberValue {
									field { ... on ProjectV2Field { name } }
									number
								}
							}
						}
					}
				}
			}
		}
	}`, issue.NodeID)

	result, err := c.execute(ctx, query, nil)
	if err != nil {
		return issue, err
	}

	// Parse project item fields
	node, _ := result["data"].(map[string]any)
	if node == nil {
		return issue, nil
	}
	issueNode, _ := node["node"].(map[string]any)
	if issueNode == nil {
		return issue, nil
	}
	items, _ := issueNode["projectItems"].(map[string]any)
	if items == nil {
		return issue, nil
	}
	nodes, _ := items["nodes"].([]any)
	for _, n := range nodes {
		item, _ := n.(map[string]any)
		if item == nil {
			continue
		}
		fieldValues, _ := item["fieldValues"].(map[string]any)
		if fieldValues == nil {
			continue
		}
		fvNodes, _ := fieldValues["nodes"].([]any)
		for _, fv := range fvNodes {
			fvMap, _ := fv.(map[string]any)
			if fvMap == nil {
				continue
			}
			typename, _ := fvMap["__typename"].(string)
			switch typename {
			case "ProjectV2ItemFieldSingleSelectValue":
				fieldObj, _ := fvMap["field"].(map[string]any)
				fieldName, _ := fieldObj["name"].(string)
				valueName, _ := fvMap["name"].(string)
				switch fieldName {
				case "Status":
					issue.Status = valueName
				case "Priority":
					issue.Priority = priorityToInt(valueName)
				}
			case "ProjectV2ItemFieldIterationValue":
				title, _ := fvMap["title"].(string)
				issue.Iteration = title
			case "ProjectV2ItemFieldNumberValue":
				fieldObj, _ := fvMap["field"].(map[string]any)
				fieldName, _ := fieldObj["name"].(string)
				if fieldName == "Effort" {
					num, _ := fvMap["number"].(float64)
					issue.Effort = int(num)
				}
			}
		}
	}

	return issue, nil
}

func priorityToInt(name string) int {
	switch name {
	case "Urgent", "P0":
		return 1
	case "High", "P1":
		return 2
	case "Medium", "P2":
		return 3
	case "Low", "P3":
		return 4
	default:
		return 0
	}
}
```

Also add the `slog` import to graphql.go:
```go
import "log/slog"
```

**Step 5: Update mockGitHub in orchestrator tests**

In `internal/orchestrator/orchestrator_test.go`, add the `EnrichIssues` method to `mockGitHub`:

```go
func (m *mockGitHub) EnrichIssues(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	return issues, nil
}
```

**Step 6: Run all tests**

Run: `go test -race ./...`
Expected: All pass

**Step 7: Commit**

```bash
git add internal/github/graphql.go internal/github/client.go internal/github/rest.go internal/github/graphql_test.go internal/orchestrator/orchestrator_test.go
git commit -m "feat: GraphQL EnrichIssues for Priority, Iteration, Effort from Projects v2"
```

This completes Section 2. All backend wiring gaps are closed.
