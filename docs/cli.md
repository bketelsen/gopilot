# CLI Reference

## Subcommands

| Command            | Description                                |
| ------------------ | ------------------------------------------ |
| `gopilot`          | Start the orchestrator (default mode)      |
| `gopilot version`  | Print the version string                   |
| `gopilot init`     | Create a default `gopilot.yaml` in the current directory (fails if the file already exists) |
| `gopilot setup`    | Ensure required labels exist on all configured repositories (idempotent) |

## Flags

Flags apply to the default orchestrator mode (`gopilot` with no subcommand).

| Flag              | Default        | Description                                          |
| ----------------- | -------------- | ---------------------------------------------------- |
| `--config <path>` | `gopilot.yaml` | Path to the configuration file                       |
| `--dry-run`       | `false`        | List eligible issues without dispatching agents      |
| `--debug`         | `false`        | Enable debug-level logging                           |
| `--port <port>`   | (none)         | Override the dashboard listen port; also enables the dashboard |
| `--log <path>`    | (none)         | Write logs to a file in addition to stderr           |

## Examples

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

# Combine flags
gopilot --config prod.yaml --debug --port 8080 --log gopilot.log

# Create labels on all configured repos
gopilot setup

# Use a custom config path
gopilot setup /etc/gopilot/production.yaml
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
