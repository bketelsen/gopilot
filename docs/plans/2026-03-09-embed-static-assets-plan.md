# Embed Static Assets Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Embed static CSS assets in the gopilot binary so it can run from any directory without needing the source tree.

**Architecture:** Use Go's `go:embed` to embed `internal/web/static/` into the binary. Replace `http.FileServer(http.Dir(...))` with `http.FileServer(http.FS(...))` using `io/fs.Sub` to strip the `static/` prefix.

**Tech Stack:** Go `embed` package, `io/fs`

---

### Task 1: Embed static assets in server.go

**Files:**
- Modify: `internal/web/server.go:1-19` (imports), `internal/web/server.go:99` (file server line)

**Step 1: Add embed import and directive**

Add `"embed"` and `"io/fs"` to the import block, and add the `go:embed` directive with the package-level var. Place the directive and var after the import block, before the interface declarations:

```go
import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/bketelsen/gopilot/internal/skills"
	"github.com/bketelsen/gopilot/internal/web/templates/pages"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed static/*
var staticFiles embed.FS
```

**Step 2: Replace file server with embedded FS**

Change line 99 from:
```go
r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))
```

To:
```go
staticSub, _ := fs.Sub(staticFiles, "static")
r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
```

The `fs.Sub` call strips the `static/` prefix so that `/static/styles.css` maps to `styles.css` within the embedded FS. The error from `fs.Sub` is safe to ignore here since the path is a compile-time constant.

**Step 3: Build and verify**

Run: `cd /home/debian/gopilot && task build`
Expected: Clean build, no errors.

**Step 4: Verify static serving works from a non-project directory**

Run:
```bash
mkdir -p /tmp/gopilot-test
cp gopilot.yaml /tmp/gopilot-test/
cd /tmp/gopilot-test && /home/debian/gopilot/gopilot --dry-run &
sleep 2
curl -s http://localhost:3000/static/styles.css | head -5
kill %1
```
Expected: CSS content is returned (not a 404).

**Step 5: Commit**

```bash
git add internal/web/server.go
git commit -m "feat: embed static assets in binary for portable deployment"
```

---

### Task 2: Update documentation

**Files:**
- Modify: `docs/getting-started.md`
- Modify: `docs/configuration.md`

**Step 1: Add "Running from a project directory" section to getting-started.md**

After the "First Run" section (after line 113), add:

```markdown
## Running from a Project Directory

The gopilot binary is fully self-contained — all dashboard assets are embedded in the binary. You can place it on your `PATH` and run it from any directory.

A typical per-project setup looks like this:

```
~/projects/my-project/
├── gopilot.yaml        # project-specific config
├── skills/             # project-specific skills (optional)
└── workspaces/         # created at runtime
```

All relative paths in `gopilot.yaml` (such as `workspace.root` and `skills.dir`) resolve relative to the directory where you run the `gopilot` command, not relative to the binary location.
```

**Step 2: Add a note to the workspace.root field in configuration.md**

In the `workspace` table description for `root` (line 65), update the description from:

```
| `root` | string | — | Base directory where per-issue workspaces are created. |
```

To:

```
| `root` | string | — | Base directory where per-issue workspaces are created. Relative paths resolve from the working directory where `gopilot` is launched. |
```

**Step 3: Add the same note to skills.dir**

In the `skills` table description for `dir` (line 188), update from:

```
| `dir` | string | — | Config-level directory containing SKILL.md files. The loader walks this directory up to 4 levels deep. |
```

To:

```
| `dir` | string | — | Config-level directory containing SKILL.md files. The loader walks this directory up to 4 levels deep. Relative paths resolve from the working directory where `gopilot` is launched. |
```

**Step 4: Commit**

```bash
git add docs/getting-started.md docs/configuration.md
git commit -m "docs: document portable binary and harness directory pattern"
```
