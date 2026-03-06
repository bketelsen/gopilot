# PR Fix Branch Awareness

**Date:** 2026-03-06
**Status:** Approved

## Problem

When the PR monitoring system detects a PR with failed CI checks and dispatches a fix agent, the agent creates a new PR instead of fixing the existing one.

### Root Cause

`dispatchPRFix` creates a synthetic issue with ID = PR number + 1,000,000 (e.g., 1000027 for PR #27). The standard `before_run` hook runs:

```bash
git checkout -B gopilot/issue-{{issue_id}} origin/main
```

This expands to `gopilot/issue-1000027` — a brand new branch off main. The agent starts on the wrong branch and has no choice but to create a new PR.

Additionally, the `{{branch}}` variable in `expandHookVars` is hardcoded to `gopilot/issue-{id}`, which also resolves to the synthetic ID.

### Observed Incident (from logs)

1. Agent dispatched for issue #27, creates PR #27
2. PR monitor detects PR #27 has failed checks
3. PR fix agent dispatched into workspace `issue-1000027`
4. Fix agent "completes successfully" but creates PR #29 (new branch, new PR)
5. Polling loop picks up #29 as a new eligible issue, dispatches another agent

## Solution

### 1. Add `Branch` field to `domain.Issue`

```go
type Issue struct {
    // ...existing fields...
    Branch string // Override for workspace hook {{branch}} variable
}
```

When set, `expandHookVars` uses this value for `{{branch}}` instead of generating `gopilot/issue-{id}`.

### 2. Add `before_pr_fix` hook to config

New hook in `HooksConfig`:

```go
BeforePRFix string `yaml:"before_pr_fix"`
```

Default value:

```yaml
before_pr_fix: |
  git fetch origin
  git checkout {{branch}}
  git pull origin {{branch}}
```

This checks out the PR's actual branch and pulls latest changes.

### 3. Update `dispatchPRFix`

- Set `Branch` on the synthetic issue to `fix.PR.HeadRef`
- Call `RunHook("before_pr_fix", ...)` instead of `RunHook("before_run", ...)`
- Fall back to `before_run` if `before_pr_fix` is empty

## Components Affected

| File | Change |
|------|--------|
| `internal/domain/types.go` | Add `Branch` field to `Issue` |
| `internal/workspace/fs_manager.go` | Support `before_pr_fix` hook; use `issue.Branch` override in `expandHookVars` |
| `internal/config/config.go` | Add `BeforePRFix` to `HooksConfig` |
| `internal/config/example.go` | Add default `before_pr_fix` hook |
| `internal/orchestrator/orchestrator.go` | Set `Branch` on synthetic issue; call `before_pr_fix` hook |
| Tests | Hook expansion with Branch override; PR fix dispatch uses correct hook |
