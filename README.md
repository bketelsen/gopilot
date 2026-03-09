# Gopilot

An AI agent orchestrator that dispatches coding agents to work on GitHub issues autonomously.

![Gopilot Dashboard](docs/assets/dashboard-placeholder.png)

## Key Features

- **Poll-Dispatch-Reconcile loop** -- Continuously watches GitHub for eligible issues, dispatches agents, and reconciles state against the source of truth.
- **Multi-agent support** -- GitHub Copilot CLI and Claude Code, with per-repo or per-label agent overrides.
- **Skills injection** -- Enforce behavioral contracts (TDD, verification, debugging) via composable SKILL.md files injected into agent prompts. [agentskills.io](https://agentskills.io) compatible.
- **Real-time dashboard** -- HTMX + SSE-powered web UI showing active agents, metrics, and logs with live updates.
- **Config hot-reload** -- Change polling intervals, concurrency limits, agent selection, and skill settings without restarting.
- **Retry with backoff** -- Failed agents get exponential backoff retries with stall detection and automatic cleanup.
- **Interactive planning** -- Dashboard-based chat UI where agents explore the codebase and collaboratively build structured plans with you.

## Quick Install

### Homebrew

```bash
brew install bketelsen/tap/gopilot
```

### Download from Releases

Pre-built binaries are available on the
[GitHub Releases](https://github.com/bketelsen/gopilot/releases) page.
Download the archive for your platform, extract it, and place the `gopilot`
binary on your `PATH`.

### Getting Started

Initialize a configuration file and start the orchestrator:

```bash
gopilot init
# Edit gopilot.yaml with your token and repos
gopilot setup
gopilot
```

The `init` command creates a `gopilot.yaml` in the current directory with
sensible defaults. Edit it to add your GitHub token, target repositories,
and preferred agent settings. Run `gopilot setup` to create the required
labels on your repos, then run `gopilot` to start the orchestrator.

## Building from Source

Requirements:

- [Go 1.25+](https://go.dev/dl/)
- [Task](https://taskfile.dev/) (task runner)

```bash
git clone https://github.com/bketelsen/gopilot.git
cd gopilot
task build
```

This generates templ templates, builds Tailwind CSS, and compiles the binary
to `./gopilot`.

Other useful commands:

```bash
task dev       # Build and run
task test      # Run tests with race detector
task lint      # Run golangci-lint
task clean     # Remove build artifacts
```

## Documentation

Full documentation is available at
**[bketelsen.github.io/gopilot](https://bketelsen.github.io/gopilot/)**.

| Guide | Description |
|-------|-------------|
| [Getting Started](https://bketelsen.github.io/gopilot/getting-started/) | Installation, first run, and basic configuration |
| [Configuration](https://bketelsen.github.io/gopilot/configuration/) | All `gopilot.yaml` options explained |
| [Writing Skills](https://bketelsen.github.io/gopilot/skills/) | Create and compose SKILL.md behavioral contracts |
| [CLI Reference](https://bketelsen.github.io/gopilot/cli/) | Command-line flags and environment variables |
| [Architecture](https://bketelsen.github.io/gopilot/architecture/) | System design, interfaces, and data flow |
| [Dashboard](https://bketelsen.github.io/gopilot/dashboard/) | Web UI features and SSE event stream |

## License

Gopilot is released under the [MIT License](LICENSE).
