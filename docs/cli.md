# CLI Reference

## Subcommands

| Command            | Description                                |
| ------------------ | ------------------------------------------ |
| `gopilot`          | Start the orchestrator (default mode)      |
| `gopilot init`     | Interactive guided setup wizard — creates `gopilot.yaml`, selects and extracts skills, creates workspace directory |
| `gopilot setup`    | Ensure required labels exist on all configured repositories (idempotent) |
| `gopilot completion` | Generate shell completion scripts (bash, zsh, fish, powershell) |

## Flags

### Global Flags

| Flag              | Default        | Description                                          |
| ----------------- | -------------- | ---------------------------------------------------- |
| `--config <path>` | `gopilot.yaml` | Path to the configuration file                       |
| `--version`       |                | Print version information                            |

### Orchestrator Flags

These flags apply when running `gopilot` with no subcommand.

| Flag              | Default        | Description                                          |
| ----------------- | -------------- | ---------------------------------------------------- |
| `--dry-run`       | `false`        | List eligible issues without dispatching agents      |
| `--debug`         | `false`        | Enable debug-level logging                           |
| `--port <port>`   | (none)         | Override the dashboard listen port; also enables the dashboard |
| `--log <path>`    | (none)         | Write logs to a file in addition to stderr           |

## Init Wizard

The `gopilot init` command launches an interactive setup wizard intended to be run in an empty directory where gopilot will operate. It walks you through:

1. **GitHub token** — enter your personal access token (auto-detects `$GITHUB_TOKEN` from environment)
2. **Repositories** — comma-separated list of `owner/repo` entries to monitor
3. **Agent selection** — choose between Claude Code or GitHub Copilot CLI
4. **Skills** — select from built-in skills to install (pre-checked defaults: `verification`, `pr-workflow`, `code-review`)
5. **Required vs optional** — choose which selected skills are always injected vs available on-demand

After completing the wizard, it creates:

- `gopilot.yaml` with your settings and sensible defaults
- `skills/` directory with extracted skill files
- `workspaces/` directory for agent workspaces

If `gopilot.yaml` already exists, the wizard prompts to confirm before overwriting.

## Examples

```bash
# Interactive guided setup
gopilot init

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

# Combine flags
gopilot --config prod.yaml --debug --port 8080 --log gopilot.log

# Create labels on all configured repos
gopilot setup

# Use a custom config path
gopilot setup --config /etc/gopilot/production.yaml

# Generate shell completions
gopilot completion bash
```

## Exit Codes

| Code | Meaning                                                      |
| ---- | ------------------------------------------------------------ |
| 0    | Clean shutdown (SIGINT/SIGTERM) or successful dry-run        |
| 1    | Error: config load failure, dry-run failure, or orchestrator error |

## Environment Variables

| Variable       | Description                                                        |
| -------------- | ------------------------------------------------------------------ |
| `GITHUB_TOKEN` | GitHub personal access token, referenced in config via `$GITHUB_TOKEN` |

## Signals

| Signal  | Behavior                                                          |
| ------- | ----------------------------------------------------------------- |
| SIGINT  | Graceful shutdown: cancels context and waits for running agents   |
| SIGTERM | Graceful shutdown: cancels context and waits for running agents   |
