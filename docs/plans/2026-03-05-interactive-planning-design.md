# Interactive Planning from GitHub Issues

**Date:** 2026-03-05
**Status:** Approved

## Overview

Add interactive planning capability to gopilot. A GitHub issue labeled `gopilot:plan` triggers a planning agent that conducts a structured conversation in the issue comments — asking clarifying questions one at a time, proposing a structured plan with checkboxes, and creating child issues with dependencies when the human approves.

## Key Decisions

- **Users:** Both human-initiated (label an issue) and auto-triggered (orchestrator detects complex issues)
- **Primary interface:** GitHub issue comments (CLI and dashboard deferred)
- **Conversation model:** Synchronous, one question at a time via issue comments
- **Approval:** Agent posts a structured plan with checkboxes; human edits checkboxes and comments `/approve`
- **Dependencies:** Sub-issues for hierarchy, `blocked by #N` for ordering, `phase: N` labels for grouping
- **Agent model:** Orchestrator-managed lifecycle, stateless agent invoked per-comment with full thread context
- **Interruption handling:** Agent re-reads thread and picks up context (no exact session replay)
- **Granularity:** Agent decides based on complexity, explains rationale
- **Planning model:** Configurable separately from coding model

## Architecture: Hybrid Orchestrator-Managed, Agent-Driven

The orchestrator owns the lifecycle (detecting planning issues, tracking state, invoking per-comment). All planning logic lives in a **planning skill** (markdown) that guides agent behavior. A new `PlanningDispatcher` component sits alongside existing coding dispatch.

## Planning Issue Lifecycle

```
[Detected] -> [Questioning] -> [AwaitingReply] -> [PlanProposed] -> [AwaitingApproval] -> [CreatingIssues] -> [Complete]
                   ^               |                                        |
                   +---------------+                                        |
                   (human replies)                              (human edits checkboxes,
                                                                 replies /approve)
```

**State transitions:**
- `Detected -> Questioning`: Agent invoked with issue body, posts first question
- `Questioning -> AwaitingReply`: Agent posted a question, waiting for human
- `AwaitingReply -> Questioning`: New human comment detected, agent invoked to ask next question or propose plan
- `Questioning -> PlanProposed`: Agent has enough info, posts structured plan with checkboxes
- `PlanProposed -> AwaitingApproval`: Waiting for human to review/edit checkboxes and `/approve`
- `AwaitingApproval -> CreatingIssues`: Human approved, agent creates issues
- `CreatingIssues -> Complete`: Issues created, label swapped to `gopilot:planned`

**State is reconstructed from the issue thread on restart** — the agent scans comments to determine current phase, making it fully crash-resilient.

## Planning Dispatcher Component

Sits alongside existing coding dispatch in the orchestrator.

**Issue partitioning:** During each tick, after fetching candidates:
- Issues with `gopilot:plan` label -> `PlanningDispatcher`
- All other eligible issues -> existing coding dispatch

**Per-tick behavior:**
1. For each planning issue, check for new comments since last processed comment ID
2. If new human comment found -> invoke agent with full thread context
3. If agent already running for this issue -> skip
4. If no new comments -> do nothing

**Agent invocation:** Fresh call each time. Agent receives: issue body, full comment thread, current phase, planning skill.

**Concurrency:** Planning agents share the `MaxConcurrentAgents` pool. Invocations are short (one question/response).

**Configuration:**

```yaml
planning:
  label: "gopilot:plan"
  completed_label: "gopilot:planned"
  approve_command: "/approve"
  max_questions: 10
  agent: "claude-code"
  model: "claude-sonnet-4-6"
```

## Planning Skill

A new `skills/planning/SKILL.md` injected into the agent prompt during planning invocations.

**Responsibilities:**
- Guide agent through phases: understand idea -> ask questions -> propose plan -> handle approval
- Enforce one-question-at-a-time style
- Define structured plan format
- Instruct agent on phase detection from thread
- Guide issue creation on approval

**Plan output format:**

```markdown
## Proposed Plan: [Feature Name]

### Phase 1: [Foundation]
- [ ] **Issue title** -- Description. _Complexity: small/medium/large_
- [ ] **Another issue** -- Description. `blocked by` the above. _Complexity: small_

### Phase 2: [Core Implementation]
- [ ] **Issue title** -- Description. _Complexity: medium_

### Dependencies
- Phase 2 issues are blocked by all Phase 1 issues
- [specific cross-phase dependencies]

### Notes
- [Rationale for granularity decisions]
- [Assumptions made]

Reply `/approve` to create these issues, or edit the checkboxes and comment with changes.
```

**Phase detection:** Agent scans comments to determine phase:
- No agent comments -> Detected, post introduction + first question
- Agent asked question, human replied -> Questioning, ask next or propose plan
- Plan comment exists with checkboxes -> PlanProposed, wait for edits/approval
- Human commented `/approve` -> create issues

## GitHub Client Extensions

**New REST methods:**

```go
CreateIssue(repo, title, body string, labels []string) (*Issue, error)
AddSubIssue(repo string, parentID, childID int) error
FetchIssueComments(repo string, issueID int) ([]Comment, error)
RemoveLabel(repo string, issueID int, label string) error
```

**New domain type:**

```go
type Comment struct {
    ID        int
    Author    string
    Body      string
    CreatedAt time.Time
}
```

**New GraphQL methods:**

```go
AddProjectItem(repo string, issueID int) error
SetProjectField(repo string, issueID int, fieldName, value string) error
```

**Existing methods reused:** `AddComment`, `AddLabel`, `SetProjectStatus`

## Issue Creation Flow

On `/approve`:

1. **Parse the approved plan** -- extract checked items only (`- [x]`), with title, description, phase, complexity, dependencies
2. **Create issues bottom-up** -- Phase 1 first (no deps), capture real issue numbers, then Phase 2+ with `blocked by #N` referencing actual numbers
3. **Wire up structure** -- `AddSubIssue` for parent/child, `AddProjectItem` for board, set Status -> "Todo" and phase labels, apply eligible label so orchestrator picks them up
4. **Finalize** -- post summary comment with links, remove `gopilot:plan`, add `gopilot:planned`

**Error handling:** Partial failure posts a comment listing what was created and what failed. No automatic retry. Agent checks for existing sub-issues to avoid duplicates.

## Dashboard & Observability

**Dashboard:** Planning issues appear with distinct "Planning" badge. Conversation state visible (phase, question count, waiting-for-human indicator).

**SSE events:** `planning:question_posted`, `planning:reply_detected`, `planning:plan_proposed`, `planning:approved`, `planning:complete`

**Metrics:** Active planning conversations, average questions per plan, token usage (reuses existing tracking).

## Future Extension Points (Deferred)

- **CLI planning** (`gopilot plan`): Same skill, conversation in terminal. Same issue creation flow.
- **Dashboard planning:** Chat UI, same skill, WebSocket/SSE for real-time.
- **Auto-triggered planning:** Orchestrator detects complex issues, auto-labels `gopilot:plan`.
- **Template plans:** Pre-defined templates for common patterns (new API endpoint, new UI page).

All deferred extensions use the same core abstraction (skill + orchestrator lifecycle + GitHub client mutations).
