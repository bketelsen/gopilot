# Architecture

## System Overview

Gopilot is an orchestrator that dispatches AI coding agents (GitHub Copilot CLI, Claude Code CLI) to work on GitHub issues. It polls for eligible issues, claims them, creates workspaces, renders prompts with injected skills, runs agents as subprocesses, and monitors them with retry and stall detection.

```
┌─────────────────────────────────────────────────────────────┐
│                        GOPILOT                              │
│                                                             │
│  ┌──────────┐  ┌──────────────┐  ┌───────────────────────┐  │
│  │ Skill    │  │   GitHub     │  │    Orchestrator       │  │
│  │ Loader   │──│   Client     │──│  (poll/dispatch/       │  │
│  │          │  │              │  │   reconcile loop)     │  │
│  └──────────┘  └──────────────┘  └───────────┬───────────┘  │
│                                              │              │
│  ┌──────────┐  ┌──────────────┐  ┌───────────┴───────────┐  │
│  │ Prompt   │  │  Workspace   │  │    Agent Runner       │  │
│  │ Renderer │──│  Manager     │──│  (Copilot / Claude)   │  │
│  │          │  │              │  │                       │  │
│  └──────────┘  └──────────────┘  └───────────────────────┘  │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Web Dashboard (HTMX + SSE)              │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
         │                              │
         ▼                              ▼
   GitHub API                    Agent subprocess
   (issues, PRs,                 (Copilot CLI or
    Projects v2)                  Claude Code)
```

## Poll-Dispatch-Reconcile Loop

The orchestrator runs a continuous loop with three phases, plus stall detection and retry logic.

### Poll

Fetches open issues from configured repos via GitHub REST API. Filters by `eligible_labels` and excludes issues matching `excluded_labels`. Issues already being worked on (tracked in in-memory state) are skipped. The number of concurrent agents is capped by the `max_concurrent_agents` setting.

### Dispatch

Once an eligible issue is found:

1. **Claim** -- Adds an "in-progress" label (or similar) to the issue so other Gopilot instances or humans know it is taken.
2. **Create workspace** -- Creates an isolated workspace directory on disk.
3. **Run `after_create` hook** -- Typically performs `git clone` of the repository into the workspace.
4. **Run `before_run` hook** -- Typically checks out a feature branch for the issue.
5. **Render prompt** -- Evaluates the prompt template with issue data and injected skills.
6. **Launch agent** -- Starts the agent subprocess (Copilot CLI or Claude Code) with the rendered prompt.
7. **Monitor** -- Spawns a background goroutine to watch agent output and lifecycle.

### Reconcile

On each poll cycle, the orchestrator checks all running agents against current issue state. Agents whose issues have been closed, reassigned, or had their eligible labels removed are stopped and their workspaces cleaned up.

### Agent Output Capture

Agent stdout is captured via `io.Pipe` and scanned line by line. Each line updates the `RunEntry` fields (`LastMessage`, `LastEventAt`, `TurnCount`) thread-safely and is stored in a `RingBuffer` (last 50 lines). Per-issue SSE events (`agent-output-{id}`) are broadcast with HTML fragments so the dashboard can stream output in real time.

### Stall Detection

The orchestrator monitors agent output continuously. If an agent has not emitted any stdout within `stall_timeout_ms`, it is considered stalled and its process is killed. Because `LastEventAt` is now updated on every output line, stall detection correctly resets whenever the agent is actively producing output.

### Retry

Failed agents enter a retry queue with exponential backoff. Each subsequent retry waits longer before re-dispatching. After `max_retries` attempts are exhausted, the issue is reset with a failure label so it can be triaged by a human.

## Key Interfaces

| Interface | Description |
|-----------|-------------|
| `github.Client` | GitHub REST and GraphQL operations including issue management, label manipulation, PR tracking, and rate limit tracking. Includes `FetchLabeledIssues` for sprint views and `FetchLinkedPullRequests` for PR status detection. |
| `workspace.Manager` | Workspace lifecycle management (create and remove directories) plus hook execution with variable interpolation. Implemented by `FSManager`. |
| `agent.Runner` | Agent launcher interface. `CopilotRunner` and `ClaudeRunner` implement this for their respective CLI tools, handling subprocess creation, output streaming, and termination. |
| `web.StateProvider` | Provides current agent state to the dashboard without creating circular imports between orchestrator and web packages. |
| `web.MetricsProvider` | Exposes token usage and cost data to the dashboard. |
| `web.RetryProvider` | Exposes retry queue state to the dashboard. |
| `web.SprintProvider` | Provides issue and PR data from GitHub for the sprint board view. |

## State Management

All runtime state lives in-memory within `orchestrator.State`, protected by a `sync.RWMutex`. This includes the set of active agents, their associated issues, and event histories.

GitHub serves as the source of truth. If Gopilot restarts, it has no persistent local state to recover. Instead, on the next poll cycle it re-discovers the current situation from GitHub: which issues are open, which labels are applied, and which issues need work. This design keeps the system simple and avoids state synchronization bugs.

## Config Hot-Reload

Gopilot uses fsnotify to watch `gopilot.yaml` for changes. When the file is modified, the following settings are reloaded without restarting the process:

- Polling interval and batch size
- Concurrency limits (`max_concurrent_agents`)
- Agent configuration (type, timeout, stall detection)
- Skills configuration
- Prompt templates

The following settings require a full restart to take effect:

- GitHub token
- Repository list

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
| `internal/skills/` | [agentskills.io](https://agentskills.io/specification)-compliant SKILL.md loader and injector. Supports multi-directory discovery (config-level + workspace `.agents/skills/`), with workspace skills overriding config-level skills by name. Uses progressive disclosure: required skills are injected as full content, optional skills appear as a catalog with file paths for on-demand activation. |
| `internal/metrics/` | Token tracking and cost estimation |
| `internal/config/` | YAML config structs, loader, fsnotify watcher |
| `internal/planning/` | Dashboard-based interactive planning sessions (WebSocket chat, plan parser) |
| `internal/domain/` | Core types: Issue, RunEntry, CompletedRun, AgentEvent, RingBuffer |
| `components/` | Reusable templ UI components (button, card, dialog, etc.) |
| `skills/` | Runtime skill definitions (SKILL.md files) |
