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
- **Last Output** -- The most recent line of agent stdout, updated in real time

This is the primary view for understanding what Gopilot is doing right now.

### Issue Detail

Each running issue links to an issue detail page (`/issues/{owner}/{repo}/{id}`) that shows:

- Issue metadata (title, labels, priority, attempt)
- **Live Output panel** -- Streams agent stdout in real time via HTMX + SSE. Pre-populated with the last 50 lines from the output buffer and appended to as the agent produces more output.
- Session history table with past runs

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

### Planning

Interactive planning sessions powered by a WebSocket-based chat UI. Start a session by clicking "New Planning Session" on the dashboard or navigating to `/planning`.

**Starting a session:**

1. Select a repository from the configured repos dropdown.
2. Optionally link a GitHub issue number for context.
3. Click "Start" to create the session and open the chat.

**During a session:**

The planning agent runs as a subprocess with full access to a workspace checkout of the repository. It can explore the codebase, read files, and cite specific code while discussing the feature with you. The conversation starts freeform and converges toward a structured plan.

Agent responses stream in real-time over WebSocket. The input is disabled while the agent is working and re-enabled when it finishes each turn.

**Completing a session:**

When the agent proposes a structured plan, you can choose what to do with it:

- **Create GitHub Issues** -- Each checked task becomes a GitHub issue with labels, dependencies, and phase metadata.
- **Save Plan Doc** -- The plan is saved as a markdown file in the repository.
- **Both** -- Creates issues and saves the document.

**Reconnecting:**

If you close the browser tab and reopen the session URL, the full conversation history is replayed so you can continue where you left off.

### Sprint View

Available at `/sprint`. Displays a Kanban-style board with four columns reflecting the real state of all issues with the eligible label:

| Column | Source |
|--------|--------|
| **Todo** | Open issues that are not claimed, not running, and have no linked PRs |
| **In Progress** | Issues with an actively running agent |
| **In Review** | Issues that have at least one open (unmerged) pull request |
| **Done** | Issues with a merged pull request, or closed issues |

The sprint view fetches both open and recently closed issues from GitHub, then enriches each issue with linked pull request data via the timeline events API. A progress bar shows completion (Done / Total).

If the sprint provider is not configured, the view falls back to showing only actively running agents in the "In Progress" column.

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
| WebSocket (nhooyr.io/websocket) | Bidirectional communication for interactive planning chat sessions. |
