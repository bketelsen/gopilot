# Dashboard

## Overview

Gopilot includes a real-time web dashboard for monitoring agent activity, viewing metrics, and managing configuration. The dashboard provides visibility into what agents are doing, how they have performed historically, and what is queued for retry.

## Enabling the Dashboard

The dashboard is enabled by default. To configure it, set the following in `gopilot.yaml`:

```yaml
dashboard:
  enabled: true
  addr: ":3000"
```

| Setting | Default | Description |
|---------|---------|-------------|
| `dashboard.enabled` | `true` | Whether the dashboard server starts |
| `dashboard.addr` | `:3000` | Address and port the dashboard listens on |

The listen address can also be overridden via the `--port` CLI flag.

## Dashboard Pages

### Home / Active Agents

Displays all currently running agents. Each entry shows:

- Issue title and number
- Repository name
- Agent type (Copilot or Claude)
- Runtime duration
- Last event received from the agent

This is the primary view for understanding what Gopilot is doing right now.

### Completed Runs

History of finished agent runs. Each entry includes:

- Issue reference
- Final status (success or failure)
- Total duration
- Token usage

Useful for reviewing agent performance and identifying patterns in failures.

### Retry Queue

Shows agents that failed and are waiting to be retried. Each entry displays:

- Issue reference
- Current attempt number out of max retries
- Backoff countdown (time until next retry)
- Failure reason from the last attempt

### Settings

Displays the current runtime configuration and GitHub API rate limit status. Helpful for verifying that config hot-reloads have taken effect and for monitoring API quota consumption.

### Metrics

Aggregated statistics including:

- Token usage across all agents
- Cost estimates based on token consumption
- Session duration statistics

## Real-Time Updates

The dashboard uses Server-Sent Events (SSE) to push state changes to connected browsers. There is no polling and no manual refresh required. When an agent starts, completes, fails, or emits new events, the dashboard updates automatically.

This is built on top of HTMX, which handles partial page updates seamlessly. When an SSE event arrives, HTMX swaps in the updated HTML fragment without a full page reload. The result is a responsive experience that stays current without user interaction.

## Tech Stack

| Technology | Role |
|------------|------|
| templ | Type-safe Go HTML templates. `.templ` files compile to Go code, ensuring compile-time safety for all rendered markup. |
| Tailwind CSS v4 | Utility-first CSS framework for styling dashboard components. |
| HTMX | Handles SSE subscriptions and partial DOM updates. Enables the real-time behavior without custom JavaScript. |
| SSE event hub | Internal Go component that broadcasts state changes to all connected dashboard clients. |
