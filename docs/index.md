# Gopilot

**An AI agent orchestrator for GitHub Issues**

## What is Gopilot

Gopilot watches your GitHub repositories for issues with eligible labels and automatically dispatches AI coding agents -- GitHub Copilot CLI or Claude Code -- to work on them in isolated workspaces. It injects behavioral contracts called "skills" into agent prompts, monitors running agents with stall detection and automatic retries, and provides a real-time web dashboard so you can see what is happening at a glance. For a deeper look at how everything fits together, see the [Architecture](architecture.md) page.

## Key Features

- **Poll-Dispatch-Reconcile loop** -- Continuously watches for eligible issues, dispatches agents, and reconciles state when issues close or become ineligible.
- **Multi-agent support** -- Works with GitHub Copilot CLI and Claude Code out of the box.
- **Skills injection** -- SKILL.md behavioral contracts are rendered into agent prompts, giving you fine-grained control over agent behavior.
- **Real-time dashboard** -- An HTMX + SSE powered web UI shows live agent status, logs, and metrics.
- **Config hot-reload** -- Change `gopilot.yaml` and settings take effect immediately via fsnotify, no restart required.
- **Retry with exponential backoff + stall detection** -- Failed or stalled agents are automatically retried with configurable limits.
- **Interactive planning** -- Agents can propose plans and wait for human approval before proceeding.

## Quick Start

```bash
brew install bketelsen/tap/gopilot
gopilot init
gopilot
```

That is all it takes to get running. See the [Getting Started](getting-started.md) guide for the full walkthrough, including configuration and verification steps.

## How It Works

Gopilot runs a continuous **poll-dispatch-reconcile** loop:

- **Poll** -- Fetches open GitHub issues with eligible labels from your configured repositories. Issues that are already claimed, blocked, or in progress are filtered out.
- **Dispatch** -- Claims an eligible issue, creates an isolated workspace, renders a prompt with injected skills, and launches an AI coding agent as a subprocess.
- **Reconcile** -- Monitors running agents and stops any whose issues have been closed, reassigned, or otherwise become ineligible.

For a deep dive into the architecture, see the [Architecture](architecture.md) page.
