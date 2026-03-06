# Label Bootstrap Design

**Date:** 2026-03-06
**Status:** Approved

## Problem

Gopilot depends on several GitHub labels (eligible, planning, failure) but does not create them. Users must manually create labels on every configured repository before gopilot works correctly. If labels are missing, issues are silently ignored.

## Solution

Add a `gopilot setup` command that reads the existing config and ensures all required labels exist on every configured repository. The command is idempotent — safe to run multiple times.

## Command Flow

```
gopilot setup
  → Load gopilot.yaml
  → For each repo in github.repos:
      → For each required label:
          → GET /repos/{owner}/{repo}/labels/{name}
          → If 404: POST create it
          → If exists but color/description differ: PATCH update it
  → Print summary
```

Example output:
```
bketelsen/gopilot: created gopilot, gopilot:plan, gopilot:planned, gopilot-failed
bketelsen/other-repo: gopilot (ok), created gopilot:plan, gopilot:planned, gopilot-failed
```

## Canonical Labels

| Label | Color | Description | Source |
|-------|-------|-------------|--------|
| `gopilot` | `0052CC` (blue) | Eligible for Gopilot agent dispatch | `github.eligible_labels` default |
| `gopilot:plan` | `7B61FF` (purple) | Gopilot interactive planning | `planning.label` default |
| `gopilot:planned` | `1D7644` (green) | Planning completed by Gopilot | `planning.completed_label` default |
| `gopilot-failed` | `D93F0B` (red) | Gopilot agent failed after max retries | hard-coded in orchestrator |

If the user has customized `eligible_labels` or planning labels in their config, `setup` uses their configured names but applies the canonical colors/descriptions.

Excluded labels (`blocked`, `needs-design`, `wontfix`) are NOT auto-created — those are standard repo labels users manage themselves.

## Changes

### 1. New GitHub client methods

Add to `github.Client` interface and REST implementation:

- `GetLabel(ctx, repo, name) (*Label, error)` — GET, returns label or not-found
- `CreateLabel(ctx, repo, name, color, description) error` — POST
- `UpdateLabel(ctx, repo, name, color, description) error` — PATCH

### 2. New `internal/setup/` package

- `labels.go` — canonical label definitions (name, color, description) with a function that merges config-customized names with canonical colors
- `setup.go` — `Run(ctx, cfg, client)` iterates repos x labels, calls ensure logic, returns summary

### 3. Wire into CLI

Add `"setup"` case in `cmd/gopilot/main.go` switch statement. Loads config, creates REST client, calls `setup.Run`.

### 4. Update `gopilot init` message

Change output from:
> Created gopilot.yaml — edit it with your GitHub token and repos.

To:
> Created gopilot.yaml — edit it with your GitHub token and repos, then run `gopilot setup` to create labels.
