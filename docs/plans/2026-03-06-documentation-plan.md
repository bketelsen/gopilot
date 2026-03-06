# Documentation & Release Pipeline Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create README, MkDocs Material doc site, goreleaser pro release pipeline, and CI workflows for Gopilot's public launch.

**Architecture:** Static documentation site built with MkDocs Material, deployed to GitHub Pages via GitHub Actions. Release pipeline uses goreleaser pro to produce multi-platform binaries, homebrew formula, and Docker images. CI workflow runs tests and linting on every PR.

**Tech Stack:** MkDocs Material (Python), goreleaser pro, GitHub Actions, Docker

---

### Task 1: README.md

**Files:**
- Create: `README.md`

**Step 1: Create README.md**

```markdown
# Gopilot

An AI agent orchestrator that dispatches coding agents to work on GitHub issues autonomously.

<!-- TODO: Replace with actual dashboard screenshot -->
![Dashboard](docs/assets/dashboard-placeholder.png)

## Features

- **Poll-Dispatch-Reconcile loop** — Continuously watches GitHub for eligible issues, dispatches agents, and reconciles state
- **Multi-agent support** — GitHub Copilot CLI and Claude Code, with per-repo or per-label overrides
- **Skills injection** — Enforce behavioral contracts (TDD, verification, debugging) via SKILL.md files
- **Real-time dashboard** — HTMX + SSE-powered web UI showing active agents, metrics, and logs
- **Config hot-reload** — Change polling, concurrency, agent, and skill settings without restarting
- **Retry with backoff** — Failed agents get exponential backoff retries with stall detection
- **Interactive planning** — AI-driven issue refinement before coding begins

## Quick Start

```bash
# Install via Homebrew
brew install bketelsen/tap/gopilot

# Or download from GitHub Releases
# https://github.com/bketelsen/gopilot/releases

# Initialize config
gopilot init

# Edit gopilot.yaml with your GitHub token and repos
# Then run:
gopilot
```

## Documentation

Full documentation is available at **[bketelsen.github.io/gopilot](https://bketelsen.github.io/gopilot/)**.

- [Getting Started](https://bketelsen.github.io/gopilot/getting-started/) — Installation, first run, verification
- [Configuration](https://bketelsen.github.io/gopilot/configuration/) — Full config reference with examples
- [Writing Skills](https://bketelsen.github.io/gopilot/skills/) — Create custom SKILL.md behavioral contracts
- [CLI Reference](https://bketelsen.github.io/gopilot/cli/) — All commands and flags
- [Architecture](https://bketelsen.github.io/gopilot/architecture/) — System design and package layout
- [Dashboard](https://bketelsen.github.io/gopilot/dashboard/) — Web UI features and SSE updates

## Building from Source

Requires [Go 1.25+](https://go.dev/) and [Task](https://taskfile.dev/).

```bash
git clone https://github.com/bketelsen/gopilot.git
cd gopilot
task build
./gopilot --help
```

## License

[MIT](LICENSE)
```

**Step 2: Create placeholder assets directory**

Run: `mkdir -p docs/assets`

**Step 3: Commit**

```bash
git add README.md docs/assets/
git commit -m "docs: add README with features, quickstart, and doc links"
```

---

### Task 2: MkDocs Configuration

**Files:**
- Create: `mkdocs.yml`

**Step 1: Create mkdocs.yml**

```yaml
site_name: Gopilot
site_url: https://bketelsen.github.io/gopilot/
site_description: An AI agent orchestrator for GitHub Issues
repo_url: https://github.com/bketelsen/gopilot
repo_name: bketelsen/gopilot
edit_uri: edit/main/docs/

theme:
  name: material
  palette:
    - scheme: default
      primary: indigo
      accent: indigo
      toggle:
        icon: material/brightness-7
        name: Switch to dark mode
    - scheme: slate
      primary: indigo
      accent: indigo
      toggle:
        icon: material/brightness-4
        name: Switch to light mode
  features:
    - navigation.tabs
    - navigation.sections
    - navigation.top
    - search.suggest
    - search.highlight
    - content.code.copy
    - content.action.edit

markdown_extensions:
  - admonition
  - pymdownx.details
  - pymdownx.superfences
  - pymdownx.highlight:
      anchor_linenums: true
  - pymdownx.inlinehilite
  - pymdownx.tabbed:
      alternate_style: true
  - tables
  - attr_list

nav:
  - Home: index.md
  - Getting Started: getting-started.md
  - Configuration: configuration.md
  - Writing Skills: skills.md
  - CLI Reference: cli.md
  - Architecture: architecture.md
  - Dashboard: dashboard.md
```

**Step 2: Commit**

```bash
git add mkdocs.yml
git commit -m "docs: add MkDocs Material configuration"
```

---

### Task 3: Doc Site — Home + Getting Started

**Files:**
- Create: `docs/index.md`
- Create: `docs/getting-started.md`

**Step 1: Create docs/index.md**

Expanded version of README: same features list, plus a brief "How it works" section explaining the poll-dispatch-reconcile loop in 3-4 sentences, linking to architecture page for details.

**Step 2: Create docs/getting-started.md**

Sections:
1. **Prerequisites** — GitHub token with `repo` scope, Go 1.25+ (if building from source), target repos with issues labeled `gopilot`
2. **Installation** — Three methods: Homebrew (`brew install bketelsen/tap/gopilot`), GitHub Releases (download binary), From source (`task build`)
3. **Initialize Configuration** — `gopilot init` creates `gopilot.yaml`, explain required edits (token, repos)
4. **First Run** — `gopilot` starts the orchestrator, `gopilot --dry-run` to verify without dispatching
5. **Verify It Works** — Check dashboard at `http://localhost:3000`, look for poll log messages, verify dry-run output shows eligible issues
6. **Next Steps** — Links to configuration, skills, dashboard pages

Content sources: `internal/config/example.go` for the default config, `cmd/gopilot/main.go` for CLI flags.

**Step 3: Commit**

```bash
git add docs/index.md docs/getting-started.md
git commit -m "docs: add home page and getting started guide"
```

---

### Task 4: Doc Site — Configuration Reference

**Files:**
- Create: `docs/configuration.md`

**Step 1: Create docs/configuration.md**

Document every config section from `internal/config/config.go`. For each field: name, type, default, description.

Sections:
1. **Overview** — Config lives in `gopilot.yaml`, supports hot-reload via fsnotify (except token and repos)
2. **`github`** — token ($GITHUB_TOKEN env var), repos list, project (owner, number), eligible_labels, excluded_labels
3. **`polling`** — interval_ms (default: 30000), max_concurrent_agents (default: 3)
4. **`workspace`** — root directory, hook_timeout_ms (default: 60000), hooks subsection
5. **`workspace.hooks`** — after_create, before_run, after_run, before_remove. Include variable interpolation docs: `{{repo}}`, `{{issue_id}}`, `${GITHUB_TOKEN}`. Include hook recipe examples (clone repo, checkout branch, run setup scripts)
6. **`agent`** — command (copilot/claude/claude-code), model, max_autopilot_continues (default: 20), turn_timeout_ms (default: 1800000 = 30min), stall_timeout_ms (default: 300000 = 5min), max_retry_backoff_ms (default: 300000), max_retries (default: 3), overrides (per-repo or per-label agent selection)
7. **`skills`** — dir, required list, optional list
8. **`dashboard`** — enabled (bool), addr (default: ":3000")
9. **`planning`** — label (default: "gopilot:plan"), completed_label, approve_command, max_questions, agent, model
10. **`prompt`** — Go text/template with available variables (.Issue.Repo, .Issue.ID, .Issue.Title, .Issue.Labels, .Issue.Body, .Issue.Priority). Include joinStrings function.
11. **Full Example** — The complete config from `internal/config/example.go`

**Step 2: Commit**

```bash
git add docs/configuration.md
git commit -m "docs: add full configuration reference"
```

---

### Task 5: Doc Site — Writing Skills Guide

**Files:**
- Create: `docs/skills.md`

**Step 1: Create docs/skills.md**

Content sources: `internal/skills/loader.go` for format, `skills/tdd/SKILL.md` for example.

Sections:
1. **Overview** — Skills are behavioral contracts injected into agent prompts. They enforce workflows like TDD, verification, debugging.
2. **SKILL.md Format** — YAML frontmatter (name, description, type) + markdown body. Show the schema:
   ```
   ---
   name: skill-name
   description: When to use this skill
   type: rigid|flexible
   ---

   Markdown content injected into agent prompt...
   ```
3. **Frontmatter Fields** — name (required, unique identifier), description (when to trigger), type (rigid = follow exactly, flexible = adapt to context)
4. **Directory Structure** — Skills live in a directory (default: `./skills`). Each skill is either `skills/<name>/SKILL.md` or `skills/<name>.md`. The loader walks up to 4 levels deep.
5. **Required vs Optional** — Required skills are always injected. Optional skills are injected based on context.
6. **Built-in Skills** — List the 5 included skills with descriptions: tdd, verification, debugging, code-review, planning
7. **Writing Your Own** — Step-by-step: create directory, create SKILL.md with frontmatter, add to config, test with dry-run
8. **Example** — Full TDD skill as example (from `skills/tdd/SKILL.md`)
9. **Tips** — Keep skills focused, use tables for "red flags" patterns, rigid skills for non-negotiable workflows

**Step 2: Commit**

```bash
git add docs/skills.md
git commit -m "docs: add skills authoring guide"
```

---

### Task 6: Doc Site — CLI Reference

**Files:**
- Create: `docs/cli.md`

**Step 1: Create docs/cli.md**

Content source: `cmd/gopilot/main.go` lines 22-38.

Sections:
1. **Subcommands**:
   - `gopilot` (no args) — Start the orchestrator
   - `gopilot version` — Print version
   - `gopilot init` — Create default `gopilot.yaml` in current directory
2. **Flags** (for the main orchestrator command):
   - `--config <path>` — Path to config file (default: `gopilot.yaml`)
   - `--dry-run` — List eligible issues without dispatching agents
   - `--debug` — Enable debug-level logging
   - `--port <port>` — Override dashboard listen port (enables dashboard)
   - `--log <path>` — Write logs to file (in addition to stderr)
3. **Examples**:
   ```bash
   # Start with default config
   gopilot

   # Dry run to see eligible issues
   gopilot --dry-run

   # Custom config with debug logging
   gopilot --config /etc/gopilot/production.yaml --debug

   # Override dashboard port
   gopilot --port 8080

   # Log to file
   gopilot --log /var/log/gopilot.log
   ```
4. **Exit Codes** — 0 success, 1 error (config load failure, dry-run failure, orchestrator error)
5. **Environment Variables** — `GITHUB_TOKEN` referenced in config via `$GITHUB_TOKEN`

**Step 2: Commit**

```bash
git add docs/cli.md
git commit -m "docs: add CLI reference"
```

---

### Task 7: Doc Site — Architecture + Dashboard

**Files:**
- Create: `docs/architecture.md`
- Create: `docs/dashboard.md`

**Step 1: Create docs/architecture.md**

Content sources: `CLAUDE.md` (architecture section, package layout), `research/SPEC-DRAFT.md` (system overview diagram).

Sections:
1. **System Overview** — Include the ASCII system diagram from SPEC-DRAFT.md
2. **Poll-Dispatch-Reconcile Loop** — Expand the 5-step description from CLAUDE.md with more context for each step
3. **Key Interfaces** — github.Client, workspace.Manager, agent.Runner, web providers. Brief description of each.
4. **State Management** — In-memory state with RWMutex, GitHub as source of truth
5. **Config Hot-Reload** — What reloads (polling, concurrency, agent, skills, prompt) and what doesn't (token, repos)
6. **Package Layout** — The table from CLAUDE.md

**Step 2: Create docs/dashboard.md**

Sections:
1. **Overview** — Real-time web dashboard for monitoring agents
2. **Enabling** — `dashboard.enabled: true` in config, default port 3000
3. **Features** — Active agents view, completed runs, retry queue, settings/config, metrics (token usage, costs)
4. **Real-Time Updates** — SSE (Server-Sent Events) pushes updates to the browser without polling
5. **Screenshot placeholder** — TODO image

**Step 3: Commit**

```bash
git add docs/architecture.md docs/dashboard.md
git commit -m "docs: add architecture overview and dashboard guide"
```

---

### Task 8: Dockerfile

**Files:**
- Create: `Dockerfile`

**Step 1: Create Dockerfile**

Multi-stage build:
- **Stage 1 (build):** `golang:1.25` base, install Task CLI, copy source, run `task generate`, `task css`, `task build`
- **Stage 2 (runtime):** `gcr.io/distroless/static-debian12` base, copy binary, set entrypoint

```dockerfile
FROM golang:1.25 AS build
WORKDIR /src
RUN sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN task build

FROM gcr.io/distroless/static-debian12
COPY --from=build /src/gopilot /usr/local/bin/gopilot
ENTRYPOINT ["gopilot"]
```

Note: The Tailwind CSS standalone CLI download in `task tailwindcss` downloads the linux-x64 binary, which works in the golang:1.25 Docker image.

**Step 2: Create .dockerignore**

```
.git
workspaces
test-workspaces
research
docs/plans
*.md
!README.md
bin/
gopilot
```

**Step 3: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "build: add multi-stage Dockerfile"
```

---

### Task 9: Goreleaser Pro Configuration

**Files:**
- Create: `.goreleaser.yaml`

**Step 1: Create .goreleaser.yaml**

```yaml
version: 2

before:
  hooks:
    - sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
    - task generate
    - task css

builds:
  - main: ./cmd/gopilot
    binary: gopilot
    ldflags:
      - -s -w -X main.Version={{.Version}}
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64

archives:
  - format: tar.gz
    format_overrides:
      - goos: windows
        format: zip
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: checksums.txt

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"

brews:
  - repository:
      owner: bketelsen
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    homepage: https://github.com/bketelsen/gopilot
    description: An AI agent orchestrator for GitHub Issues
    license: MIT
    install: |
      bin.install "gopilot"
    test: |
      system "#{bin}/gopilot", "version"

dockers:
  - image_templates:
      - "ghcr.io/bketelsen/gopilot:{{ .Version }}-amd64"
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
    dockerfile: Dockerfile.goreleaser
  - image_templates:
      - "ghcr.io/bketelsen/gopilot:{{ .Version }}-arm64"
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
    goarch: arm64
    dockerfile: Dockerfile.goreleaser

docker_manifests:
  - name_template: "ghcr.io/bketelsen/gopilot:{{ .Version }}"
    image_templates:
      - "ghcr.io/bketelsen/gopilot:{{ .Version }}-amd64"
      - "ghcr.io/bketelsen/gopilot:{{ .Version }}-arm64"
  - name_template: "ghcr.io/bketelsen/gopilot:latest"
    image_templates:
      - "ghcr.io/bketelsen/gopilot:{{ .Version }}-amd64"
      - "ghcr.io/bketelsen/gopilot:{{ .Version }}-arm64"
```

**Step 2: Create Dockerfile.goreleaser**

Goreleaser builds the binary itself, so this Dockerfile just copies it:

```dockerfile
FROM gcr.io/distroless/static-debian12
COPY gopilot /usr/local/bin/gopilot
ENTRYPOINT ["gopilot"]
```

**Step 3: Commit**

```bash
git add .goreleaser.yaml Dockerfile.goreleaser
git commit -m "build: add goreleaser pro config with homebrew tap and Docker"
```

---

### Task 10: GitHub Actions — CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

**Step 1: Create directory and workflow**

Run: `mkdir -p .github/workflows`

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true

      - name: Install Task
        uses: arduino/setup-task@v2
        with:
          version: 3.x

      - name: Generate templates
        run: task generate

      - name: Build CSS
        run: task css

      - name: Run tests
        run: task test

      - name: Lint
        run: task lint
```

**Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add test and lint workflow for PRs"
```

---

### Task 11: GitHub Actions — Release Workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Step 1: Create release workflow**

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write
  packages: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true

      - name: Login to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser-pro
          version: "~> 2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GORELEASER_KEY: ${{ secrets.GORELEASER_KEY }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

**Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add goreleaser pro release workflow"
```

---

### Task 12: GitHub Actions — Docs Deployment

**Files:**
- Create: `.github/workflows/docs.yml`

**Step 1: Create docs workflow**

```yaml
name: Deploy Docs

on:
  push:
    branches: [main]
    paths:
      - "docs/**"
      - "mkdocs.yml"

permissions:
  contents: write

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"

      - name: Install MkDocs Material
        run: pip install mkdocs-material

      - name: Deploy to GitHub Pages
        run: mkdocs gh-deploy --force
```

**Step 2: Commit**

```bash
git add .github/workflows/docs.yml
git commit -m "ci: add MkDocs deployment workflow"
```

---

### Task 13: Final Verification

**Step 1: Verify MkDocs builds locally**

Run: `pip install mkdocs-material && mkdocs build --strict`
Expected: Site builds with no warnings.

**Step 2: Verify all files are committed**

Run: `git status`
Expected: Clean working tree (no untracked files related to this work).

**Step 3: Review file tree**

Run: `find . -name "*.md" -path "./docs/*" | sort`
Expected:
```
./docs/architecture.md
./docs/cli.md
./docs/configuration.md
./docs/dashboard.md
./docs/getting-started.md
./docs/index.md
./docs/skills.md
```

Run: `ls .github/workflows/`
Expected: `ci.yml  docs.yml  release.yml`

Run: `ls .goreleaser.yaml Dockerfile Dockerfile.goreleaser mkdocs.yml README.md`
Expected: All files exist.
