# Design: Filter Issues with Open Pull Requests

**Date:** 2026-03-06
**Problem:** Agents attempt work on issues that already have open pull requests, wasting compute and potentially creating conflicting PRs (observed with issue #25).

## Solution

Add PR-awareness at two points in the orchestrator's Tick loop:

1. **Candidate filtering** — prevent dispatch of issues with open PRs
2. **Reconciliation** — stop running agents when a PR appears, mark issue completed

## Approach

Use the existing `FetchLinkedPullRequests` (timeline API, 1 call per issue) and `HasOpenPR()` method — both already exist but are unused in the orchestrator.

### Candidate Filtering (in `Tick`)

After fetching candidate issues and parsing BlockedBy, enrich each issue with linked PR data:

```go
for i := range issues {
    prs, err := o.github.FetchLinkedPullRequests(ctx, issues[i].Repo, issues[i].ID)
    if err != nil {
        slog.Warn("failed to fetch linked PRs", "issue", issues[i].Identifier(), "error", err)
        continue
    }
    issues[i].LinkedPRs = prs
}
```

Then in the filtering loop, skip issues with open PRs:

```go
if issue.HasOpenPR() {
    slog.Info("skipping issue with open PR", "issue", issue.Identifier())
    continue
}
```

### Reconciliation (in `reconcile`)

After the existing eligibility check, fetch linked PRs for running agents:

```go
prs, err := o.github.FetchLinkedPullRequests(ctx, entry.Issue.Repo, entry.Issue.ID)
if err == nil {
    entry.Issue.LinkedPRs = prs
    if entry.Issue.HasOpenPR() {
        slog.Info("reconcile: issue has open PR, stopping agent", "issue", entry.Issue.Identifier())
        o.stopAndCleanup(ctx, entry, true)
        o.state.MarkCompleted(entry.Issue.ID)
        continue
    }
}
```

## Files Changed

| File | Change |
|------|--------|
| `internal/orchestrator/orchestrator.go` | Add PR enrichment in `Tick` candidate loop; add PR check in `reconcile()` |
| `internal/orchestrator/orchestrator_test.go` | Update mocks to return linked PRs; add test cases for both filtering and reconciliation |

## What This Does NOT Change

- No new config options
- No changes to the GitHub client interface (methods already exist)
- No changes to domain types (`HasOpenPR` already exists)
- `HasMergedPR` is not checked — merged PRs on "Todo" issues are handled by existing status-based filtering

## API Cost

~1 additional API call per candidate issue per tick (timeline endpoint). Candidate lists are typically small (< 20 issues), so this adds negligible rate limit pressure.
