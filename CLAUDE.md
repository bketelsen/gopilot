# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Gopilot

Gopilot is an orchestrator that dispatches AI coding agents (GitHub Copilot CLI, Claude Code CLI) to work on GitHub issues. It polls for eligible issues, claims them, creates workspaces, renders prompts with injected skills, runs agents as subprocesses, and monitors them with retry/stall detection. Includes a real-time web dashboard.

## Build & Development Commands

Uses [Task](https://taskfile.dev/) as the build system (see `Taskfile.yml`):

```bash
task build          # Generate templ + CSS, then build binary (output: ./gopilot)
task dev            # Build and run the binary
task test           # Run tests with race detector: go test -race ./...
task lint           # Install (if needed) and run golangci-lint
task generate       # Generate templ templates: go tool templ generate
task css            # Build Tailwind CSS
task clean          # Remove build artifacts
task tailwindcss    # Download Tailwind CSS v4 standalone CLI
```

To run a single test:
```bash
go test -race -run TestName ./internal/package/...
```

Build order matters: `generate` must run before `css`, both before `build`.

## Architecture

**Poll-Dispatch-Reconcile loop** (`internal/orchestrator/`):
1. **Poll**: Fetch open GitHub issues with eligible labels, filter by status/blocking checks
2. **Dispatch**: Claim issue → create workspace (with hooks) → render prompt (with skills) → launch agent subprocess → monitor in background goroutine
3. **Reconcile**: Stop agents whose issues became terminal/ineligible
4. **Stall detection**: Kill agents that haven't emitted events within timeout
5. **Retry**: Failed agents get exponential backoff retries; after max retries, issue is reset with failure label

**Key interfaces** (all in `internal/`):
- `github.Client` — GitHub REST + GraphQL operations (mocked in tests)
- `workspace.Manager` — Workspace lifecycle + hook execution (`FSManager` impl)
- `agent.Runner` — Agent launcher (`CopilotRunner`, `ClaudeRunner` impls)
- `web.StateProvider/MetricsProvider/RetryProvider` — Dashboard data access (avoids circular imports)

**State**: All runtime state is in-memory (`orchestrator.State` with RWMutex); GitHub is source of truth.

**Config hot-reload**: fsnotify watches `gopilot.yaml`; reloads polling, concurrency, agent, skills, prompt settings. Does NOT reload token or repos.

## Tech Stack

- **Go 1.25** with `go tool` for templ
- **chi/v5** HTTP router for web dashboard
- **templ** (`.templ` files) for type-safe UI components → generates `*_templ.go` files
- **Tailwind CSS v4** for styling
- **HTMX** for interactive UI updates
- **SSE** (Server-Sent Events) for real-time dashboard updates
- **Go text/template** for prompt rendering with skill injection

## Package Layout

| Package | Purpose |
|---------|---------|
| `cmd/gopilot/` | Entry point, wires everything together |
| `internal/orchestrator/` | Main loop, state management, retry queue |
| `internal/github/` | REST + GraphQL client, rate limit tracking |
| `internal/agent/` | Runner interface, Copilot/Claude subprocess adapters |
| `internal/workspace/` | Workspace creation, hook execution |
| `internal/web/` | Dashboard server, SSE hub, templ page templates |
| `internal/prompt/` | Prompt template rendering |
| `internal/skills/` | SKILL.md loader (frontmatter + markdown), injector |
| `internal/metrics/` | Token tracking and cost estimation |
| `internal/config/` | YAML config structs, loader, fsnotify watcher |
| `internal/domain/` | Core types: Issue, RunEntry, CompletedRun, AgentEvent |
| `skills/` | Runtime skill definitions (SKILL.md files) |

## Code Quality

- **Always run `task lint` before committing code changes.** Fix all lint issues before considering work complete.
- The linter (`golangci-lint`) is auto-installed to `bin/` on first run via the `golangci-lint` task dependency.

## Testing Patterns

- Table-driven tests with stdlib `testing` package
- GitHub client is mocked via interface for unit tests
- Always run with `-race` flag
- Test files are colocated with source (`*_test.go`)

## Configuration

Runtime config is in `gopilot.yaml` (see `internal/config/example.go` for defaults). Key sections: `github`, `polling`, `workspace` (with hooks), `agent`, `skills`, `dashboard`, `prompt`.

Workspace hooks support variable interpolation: `{{repo}}`, `{{issue_id}}`, `${GITHUB_TOKEN}`.

## Documentation

This project has two documentation surfaces that **must be kept in sync** with code changes:

1. **`README.md`** — Project overview, quick-start instructions, and feature summary.
2. **MkDocs site** (`docs/` directory, configured by `mkdocs.yml`) — Full user-facing documentation including getting started, configuration, skills, CLI reference, architecture, and dashboard guides.

### When to update docs

Any change that affects user-facing behavior, configuration, CLI flags, new features, removed features, architecture, or the build/development workflow **must** include corresponding documentation updates. Specifically:

- **New feature or package**: Add/update the relevant `docs/*.md` page and update `README.md` if it changes the project summary or quick-start flow. If a new top-level doc page is needed, add it to the `nav:` section in `mkdocs.yml`.
- **Changed configuration options**: Update `docs/configuration.md` and any examples in `README.md`.
- **New or changed CLI commands/flags**: Update `docs/cli.md`.
- **Architecture changes** (new packages, changed interfaces, modified flows): Update `docs/architecture.md` and the Package Layout table above.
- **Dashboard changes**: Update `docs/dashboard.md`.
- **Skills changes**: Update `docs/skills.md`.
- **Build/dev workflow changes**: Update the Build & Development Commands section above, `docs/getting-started.md`, and `README.md` if applicable.

### Doc structure

| File | Content |
|------|---------|
| `README.md` | Project intro, badges, feature highlights, quick-start, links to full docs |
| `docs/index.md` | MkDocs home page |
| `docs/getting-started.md` | Installation and first-run guide |
| `docs/configuration.md` | `gopilot.yaml` reference |
| `docs/skills.md` | Writing and using SKILL.md files |
| `docs/cli.md` | CLI flags and usage |
| `docs/architecture.md` | System design and package overview |
| `docs/dashboard.md` | Web dashboard features and usage |
| `mkdocs.yml` | MkDocs site configuration and nav structure |
