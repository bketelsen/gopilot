# Gopilot Gap Closure — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close all gaps between the original 6-phase spec and the current codebase — toolchain, backend wiring, dashboard UI, and metrics.

**Architecture:** Foundation-first approach. Set up the templ/HTMX/Tailwind build chain, then wire backend gaps (multi-agent, blocking, GraphQL enrichment), then build all dashboard pages, then fill metrics gaps.

**Tech Stack:** Go 1.26, templ (via `go tool`), templUI, HTMX, Tailwind CSS v4, chi, SSE

---

## Plan Structure

| Section | File | Tasks |
|---------|------|-------|
| 1 | [section-1-toolchain.md](./section-1-toolchain.md) | Templ, Tailwind, templUI, base layout, build chain |
| 2 | [section-2-backend-wiring.md](./section-2-backend-wiring.md) | Multi-agent, blocking, GraphQL, retry fixes, CLI |
| 3 | [section-3-dashboard-pages.md](./section-3-dashboard-pages.md) | Dashboard, Issue Detail, Sprint, Settings, SSE, API |
| 4 | [section-4-metrics.md](./section-4-metrics.md) | Session duration, rate limits, dashboard wiring |

## Execution Order

Sections are strictly sequential. Within each section, tasks are numbered and should be completed in order. Each task follows TDD where applicable.

## Key References

- **Design:** `docs/plans/2026-03-05-gap-closure-design.md`
- **Original Plan:** `docs/plans/2026-03-05-gopilot-rewrite-plan.md`
- **Spec:** `research/SPEC-DRAFT.md`
- **Test command:** `go test -race ./...`
- **Build command:** `task build`
