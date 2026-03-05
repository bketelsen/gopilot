# Gopilot Full Rewrite — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Clean slate rewrite of Gopilot — a GitHub issue-to-PR orchestrator that dispatches AI coding agents to isolated workspaces with behavioral contracts.

**Architecture:** Poll-dispatch-reconcile loop. The orchestrator polls GitHub for eligible issues, dispatches Copilot CLI agents to per-issue workspaces with skill-based prompts, monitors their lifecycle, retries on failure, and exposes a real-time web dashboard. In-memory state; GitHub is the source of truth.

**Tech Stack:** Go 1.25, chi (routing), go-github (REST), manual GraphQL, fsnotify (config watch), templ + templUI + HTMX (dashboard), Tailwind CSS v4, yaml.v3, log/slog

**Module:** `github.com/bketelsen/gopilot`

---

## Plan Structure

The plan is split into per-phase files for manageability:

| Phase | File | Tasks |
|-------|------|-------|
| 0 | [phase-0-scaffold.md](./phase-0-scaffold.md) | Clean slate setup, domain model, interfaces |
| 1 | [phase-1-core-loop.md](./phase-1-core-loop.md) | Config, GitHub client, workspace, agent, orchestrator, CLI |
| 2 | [phase-2-reliability.md](./phase-2-reliability.md) | Retry queue, stall detection, reconciliation, hot-reload |
| 3 | [phase-3-skills.md](./phase-3-skills.md) | Skill loader, injector, default skills |
| 4 | [phase-4-dashboard.md](./phase-4-dashboard.md) | Web server, pages, SSE, JSON API |
| 5 | [phase-5-analytics.md](./phase-5-analytics.md) | Token tracking, cost estimation, sprint view |
| 6 | [phase-6-multi-agent.md](./phase-6-multi-agent.md) | Claude Code adapter, sub-issues, settings page |

## Execution Order

Phases are strictly sequential. Within each phase, tasks are numbered and should be completed in order (later tasks depend on earlier ones). Each task follows TDD: write failing test → verify failure → implement → verify pass → commit.

## Key References

- **Spec:** `research/SPEC-DRAFT.md`
- **Design:** `docs/plans/2026-03-05-gopilot-rewrite-design.md`
- **Test command:** `go test -race ./...`
- **Build command:** `task build`
- **Lint command:** `task lint`
