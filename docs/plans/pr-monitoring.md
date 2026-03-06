# Plan: PR Monitoring and Auto-Fix

## Problem Statement

Gopilot dispatches agents to work on GitHub issues. Agents create PRs, but gopilot
has no mechanism to monitor those PRs for CI failures or dispatch agents to fix them.
This leads to broken PRs sitting unattended (e.g., PR #22 which added golangci-lint
config but didn't install it in CI).

## Two-Part Fix

### Part A: Fix PR #22 (immediate)

Add `golangci-lint` installation step to `.github/workflows/ci.yml` before the lint step.

**File:** `.github/workflows/ci.yml`

Add after the "Install Task" step:
```yaml
- name: Install golangci-lint
  uses: golangci/golangci-lint-action@v7
  with:
    args: run ./...
```

Note: The `golangci-lint-action` both installs and runs the linter, so the separate
`task lint` step can be replaced or kept as a fallback. Alternatively, install the
binary separately and keep `task lint`.

### Part B: PR Monitoring System (feature)

#### Overview

Extend the orchestrator's poll-dispatch-reconcile loop to also monitor PRs created
by agents and re-dispatch agents when CI checks fail.

#### Step 1: Domain Types

**File:** `internal/domain/types.go`

Add new types:

```go
// PullRequest represents a GitHub PR being monitored by gopilot.
type PullRequest struct {
    Number    int
    Repo      string
    HeadRef   string
    IssueID   int       // originating issue, 0 if standalone
    URL       string
    State     string    // open, closed, merged
    CheckRuns []CheckRun
}

// CheckRun represents a CI check result on a PR.
type CheckRun struct {
    Name       string
    Status     string // queued, in_progress, completed
    Conclusion string // success, failure, neutral, cancelled, skipped, timed_out
    DetailsURL string
    Output     string // truncated failure output for agent context
}

// PRFixEntry tracks an agent dispatched to fix a PR.
type PRFixEntry struct {
    PR          PullRequest
    Attempt     int
    NextRetryAt time.Time
}
```

#### Step 2: GitHub Client Extensions

**File:** `internal/github/client.go`

Add to the `Client` interface:

```go
// FetchMonitoredPRs returns open PRs created by the configured bot user
// in the configured repos.
FetchMonitoredPRs(ctx context.Context) ([]domain.PullRequest, error)

// FetchCheckRuns returns check run results for a PR's head commit.
FetchCheckRuns(ctx context.Context, repo string, ref string) ([]domain.CheckRun, error)

// FetchCheckRunLog returns the truncated failure log for a specific check run.
FetchCheckRunLog(ctx context.Context, repo string, checkRunID int64) (string, error)
```

**File:** `internal/github/rest.go`

Implement using:
- `GET /repos/{owner}/{repo}/pulls?state=open&creator={bot_user}` (or search by label)
- `GET /repos/{owner}/{repo}/commits/{ref}/check-runs`
- `GET /repos/{owner}/{repo}/check-runs/{check_run_id}` for failure details

**PR Discovery strategy:** Rather than monitoring all open PRs, track PRs by:
1. Label-based: Agent adds a `gopilot` label when creating PRs
2. OR: Search for PRs whose body contains `Closes #N` where N is a tracked issue
3. OR: Store PR numbers in state when agent output is parsed

Option 1 (label-based) is simplest and most reliable. The agent prompt template
should include instructions to add a `gopilot` label to created PRs.

#### Step 3: Orchestrator PR Monitoring

**File:** `internal/orchestrator/orchestrator.go`

Add a new phase to the `Tick()` loop, after issue reconciliation:

```go
func (o *Orchestrator) Tick(ctx context.Context) {
    o.reconcile(ctx)
    o.detectStalls(ctx)
    o.monitorPRs(ctx)      // NEW: check PR CI status
    o.retryPRFixes(ctx)    // NEW: dispatch agents to fix failed PRs

    // ... existing issue polling and dispatch ...
}
```

**`monitorPRs(ctx)`** logic:
1. Call `FetchMonitoredPRs()` to get open PRs with gopilot label
2. For each PR, call `FetchCheckRuns()` to get CI status
3. If all checks pass: mark PR as healthy, optionally comment on originating issue
4. If any check failed AND PR is not already being fixed:
   - Fetch failure logs (truncated)
   - Add to PR fix queue with failure context
5. If PR was merged: mark originating issue as done (if not already)

**`retryPRFixes(ctx)`** logic:
1. Iterate PR fix queue entries where `NextRetryAt <= now`
2. Check concurrency slots (shared with issue agents)
3. Dispatch agent with PR-fix prompt (see Step 5)
4. Track in state as a running fix entry

#### Step 4: State Management

**File:** `internal/orchestrator/state.go`

Add to `State`:

```go
type State struct {
    // ... existing fields ...

    prFixes    map[int]*domain.PRFixEntry  // PR number -> fix entry
    prRunning  map[int]*domain.RunEntry    // PR number -> active fix session
    prHistory  map[int][]domain.CompletedRun // PR number -> completed fix runs
}
```

Methods to add:
- `AddPRFix(pr domain.PullRequest, checkRuns []domain.CheckRun)`
- `GetPendingPRFixes() []domain.PRFixEntry`
- `StartPRFix(prNumber int, entry *domain.RunEntry)`
- `CompletePRFix(prNumber int, run domain.CompletedRun)`
- `IsPRBeingFixed(prNumber int) bool`

#### Step 5: PR-Fix Prompt Template

**File:** `internal/prompt/templates/pr-fix.tmpl` (new)

```
You are fixing a pull request that has failing CI checks.

## Pull Request
- PR: {{.PR.URL}}
- Branch: {{.PR.HeadRef}}
- Original Issue: {{if .PR.IssueID}}#{{.PR.IssueID}}{{else}}N/A{{end}}

## Failed Checks
{{range .FailedChecks}}
### {{.Name}}
Status: {{.Conclusion}}
{{if .Output}}
Failure output (truncated):
```
{{.Output}}
```
{{end}}
{{end}}

## Instructions
1. Check out the existing branch `{{.PR.HeadRef}}`
2. Read the CI failure output carefully
3. Fix the root cause of the failure
4. Ensure all checks will pass
5. Commit and push to the same branch

Do NOT create a new PR. Push fixes to the existing branch.
```

#### Step 6: Workspace Hooks for PR Fixes

PR fix workspaces need different hooks than issue workspaces:
- Clone the repo
- Check out the existing PR branch (not create a new one)
- Push to the same branch

**File:** `internal/config/config.go`

Add to config:

```go
type PRMonitoringConfig struct {
    Enabled        bool          `yaml:"enabled"`
    PollInterval   time.Duration `yaml:"poll_interval"`   // default: 5m
    Label          string        `yaml:"label"`           // default: "gopilot"
    MaxFixAttempts int           `yaml:"max_fix_attempts"` // default: 2
    LogTruncateLen int           `yaml:"log_truncate_len"` // default: 2000 chars
}
```

#### Step 7: Agent Prompt Update

Update the default issue-working prompt to instruct agents to:
1. Add the `gopilot` label to PRs they create
2. Include `Closes #N` in PR body (already common)

This enables the PR monitoring system to discover PRs created by agents.

#### Step 8: Dashboard Updates

**File:** `internal/web/`

Add a PR monitoring section to the dashboard showing:
- Monitored PRs and their check status
- Active PR fix sessions
- PR fix history

Implement `PRProvider` interface (similar to existing `StateProvider`):

```go
type PRProvider interface {
    MonitoredPRs() []domain.PullRequest
    ActivePRFixes() map[int]*domain.RunEntry
    PRFixHistory() map[int][]domain.CompletedRun
}
```

## Implementation Order

1. **Part A: Fix CI** (5 min) - Fix `.github/workflows/ci.yml` for PR #22
2. **Step 1: Domain types** - Add PR/CheckRun types
3. **Step 2: GitHub client** - Add PR and check-run fetching
4. **Step 6: Config** - Add PR monitoring config section
5. **Step 3: Orchestrator** - Add `monitorPRs()` and `retryPRFixes()` to tick loop
6. **Step 4: State** - Add PR tracking to state
7. **Step 5: Prompt template** - Create PR-fix prompt
8. **Step 7: Agent prompt** - Update issue prompt to add gopilot label to PRs
9. **Step 8: Dashboard** - Add PR monitoring UI

Steps 1-4 form the MVP. Steps 5-8 build on it.

## Design Decisions

**Why label-based PR discovery?**
Simpler than parsing agent output or searching PR bodies. The label is added by the
agent prompt, giving gopilot a reliable signal. Avoids false positives from unrelated PRs.

**Why share concurrency slots?**
PR fixes and issue work compete for the same agent resources. A single concurrency
pool prevents overloading. PR fixes could optionally have a dedicated slot reservation.

**Why truncate failure logs?**
Agent context windows are limited. Full CI logs can be huge. Truncating to ~2000 chars
of the relevant failure section keeps the prompt focused.

**Why max 2 fix attempts?**
If an agent can't fix CI in 2 attempts, the problem likely needs human intervention.
After max attempts, add a `needs-human` label and comment on the PR.

## Risks

- **Rate limits:** Additional API calls for PR monitoring. Mitigated by longer poll
  interval (5m vs 30s for issues) and only monitoring labeled PRs.
- **Infinite loops:** Agent fix creates new failures. Mitigated by max fix attempts
  and tracking attempt count.
- **Stale PRs:** Old PRs with `gopilot` label sitting open. Add a staleness check
  to skip PRs older than configurable threshold.
