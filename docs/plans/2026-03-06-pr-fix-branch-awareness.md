# PR Fix Branch Awareness Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make PR fix agents check out and push to the existing PR branch instead of creating a new branch/PR.

**Architecture:** Add a `Branch` override field to `domain.Issue`. Add a `before_pr_fix` hook to config. Wire `dispatchPRFix` to set the PR's HeadRef as the branch and use the new hook.

**Tech Stack:** Go 1.25, stdlib testing

---

### Task 1: Add Branch field to domain.Issue

**Files:**
- Modify: `internal/domain/types.go:15-45`

**Step 1: Add the Branch field**

Add after the `LinkedPRs` field (line 40):

```go
// Branch overrides the default {{branch}} hook variable when set.
Branch string
```

**Step 2: Run tests to verify no breakage**

Run: `go test -race ./internal/domain/...`
Expected: PASS (new field has zero value, no existing code affected)

**Step 3: Commit**

```bash
git add internal/domain/types.go
git commit -m "feat: add Branch override field to domain.Issue"
```

---

### Task 2: Use Issue.Branch in expandHookVars

**Files:**
- Modify: `internal/workspace/fs_manager.go:100-108`
- Test: `internal/workspace/fs_manager_test.go`

**Step 1: Write the failing test**

Add to `fs_manager_test.go`:

```go
func TestExpandHookVarsBranchOverride(t *testing.T) {
	issue := domain.Issue{ID: 42, Repo: "owner/repo", Branch: "feature/my-pr-branch"}
	got := expandHookVars("checkout {{branch}}", issue, "/tmp/ws")
	want := "checkout feature/my-pr-branch"
	if got != want {
		t.Errorf("expandHookVars() = %q, want %q", got, want)
	}
}

func TestExpandHookVarsBranchDefault(t *testing.T) {
	issue := domain.Issue{ID: 42, Repo: "owner/repo"}
	got := expandHookVars("checkout {{branch}}", issue, "/tmp/ws")
	want := "checkout gopilot/issue-42"
	if got != want {
		t.Errorf("expandHookVars() = %q, want %q", got, want)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -run TestExpandHookVarsBranch ./internal/workspace/...`
Expected: FAIL — `TestExpandHookVarsBranchOverride` gets `"checkout gopilot/issue-42"` instead of `"checkout feature/my-pr-branch"`

**Step 3: Implement the Branch override**

In `fs_manager.go`, update `expandHookVars` (line 100-108):

```go
func expandHookVars(script string, issue domain.Issue, workspace string) string {
	branch := fmt.Sprintf("gopilot/issue-%d", issue.ID)
	if issue.Branch != "" {
		branch = issue.Branch
	}
	r := strings.NewReplacer(
		"{{repo}}", issue.Repo,
		"{{issue_id}}", fmt.Sprintf("%d", issue.ID),
		"{{branch}}", branch,
		"{{workspace}}", workspace,
	)
	return r.Replace(script)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -run TestExpandHookVarsBranch ./internal/workspace/...`
Expected: PASS

**Step 5: Run full workspace tests**

Run: `go test -race ./internal/workspace/...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/workspace/fs_manager.go internal/workspace/fs_manager_test.go
git commit -m "feat: support Branch override in workspace hook variable expansion"
```

---

### Task 3: Add before_pr_fix hook to config and workspace manager

**Files:**
- Modify: `internal/config/config.go:50-55`
- Modify: `internal/config/example.go`
- Modify: `internal/workspace/fs_manager.go:55-71`
- Test: `internal/workspace/fs_manager_test.go`

**Step 1: Write the failing test**

Add to `fs_manager_test.go`:

```go
func TestRunHookBeforePRFix(t *testing.T) {
	root := t.TempDir()
	cfg := config.WorkspaceConfig{
		Root:          root,
		HookTimeoutMS: 5000,
		Hooks: config.HooksConfig{
			BeforeRun:    `echo "before_run" > hook_output.txt`,
			BeforePRFix: `echo "branch={{branch}}" > hook_output.txt`,
		},
	}
	mgr := NewFSManager(cfg)
	issue := domain.Issue{ID: 99, Repo: "myorg/myrepo", Branch: "gopilot/issue-11"}

	path, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}

	err = mgr.RunHook(context.Background(), "before_pr_fix", path, issue)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(path, "hook_output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	want := "branch=gopilot/issue-11"
	if got != want {
		t.Errorf("hook output = %q, want %q", got, want)
	}
}

func TestRunHookBeforePRFixFallsBackToBeforeRun(t *testing.T) {
	root := t.TempDir()
	cfg := config.WorkspaceConfig{
		Root:          root,
		HookTimeoutMS: 5000,
		Hooks: config.HooksConfig{
			BeforeRun: `echo "before_run" > hook_output.txt`,
			// BeforePRFix is empty — should fall back to BeforeRun
		},
	}
	mgr := NewFSManager(cfg)
	issue := domain.Issue{ID: 99, Repo: "myorg/myrepo", Branch: "gopilot/issue-11"}

	path, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}

	err = mgr.RunHook(context.Background(), "before_pr_fix", path, issue)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(path, "hook_output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	want := "before_run"
	if got != want {
		t.Errorf("hook output = %q, want %q (should fall back to before_run)", got, want)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -run TestRunHookBeforePRFix ./internal/workspace/...`
Expected: FAIL — compilation error: `config.HooksConfig` has no field `BeforePRFix`, and `RunHook` returns error for unknown hook `"before_pr_fix"`

**Step 3: Add BeforePRFix to HooksConfig**

In `internal/config/config.go`, update `HooksConfig` (line 50-55):

```go
type HooksConfig struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	BeforePRFix string `yaml:"before_pr_fix"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
}
```

**Step 4: Add before_pr_fix case to RunHook**

In `internal/workspace/fs_manager.go`, update the `RunHook` switch (line 55-71):

```go
func (m *FSManager) RunHook(ctx context.Context, hook string, workspacePath string, issue domain.Issue) error {
	var script string
	switch hook {
	case "before_run":
		script = m.cfg.Hooks.BeforeRun
	case "before_pr_fix":
		script = m.cfg.Hooks.BeforePRFix
		if script == "" {
			script = m.cfg.Hooks.BeforeRun
		}
	case "after_run":
		script = m.cfg.Hooks.AfterRun
	case "before_remove":
		script = m.cfg.Hooks.BeforeRemove
	default:
		return fmt.Errorf("unknown hook: %s", hook)
	}
	if script == "" {
		return nil
	}
	return m.runHook(ctx, script, workspacePath, issue)
}
```

**Step 5: Update example config**

In `internal/config/example.go`, add after the `before_run` hook (after line 31):

```yaml
    before_pr_fix: |
      git fetch origin
      git checkout {{branch}}
      git pull origin {{branch}}
```

**Step 6: Run tests to verify they pass**

Run: `go test -race -run TestRunHookBeforePRFix ./internal/workspace/...`
Expected: PASS

**Step 7: Run all workspace tests**

Run: `go test -race ./internal/workspace/...`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/config/config.go internal/config/example.go internal/workspace/fs_manager.go internal/workspace/fs_manager_test.go
git commit -m "feat: add before_pr_fix hook with fallback to before_run"
```

---

### Task 4: Wire dispatchPRFix to use Branch and before_pr_fix hook

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:718-740`
- Test: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the failing test**

Add a mock workspace manager that records which hook was called and with what issue, then add a test. First, add to `orchestrator_test.go`:

```go
// mockWorkspace records hook calls for testing.
type mockWorkspace struct {
	ensuredIssues []domain.Issue
	hookCalls     []hookCall
	root          string
}

type hookCall struct {
	hook   string
	issue  domain.Issue
}

func (m *mockWorkspace) Ensure(ctx context.Context, issue domain.Issue) (string, error) {
	m.ensuredIssues = append(m.ensuredIssues, issue)
	path := filepath.Join(m.root, fmt.Sprintf("issue-%d", issue.ID))
	os.MkdirAll(path, 0755)
	return path, nil
}

func (m *mockWorkspace) RunHook(ctx context.Context, hook string, workspacePath string, issue domain.Issue) error {
	m.hookCalls = append(m.hookCalls, hookCall{hook: hook, issue: issue})
	return nil
}

func (m *mockWorkspace) Cleanup(ctx context.Context, issue domain.Issue) error { return nil }

func (m *mockWorkspace) Path(issue domain.Issue) string {
	return filepath.Join(m.root, fmt.Sprintf("issue-%d", issue.ID))
}
```

Then add the test:

```go
func TestDispatchPRFixUsesCorrectHookAndBranch(t *testing.T) {
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
		PRMonitoring: config.PRMonitoringConfig{
			Enabled:        true,
			PollIntervalMS: 1,
			Label:          "gopilot",
			MaxFixAttempts: 2,
			LogTruncateLen: 2000,
		},
	}

	gh := &mockGitHubPR{
		prs: []domain.PullRequest{
			{Number: 27, Repo: "o/r", HeadRef: "gopilot/issue-15", HeadSHA: "abc123", State: "open", Title: "feat: something"},
		},
		checkRuns: map[string][]domain.CheckRun{
			"abc123": {
				{ID: 1, Name: "test", Status: "completed", Conclusion: "failure"},
			},
		},
		checkLogs: map[int64]string{1: "test failed"},
	}
	ag := &mockAgent{}
	ws := &mockWorkspace{root: t.TempDir()}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})
	orch.workspace = ws

	ctx := context.Background()
	orch.monitorPRs(ctx)

	// Verify the before_pr_fix hook was called (not before_run)
	if len(ws.hookCalls) == 0 {
		t.Fatal("expected at least one hook call")
	}
	if ws.hookCalls[0].hook != "before_pr_fix" {
		t.Errorf("hook = %q, want %q", ws.hookCalls[0].hook, "before_pr_fix")
	}
	// Verify the Branch field was set to the PR's HeadRef
	if ws.hookCalls[0].issue.Branch != "gopilot/issue-15" {
		t.Errorf("Branch = %q, want %q", ws.hookCalls[0].issue.Branch, "gopilot/issue-15")
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `go test -race -run TestDispatchPRFixUsesCorrectHookAndBranch ./internal/orchestrator/...`
Expected: FAIL — the test should fail because `dispatchPRFix` currently calls `"before_run"` not `"before_pr_fix"`, and doesn't set `Branch`.

Note: You'll need to check if the orchestrator has a `workspace` field that can be replaced. Look at `orchestrator.go` for the field name. It's likely `workspace workspace.Manager`. If it's unexported, the test can set it directly since it's in the same package.

**Step 3: Update dispatchPRFix**

In `internal/orchestrator/orchestrator.go`, update `dispatchPRFix` (around line 722-735):

Change the synthetic issue creation to include `Branch`:

```go
syntheticIssue := domain.Issue{
	ID:     fix.PR.Number + 1000000,
	Repo:   fix.PR.Repo,
	Title:  fmt.Sprintf("PR Fix: %s", fix.PR.Title),
	URL:    fix.PR.URL,
	Branch: fix.PR.HeadRef,
}
```

Change the hook call from `"before_run"` to `"before_pr_fix"`:

```go
if err := o.workspace.RunHook(ctx, "before_pr_fix", wsPath, syntheticIssue); err != nil {
	log.Error("before_pr_fix hook failed for PR fix", "error", err)
	return
}
```

**Step 4: Run the test to verify it passes**

Run: `go test -race -run TestDispatchPRFixUsesCorrectHookAndBranch ./internal/orchestrator/...`
Expected: PASS

**Step 5: Run all orchestrator tests**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go
git commit -m "feat: PR fix agents use before_pr_fix hook and check out PR branch"
```

---

### Task 5: Lint, full test suite, and docs

**Files:**
- Modify: `docs/configuration.md` (if it documents hooks)

**Step 1: Run linter**

Run: `task lint`
Expected: 0 issues

**Step 2: Run full test suite**

Run: `task test`
Expected: PASS

**Step 3: Update docs if hooks are documented**

Check `docs/configuration.md` for hooks documentation. If present, add `before_pr_fix` to the hooks table with description: "Runs before a PR fix agent starts. Falls back to `before_run` if empty. Use `{{branch}}` for the PR's head branch."

**Step 4: Commit docs if changed**

```bash
git add docs/configuration.md
git commit -m "docs: add before_pr_fix hook to configuration reference"
```
