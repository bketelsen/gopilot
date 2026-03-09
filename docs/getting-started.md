# Getting Started

This guide walks you through installing Gopilot, creating a configuration file, and running your first orchestration loop.

## Prerequisites

Before you begin, make sure you have:

- A **GitHub personal access token** with `repo` scope. Gopilot uses this to read issues, post comments, and create branches.
- One or more **GitHub repositories** with issues labeled `gopilot` (or whatever label you configure as eligible).
- **Go 1.25+** -- only required if building from source.
- **Task CLI** ([taskfile.dev](https://taskfile.dev)) -- only required if building from source.

## Installation

=== "Homebrew"

    ```bash
    brew install bketelsen/tap/gopilot
    ```

=== "Binary"

    Download the latest release for your platform from [GitHub Releases](https://github.com/bketelsen/gopilot/releases), extract the archive, and move the `gopilot` binary to a directory on your `PATH`:

    ```bash
    tar xzf gopilot_*.tar.gz
    sudo mv gopilot /usr/local/bin/
    ```

=== "Source"

    ```bash
    git clone https://github.com/bketelsen/gopilot.git
    cd gopilot
    task build
    ```

    The compiled binary will be at `./gopilot` in the project root.

## Initialize Configuration

Run the init command to generate a starter configuration file:

```bash
gopilot init
```

This creates `gopilot.yaml` in the current directory. Open it and edit the key fields:

| Field | Description |
|-------|-------------|
| `github.token` | Your GitHub token. Use `$GITHUB_TOKEN` to read from an environment variable. |
| `github.repos` | List of repositories to watch, in `owner/repo` format. |
| `github.eligible_labels` | Labels that mark an issue as eligible for agent dispatch. |
| `agent.command` | Which agent to use: `copilot` or `claude-code`. |

Here is a minimal working configuration:

```yaml
github:
  token: $GITHUB_TOKEN
  repos:
    - owner/repo
  eligible_labels:
    - gopilot
polling:
  interval_ms: 30000
  max_concurrent_agents: 3
agent:
  command: copilot
  model: claude-sonnet-4.6
dashboard:
  enabled: true
  addr: ":3000"
```

Replace `owner/repo` with your actual repository and make sure `GITHUB_TOKEN` is set in your environment.

## Set Up Repository Labels

Gopilot uses specific GitHub labels to identify eligible issues, plan work, and track failures. Run the setup command to create these labels on all your configured repositories:

```bash
gopilot setup
```

This creates the following labels (or updates them if they already exist):

| Label | Color | Purpose |
|-------|-------|---------|
| `gopilot` | Blue | Marks issues as eligible for agent dispatch |
| `gopilot:plan` | Purple | Triggers interactive planning mode |
| `gopilot:planned` | Green | Applied when planning completes |
| `gopilot-failed` | Red | Applied when an agent fails after max retries |

The command is idempotent — safe to run multiple times.

## First Run

Start with a dry run to verify that Gopilot can connect to GitHub and find eligible issues without actually dispatching any agents:

```bash
gopilot --dry-run
```

You should see log output listing any issues that match your eligible labels. Once you are satisfied, start the orchestrator for real:

```bash
gopilot
```

Gopilot will begin its poll-dispatch-reconcile loop. You will see log messages as it polls for issues, claims them, creates workspaces, and launches agents.

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

## Verify It Works

With Gopilot running:

1. **Open the dashboard** at [http://localhost:3000](http://localhost:3000) to see live agent status, run history, and metrics.
2. **Check the logs** for messages like `polling for eligible issues` and `dispatching agent` to confirm the loop is active.
3. **Review dry-run output** (if you ran `--dry-run` first) to confirm it found the correct issues by title and number.

## Next Steps

- [Configuration](configuration.md) -- Full reference for every `gopilot.yaml` setting.
- [Writing Skills](skills.md) -- Create custom SKILL.md behavioral contracts to shape agent behavior.
- [CLI Reference](cli.md) -- All command-line flags and options.
- [Dashboard](dashboard.md) -- Features of the real-time web UI.
