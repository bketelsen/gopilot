# Dashboard-Based Interactive Planning

## Problem

The current issue-based planning flow has three key problems:
1. **Latency** - polling-based detection of human replies means long delays between turns
2. **UX friction** - labeling an issue, replying in comments, waiting for the poll cycle feels clunky
3. **No code context** - the planning agent can't explore the actual codebase during the conversation

## Solution

Replace the issue-comment planning flow with an interactive chat UI in the web dashboard. The planning agent runs as a subprocess with full repo access, communicating with the user in real-time via WebSocket.

## Architecture

### Session Lifecycle

New package `internal/planning/` manages planning sessions independently from the orchestrator's poll-dispatch loop.

```
PlanningSession {
    ID            string          // UUID
    Repo          string          // e.g. "owner/repo"
    LinkedIssue   *int            // optional GitHub issue ID
    WorkspaceDir  string          // cloned/reused checkout path
    Agent         agent.Process   // running subprocess
    Status        enum            // active, completing, done, failed
    CreatedAt     time.Time
    Messages      []ChatMessage   // conversation history for reconnect
}
```

**Flow:**
1. HTTP POST `/api/planning/sessions` creates session (repo required, issue optional)
2. Backend calls `workspace.Manager.Create()` to get a checkout
3. Renders initial prompt with repo context + planning skill
4. Launches agent subprocess via existing `agent.Runner`
5. Returns session ID; frontend redirects to `/planning/{id}`

**Session storage:** In-memory map with RWMutex (same pattern as `orchestrator.State`). Messages kept in memory for reconnect. Sessions cleaned up after configurable TTL post-completion.

### WebSocket Layer

**Endpoint:** `GET /api/planning/sessions/{id}/ws` upgrades to WebSocket.

**Protocol — JSON messages in both directions:**

```json
// Client -> Server
{ "type": "message", "content": "How is auth handled in this repo?" }
{ "type": "cancel" }

// Server -> Client
{ "type": "agent", "content": "Looking at the codebase..." }
{ "type": "status", "status": "active" }
{ "type": "error", "content": "Agent process exited unexpectedly" }
{ "type": "done", "plan": { ... } }
```

**Key decisions:**
- Agent stdout forwarded as `agent` messages in real-time (chunked by line) for streaming effect
- Disconnected clients get message history replayed from in-memory buffer on reconnect
- Library: `nhooyr.io/websocket` (cleaner API, better context support than gorilla)
- Existing SSE hub stays for the main dashboard; planning uses WebSocket for bidirectional communication

**Integration:** New routes registered alongside existing chi routes in `internal/web/server.go`. Planning session manager injected via interface (same pattern as `StateProvider`, `MetricsProvider`).

### Chat UI

**New pages:**
- `/planning/{id}` — chat page for an active session
- "New Planning Session" button on main dashboard

**Entry point:**
- Button opens a form: pick repo (dropdown from configured repos), optionally paste a GitHub issue URL
- On submit, POST creates session, redirects to `/planning/{id}`

**Chat page layout:**
- Chat thread with messages rendered as markdown; agent and user messages visually distinct
- Input box at bottom with send button
- Status indicator: "Agent is thinking..." / "Waiting for your input"
- Structured plan renders as a formatted card with editable checkboxes

**Tech approach:**
- templ components for page shell and message rendering
- Vanilla JS + WebSocket for real-time communication (consistent with existing HTMX approach)
- Messages appended to DOM as they arrive; auto-scroll
- Markdown rendering: server-side via templ for history, client-side for live messages

**Planning sessions list:** Replace/augment the current "Planning Sessions" table on the dashboard. Each row links to `/planning/{id}`. Shows: repo, linked issue, status, message count, created time.

### Agent Prompt & Skill Adaptation

The planning skill is adapted for dashboard use:
- Remove GitHub-comment-specific instructions (`gh issue comment`, marker comments)
- Agent writes to stdout instead of posting comments
- Keep the conversational structure: freeform exploration that converges to a structured plan
- Agent is instructed to proactively explore the codebase and cite specific files/functions

**Prompt template:**
```
You are a planning assistant for the {{repo}} repository.
You have full access to the codebase in your working directory.

{{if .LinkedIssue}}
This planning session is about GitHub issue #{{.IssueID}}:
Title: {{.IssueTitle}}
Body: {{.IssueBody}}
{{else}}
The user wants to plan a new feature or change. Ask them what they'd like to build.
{{end}}

{{.PlanningSkill}}

When you have enough context, propose a structured plan in this format:
## Plan: <title>
### Phase 1: <name>
- [ ] Task description (complexity: S/M/L)
  Dependencies: none
- [ ] Another task (complexity: M)
  Dependencies: Task 1
### Phase 2: ...
```

### Output Generation

When the agent proposes a plan and the user approves, the user picks output format.

**UI:** Below the plan card, three action buttons: "Create GitHub Issues", "Save Plan Doc", "Both". User can uncheck items to exclude before clicking.

**Create GitHub Issues:**
- Backend parses structured plan format from agent's message
- Creates issues via `github.Client.CreateIssue()`
- If parent issue linked, wires up sub-issues via `AddSubIssue()`
- Adds dependency references ("blocked by #N") in bodies
- Labels with `gopilot` label to enter the coding work queue
- Returns created issue URLs to chat

**Save Plan Doc:**
- Writes `docs/plans/YYYY-MM-DD-<topic>-plan.md` to workspace checkout
- Commits to a branch
- Optionally opens a PR

**Both:** Does both.

Plan parsing happens server-side, not in the agent. The agent outputs a known markdown format; gopilot parses it.

### Existing Issue-Based Planning: Redirect

The `gopilot:plan` label is kept as a discovery mechanism. When the orchestrator detects a planning-labeled issue, it posts a single comment:

> Planning sessions are now interactive. Start one at: [dashboard URL]/planning/new?repo={repo}&issue={id}

The existing comment-based planning code (`orchestrator/planning.go`, comment detection, `prompt.RenderPlanning()`) is removed.

## What's NOT in scope

- **Database persistence** — sessions are in-memory only; resumability is nice-to-have, deferred
- **Multi-user** — single user assumed (matches current gopilot model)
- **Agent selection UI** — uses configured default agent; no per-session override
