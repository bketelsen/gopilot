# Documentation & Release Pipeline Design

Date: 2026-03-06
Status: Approved

---

## 1. Problem Statement

Gopilot has no README, no public-facing documentation, no release automation, and no CI pipeline. The project is preparing to go public, and needs all of these before launch.

## 2. Goals

1. **Operator-first documentation**: Help people install, configure, and run Gopilot
2. **Release automation**: One `git tag` produces binaries, homebrew formula, and Docker images
3. **CI pipeline**: Every PR gets tested and linted automatically
4. **Doc site**: Searchable, navigable documentation deployed to GitHub Pages

## 3. Audience

- **Phase 1 (this design)**: Operators who will deploy and run Gopilot
- **Phase 2 (future)**: Contributors who want to understand and extend the codebase

## 4. Deliverables

### 4.1 README.md

Concise landing page (~120 lines):

1. Title + one-line description
2. Dashboard screenshot placeholder (capture once binary is running)
3. Key features bullet list (poll-dispatch-reconcile, multi-agent, skills, dashboard, hot-reload, retry/stall detection)
4. Quick install: `brew install bketelsen/tap/gopilot`
5. Quickstart: `gopilot init` + edit config + `gopilot`
6. Links to doc site pages
7. License

### 4.2 MkDocs Material Site

**Config:** `mkdocs.yml` at project root.

**Pages:**

| File | Purpose |
|------|---------|
| `docs/index.md` | Home page (expanded README) |
| `docs/getting-started.md` | Install methods, init, first run, verify |
| `docs/configuration.md` | Full config reference — all sections with defaults, examples, hook recipes |
| `docs/skills.md` | Writing custom SKILL.md files: frontmatter format, type (rigid/flexible), examples |
| `docs/architecture.md` | System overview, poll-dispatch-reconcile loop, package layout |
| `docs/dashboard.md` | Dashboard features, SSE real-time updates, screenshots |
| `docs/cli.md` | CLI reference: all flags and subcommands (version, init, --dry-run, --debug, --port, --log, --config) |

**Theme features:**
- Material theme with dark/light toggle
- Search enabled
- GitHub repo link + edit button
- Navigation tabs
- Code highlighting for YAML, Go, bash

### 4.3 Goreleaser Pro

**`.goreleaser.yaml`:**

- **Platforms:** Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64)
- **Build:** `cmd/gopilot/main.go` with `-ldflags` setting `Version` from git tag
- **Archives:** tar.gz (Linux/macOS), zip (Windows)
- **Checksums:** SHA256
- **Changelog:** Auto-generated from commits
- **Homebrew:** Push formula to `bketelsen/homebrew-tap`
- **Docker:** Multi-arch images at `ghcr.io/bketelsen/gopilot`
- **Build deps:** `task generate` and `task css` must run before Go build (templ + Tailwind)

### 4.4 GitHub Actions Workflows

**`.github/workflows/release.yml`** (tag-triggered):
- Trigger: push tag `v*`
- Steps: checkout, setup-go, install task, `task generate`, `task css`, goreleaser release
- Secrets: `GORELEASER_KEY`, `GH_PAT` (for homebrew tap push)

**`.github/workflows/docs.yml`** (docs deploy):
- Trigger: push to main (paths: `docs/**`, `mkdocs.yml`)
- Steps: checkout, setup-python, install mkdocs-material, `mkdocs gh-deploy`

**`.github/workflows/ci.yml`** (PR checks):
- Trigger: PR and push to main
- Matrix: Go 1.25, ubuntu-latest
- Steps: checkout, setup-go, install task, `task generate`, `task css`, `task test`, `task lint`
- Caches: Go modules + build cache

## 5. Content Sources

- `CLAUDE.md` — architecture, package layout, build commands, config reference
- `research/SPEC-DRAFT.md` — problem statement, goals, system overview diagram
- `internal/config/example.go` — full example config with comments
- `internal/skills/loader.go` — SKILL.md format (frontmatter + markdown)
- `skills/tdd/SKILL.md` — example skill file
- `cmd/gopilot/main.go` — CLI flags and subcommands

## 6. Non-Goals

- Contributor documentation (phase 2)
- API documentation / godoc (phase 2)
- Tutorials or video content
- Blog posts
