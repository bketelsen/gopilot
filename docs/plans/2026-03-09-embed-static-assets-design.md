# Design: Embed Static Assets in Binary

**Date:** 2026-03-09
**Status:** Approved

## Problem

Static CSS is served from `internal/web/static/` via a hardcoded relative path (`http.FileServer(http.Dir("internal/web/static"))`). This means the binary must be run from the project root directory, making it non-portable.

## Goal

Make the gopilot binary fully self-contained so it can be run from any directory — specifically a per-project "harness directory" containing only:

```
~/projects/my-project/
├── gopilot.yaml        # project-specific config
├── skills/             # project-specific skills
└── workspaces/         # created at runtime (default relative to CWD)
```

## Solution

Use `go:embed` to embed the `static/` directory into the binary. Replace `http.FileServer(http.Dir(...))` with `http.FileServer(http.FS(...))`.

## Changes

1. **`internal/web/server.go`** — Add `//go:embed static/*` directive, create an `embed.FS`, replace disk-based file server with embedded FS.
2. **Docs** — Document the harness directory pattern and clarify that the binary is portable.

## Non-goals

- No `-static-dir` override flag (not needed; `task dev` rebuilds the binary anyway).
- No changes to skills loading (already configurable via `skills.dir` in config).
- No changes to workspace root (already configurable, defaults to `./workspaces` relative to CWD).
