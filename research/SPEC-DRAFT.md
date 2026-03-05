# Gopilot Specification — DRAFT

Version: 0.1.0-draft
Date: 2026-03-05
Status: Draft for review

---

## 1. Problem Statement

Software teams use GitHub Issues to track work, but turning an issue into a shipped pull request still requires a human to: read the issue, understand the codebase, create a branch, write code, write tests, open a PR, respond to review feedback, and merge. Each step is interruptible and context-dependent.

AI coding agents (Claude Code, Codex, Copilot CLI) can perform most of these steps autonomously for well-scoped issues — but they need orchestration: something to decide *which* issues to work on, *when* to start, *how* to set up the workspace, *what behavioral contracts* to enforce, and *when to stop*.

**Gopilot** is a long-running orchestrator that bridges GitHub issue tracking and AI coding agents. It watches GitHub for eligible issues, dispatches agents to isolated workspaces with disciplined workflow contracts, manages retries and concurrency, and provides a web dashboard for visibility.

---

## 2. Goals

1. **GitHub-native**: Use GitHub Issues, Projects v2, sub-issues, labels, and PRs as the single source of truth. No external issue tracker.
2. **Copilot-first**: Support GitHub Copilot CLI as the primary agent, with an adapter interface for future agents (Claude Code, Codex, etc.).
3. **Disciplined execution**: Enforce superpowers-style behavioral contracts (TDD, code review, verification) so agents cannot skip steps.
4. **Autonomous but supervised**: Agents work independently, but humans retain control via issue state, labels, and the dashboard. The orchestrator never writes to GitHub — the agent does.
5. **Single binary**: Ship as one Go binary with embedded web UI. No Node.js runtime, no external databases.
6. **Economically viable**: Track token usage per issue, per sprint, per project. Make cost visible.

---

## 3. System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        GOPILOT                              │
│                                                             │
│  ┌──────────┐  ┌──────────────┐  ┌───────────────────────┐  │
│  │ Workflow  │  │   GitHub     │  │    Orchestrator       │  │
│  │ Loader   │──│   Tracker    │──│  (poll/dispatch/       │  │
│  │          │  │   Client     │  │   reconcile loop)     │  │
│  └──────────┘  └──────────────┘  └───────────┬───────────┘  │
│                                              │              │
│  ┌──────────┐  ┌──────────────┐  ┌───────────┴───────────┐  │
│  │ Skill    │  │  Workspace   │  │    Agent Runner       │  │
│  │ Loader   │──│  Manager     │──│  (Copilot CLI /       │  │
│  │          │  │              │  │   future adapters)    │  │
│  └──────────┘  └──────────────┘  └───────────────────────┘  │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Web Dashboard (templUI + HTMX)          │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Structured Logging                      │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
         │                              │
         ▼                              ▼
   GitHub API                    Agent subprocess
   (read issues,                 (Copilot CLI in
    Projects v2)                  workspace dir)
                                       │
                                       ▼
                                 GitHub API
                                 (agent writes:
                                  branches, PRs,
                                  comments, state)
```

### Components

| # | Component | Purpose |
|---|-----------|---------|
| 1 | **Workflow Loader** | Reads `gopilot.yaml`, watches for changes, hot-reloads config and prompt templates |
| 2 | **GitHub Tracker Client** | Polls GitHub Issues + Projects v2 for candidate issues, refreshes states for reconciliation |
| 3 | **Orchestrator** | Single-authority state machine: poll → dispatch → reconcile loop with retry queue |
| 4 | **Skill Loader** | Discovers and loads superpowers-style behavioral contract Markdown files |
| 5 | **Workspace Manager** | Creates/manages per-issue git worktrees with lifecycle hooks |
| 6 | **Agent Runner** | Launches Copilot CLI (or other agent) subprocess in workspace, streams events, enforces timeouts |
| 7 | **Web Dashboard** | Real-time status UI built with Go + templ + templUI + HTMX |
| 8 | **Structured Logger** | Issue-scoped, session-scoped structured logging |

---

## 4. Domain Model

### 4.1 Issue (normalized from GitHub)

```go
type Issue struct {
    // Identity
    ID         int       // GitHub issue number
    NodeID     string    // GitHub GraphQL node ID
    Repo       string    // "owner/repo"
    URL        string    // Full GitHub URL

    // Content
    Title       string
    Body        string
    Labels      []string  // lowercase
    Assignees   []string

    // Hierarchy
    ParentID    *int      // Parent issue number (sub-issues)
    ChildIDs    []int     // Child issue numbers
    BlockedBy   []int     // Issues blocking this one

    // Project fields (from Projects v2)
    Status      string    // Custom field: Todo, In Progress, In Review, Done
    Priority    int       // Custom field: 0=none, 1=urgent, 2=high, 3=medium, 4=low
    Iteration   string    // Sprint/iteration name
    Effort      int       // Story points / effort estimate

    // Timestamps
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 4.2 Issue State Categories

Issues are categorized by their Projects v2 Status field (not GitHub open/closed state alone):

| Category | Status Values | Behavior |
|----------|---------------|----------|
| **Eligible** | `Todo` | Can be dispatched (if not blocked) |
| **Active** | `In Progress` | Currently being worked on by an agent |
| **Review** | `In Review` | Agent completed, awaiting human review |
| **Terminal** | `Done`, `Closed`, `Canceled` | Work complete or abandoned |

The orchestrator transitions Status from `Todo` → `In Progress` when dispatching. All other transitions are made by the agent or by humans.

### 4.3 Workflow Definition (`gopilot.yaml`)

```yaml
# gopilot.yaml — project configuration

github:
  token: $GITHUB_TOKEN              # or $GH_TOKEN, supports env var resolution
  repos:                            # repositories to watch
    - owner/repo-a
    - owner/repo-b
  project:                          # GitHub Projects v2
    owner: "@me"                    # or org name
    number: 1                       # project number
  eligible_labels:                  # issues must have at least one
    - gopilot
    - autopilot
  excluded_labels:                  # never dispatch these
    - blocked
    - needs-design
    - wontfix

polling:
  interval_ms: 30000               # how often to check for new issues
  max_concurrent_agents: 3         # concurrency limit

workspace:
  root: /var/gopilot/workspaces    # where per-issue worktrees live
  hooks:
    after_create: |
      git clone --branch main https://x-access-token:${GITHUB_TOKEN}@github.com/{{repo}}.git .
    before_run: |
      git fetch origin
      git checkout -B gopilot/issue-{{issue_id}} origin/main
    after_run: ""
    before_remove: ""

agent:
  command: "copilot"               # GitHub Copilot CLI (primary)
  model: "claude-sonnet-4.6"      # model for agent to use
  max_autopilot_continues: 20     # max continuation steps per session
  turn_timeout_ms: 1800000        # 30 min per turn
  stall_timeout_ms: 300000        # 5 min inactivity
  max_retry_backoff_ms: 300000    # max 5 min between retries
  max_retries: 3                  # give up after N failures

skills:
  dir: ./skills                    # path to skill definitions
  required:                        # skills injected into every agent session
    - tdd
    - verification
  optional:                        # loaded when relevant
    - debugging
    - code-review

prompt: |
  You are an AI software engineer working on a GitHub issue.

  ## Issue
  - Repository: {{ .Issue.Repo }}
  - Issue: #{{ .Issue.ID }} — {{ .Issue.Title }}
  - Labels: {{ .Issue.Labels | join ", " }}
  - Priority: {{ .Issue.Priority }}

  ## Description
  {{ .Issue.Body }}

  {% if .Issue.ParentID %}
  ## Parent Issue
  This is a sub-task of #{{ .Issue.ParentID }}. Focus only on this sub-task.
  {% endif %}

  {% if .Attempt > 1 %}
  ## Retry
  This is attempt {{ .Attempt }}. Previous attempt failed or timed out.
  Review any existing work in this workspace before starting fresh.
  {% endif %}

  ## Your Workflow
  1. Read and understand the issue requirements
  2. Explore the codebase to understand relevant code
  3. Write failing tests that verify the requirements (TDD red)
  4. Implement the minimum code to pass the tests (TDD green)
  5. Refactor if needed (TDD refactor)
  6. Run the full test suite — all tests must pass
  7. Create a branch, commit your changes, and open a pull request
  8. Add a comment to the issue summarizing what you did
  9. Move the issue status to "In Review" in the GitHub Project

  ## Rules
  - NEVER push directly to main
  - ALWAYS write tests before implementation
  - ALWAYS run the full test suite before opening a PR
  - NEVER mark work as done without evidence (test output, build output)
  - If you are stuck or the issue is unclear, add a comment asking for clarification and stop

  ## Tools Available
  - Built-in GitHub MCP server for issue/PR operations
  - `gh` CLI for GitHub operations not covered by MCP
  - Standard development tools in the workspace
```

### 4.4 Orchestrator Runtime State

```go
type OrchestratorState struct {
    // Active work
    Running       map[int]*RunEntry    // issue ID → run info
    Claimed       map[int]bool         // issue IDs claimed for dispatch
    RetryQueue    map[int]*RetryEntry  // issue ID → retry info

    // Bookkeeping
    Completed     map[int]bool         // issues that completed at least once
    TotalTokens   TokenTotals          // aggregate token usage
    StartedAt     time.Time
}

type RunEntry struct {
    Issue         Issue
    SessionID     string
    ProcessPID    int
    StartedAt     time.Time
    LastEventAt   time.Time
    LastEvent     string
    LastMessage   string
    TurnCount     int
    Attempt       int
    Tokens        TokenCounts
}

type RetryEntry struct {
    IssueID       int
    Identifier    string      // "owner/repo#42"
    Attempt       int
    DueAt         time.Time
    Error         string
    DelayType     string      // "continuation" or "backoff"
}

type TokenCounts struct {
    InputTokens   int64
    OutputTokens  int64
    TotalTokens   int64
}

type TokenTotals struct {
    TokenCounts
    SecondsRunning float64
    CostEstimate   float64    // estimated USD cost
}
```

### 4.5 Skill Definition

Skills follow the superpowers pattern — Markdown files with YAML frontmatter:

```yaml
---
name: tdd
description: Use when implementing any code change, feature, or bug fix
type: rigid
---
```

Skill types:
- **rigid**: Must be followed exactly. Includes iron laws, red flags, and anti-rationalization tables.
- **flexible**: Principles to adapt to the situation.
- **technique**: Concrete steps for a specific task.

---

## 5. Orchestrator Behavior

### 5.1 Startup

```
1. Load and validate gopilot.yaml
2. Discover and load skills from skills directory
3. Validate GitHub credentials and connectivity
4. Validate agent command exists
5. Clean up workspaces for terminal issues (from a previous run)
6. Start web dashboard server
7. Begin poll-dispatch-reconcile loop
```

### 5.2 Poll-Dispatch-Reconcile Tick

```
on_tick():
  1. RECONCILE
     - For each running agent:
       a. Check issue state in GitHub (API call)
       b. If terminal → kill agent, cleanup workspace
       c. If no longer eligible → kill agent (keep workspace)
       d. If still active → update local state
     - For each running agent:
       a. Check time since last event
       b. If stalled > stall_timeout_ms → kill agent, schedule retry

  2. PROCESS RETRY QUEUE
     - For each retry entry where due_at <= now:
       a. Re-fetch issue from GitHub
       b. If issue still eligible and slots available → dispatch
       c. If issue no longer eligible → release claim
       d. If no slots → requeue with incremented backoff

  3. FETCH CANDIDATES
     - Query GitHub Issues across configured repos
     - Filter: has eligible_label, no excluded_label, status=Todo
     - Filter: not blocked (all BlockedBy issues are terminal)
     - Filter: not already running or claimed
     - Normalize and enrich with Projects v2 field data

  4. SORT
     - Primary: Priority (1=urgent first, 4=low last, 0=none last)
     - Secondary: oldest created_at first

  5. DISPATCH
     - For each candidate, if slots available:
       a. Set issue Status to "In Progress" via Projects v2 API
       b. Create/reuse workspace
       c. Run before_run hook
       d. Build prompt from template + issue + skills + attempt
       e. Launch agent subprocess in workspace
       f. Record in running map

  6. NOTIFY
     - Push state update to dashboard via SSE

  7. SCHEDULE next tick after poll_interval_ms
```

### 5.3 Dispatch: Single Issue

```
dispatch_issue(issue, attempt):
  1. workspace = workspace_manager.ensure(issue)
     - Path: {root}/{repo_name}/issue-{id}
     - If new: run after_create hook (clone repo)
     - Run before_run hook (fetch, create branch)

  2. prompt = render_prompt(template, issue, attempt, skills)
     - Inject required skills into prompt
     - Render Go template with issue data

  3. process = agent_runner.start(workspace, prompt, opts)
     - Launch via configured adapter (Copilot CLI by default)
     - Stream events to orchestrator

  4. Update orchestrator state:
     - running[issue.ID] = new RunEntry
     - claimed[issue.ID] = true

  5. On process exit:
     - normal → schedule continuation (attempt=1, delay=10s)
     - error  → schedule retry (attempt+1, backoff)
     - If attempt > max_retries → release, set status back to Todo,
       add comment explaining failure
```

### 5.4 State Machine

```
                        dispatch
            ┌────────────────────────────┐
            │                            ▼
         ┌──┴──┐    claim      ┌────────────────┐
         │ Todo │──────────────│  In Progress    │
         └──┬──┘               │  (agent runs)  │
            ▲                  └───────┬────────┘
            │                     │         │
            │              normal exit   error/timeout
            │                     │         │
            │                     ▼         ▼
            │              ┌──────────────────┐
            │              │   Retry Queue    │
            │              └────────┬─────────┘
            │                       │
            │            ┌──────────┴──────────┐
            │            │                     │
            │       retries left         max retries
            │            │                     │
            │            ▼                     ▼
            │       re-dispatch         release + comment
            │            │                     │
            └────────────┘                     │
                                              ▼
         ┌──────────┐                  ┌────────────┐
         │In Review │◄─── agent ──────│  (failed)  │
         └────┬─────┘    opens PR      └────────────┘
              │
         human review
              │
              ▼
         ┌──────────┐
         │   Done   │
         └──────────┘
```

### 5.5 Reconciliation Details

**Issue moved to terminal state externally** (human closed it, or marked Done):
- Kill the agent process (SIGTERM, then SIGKILL after 10s)
- Run after_run hook (best-effort)
- Remove workspace (run before_remove hook first)
- Release claim

**Issue moved to non-eligible state** (e.g., removed gopilot label, or status changed to something unexpected):
- Kill the agent process
- Run after_run hook (best-effort)
- Keep workspace (may be re-dispatched later)
- Release claim

**Agent stalled** (no events for stall_timeout_ms):
- Kill the agent process
- Schedule retry with backoff
- Add comment: "Agent stalled after {duration}, retrying"

---

## 6. GitHub Tracker Client

### 6.1 Required Operations

| Operation | GitHub API | Purpose |
|-----------|-----------|---------|
| `FetchCandidateIssues()` | REST: `GET /repos/{owner}/{repo}/issues` + GraphQL for Projects v2 fields | Get eligible issues |
| `FetchIssueState(id)` | REST: `GET /repos/{owner}/{repo}/issues/{id}` | Reconciliation |
| `FetchIssueStates(ids)` | GraphQL batch query | Bulk reconciliation |
| `SetProjectField(issue, field, value)` | GraphQL mutation: `updateProjectV2ItemFieldValue` | Set Status to "In Progress" |
| `AddComment(issue, body)` | REST: `POST /repos/{owner}/{repo}/issues/{id}/comments` | Agent failure notifications |

### 6.2 Issue Eligibility Rules

An issue is eligible for dispatch when ALL of the following are true:

1. Issue is `open` in GitHub
2. Issue has at least one `eligible_label`
3. Issue has no `excluded_label`
4. Issue's Projects v2 Status is `Todo`
5. All issues in `BlockedBy` are in terminal state
6. Issue is not already running or claimed
7. Issue is not in the retry queue

### 6.3 Projects v2 Integration

Gopilot uses GitHub Projects v2 as the status/priority/sprint layer:

| Custom Field | Type | Values | Purpose |
|-------------|------|--------|---------|
| **Status** | Single Select | Todo, In Progress, In Review, Done, Canceled | Workflow state |
| **Priority** | Single Select | Urgent, High, Medium, Low | Dispatch ordering |
| **Iteration** | Iteration | Sprint names | Sprint planning |
| **Effort** | Number | Story points | Capacity planning |
| **Agent** | Text | Agent session ID | Traceability |
| **Cost** | Number | Estimated USD | Cost tracking |

### 6.4 Normalization

- Labels → lowercase
- Priority → integer (Urgent=1, High=2, Medium=3, Low=4, none=0)
- Blockers → derived from sub-issue parent/child relationships and "blocked by #N" in body
- Timestamps → `time.Time` from ISO-8601

### 6.5 Authentication

- `$GITHUB_TOKEN` or `$GH_TOKEN` environment variable
- Must have scopes: `repo`, `project`, `read:org` (if using org projects)
- Agent subprocess inherits the token for `gh` CLI operations

---

## 7. Workspace Manager

### 7.1 Layout

```
{workspace.root}/
├── {repo-name}/
│   ├── issue-42/          # per-issue git worktree
│   │   ├── .git           # worktree link
│   │   ├── src/           # repo contents
│   │   └── ...
│   ├── issue-87/
│   └── ...
└── {other-repo}/
    └── ...
```

### 7.2 Lifecycle

```
ensure(issue) →
  path = {root}/{repo_name}/issue-{issue_id}

  IF path does not exist:
    mkdir -p path
    run after_create hook in path  (clone repo)
    RETURN path

  IF path exists:
    RETURN path  (reuse existing workspace)

cleanup(issue) →
  path = {root}/{repo_name}/issue-{issue_id}
  run before_remove hook in path  (best-effort)
  rm -rf path
```

### 7.3 Hooks

Hooks are shell commands from `gopilot.yaml`, executed in the workspace directory.

| Hook | When | Failure Behavior |
|------|------|-----------------|
| `after_create` | Once, when workspace first created | Fatal — fail the dispatch attempt |
| `before_run` | Before each agent session | Fatal — fail the dispatch attempt |
| `after_run` | After each agent session | Best-effort — log and continue |
| `before_remove` | Before workspace deletion | Best-effort — log and continue |

Template variables available in hooks: `{{repo}}`, `{{issue_id}}`, `{{branch}}`.

Hook timeout: 60 seconds (configurable via `workspace.hook_timeout_ms`).

### 7.4 Safety

- Workspace paths must be under `workspace.root` — reject traversal attempts
- Sanitize issue identifiers in directory names
- Agent CWD is locked to the workspace directory
- Workspaces persist across retries (preserves git history, build artifacts)
- Cleanup only on terminal state or explicit operator action

---

## 8. Agent Runner

### 8.1 Agent Runner Interface

The runner is designed as an adapter interface from day one:

```go
type AgentRunner interface {
    // Start launches an agent subprocess in the given workspace with the prompt.
    // Returns a Session that can be used to monitor and stop the agent.
    Start(ctx context.Context, workspace string, prompt string, opts AgentOpts) (*Session, error)

    // Stop terminates a running agent session (SIGTERM → wait → SIGKILL).
    Stop(session *Session) error

    // Name returns the adapter name for logging/display.
    Name() string
}

type AgentOpts struct {
    Model             string   // model to use (adapter-specific)
    MaxContinuations  int      // max autonomous continuation steps
    Env               []string // additional environment variables
}
```

### 8.2 Copilot CLI Adapter (Primary)

The primary agent adapter uses GitHub Copilot CLI:

```
Command:
  copilot -p <prompt> \
    --allow-all \
    --no-ask-user \
    --autopilot \
    --max-autopilot-continues {max_autopilot_continues} \
    --model {model} \
    --share {workspace}/.gopilot-session.md \
    -s

Working directory: workspace path
Environment:
  GITHUB_TOKEN={token}
  COPILOT_GITHUB_TOKEN={token}
  GH_TOKEN={token}
  PATH includes: gh, git, standard dev tools
```

Key flags:
- `-p` — non-interactive mode: execute prompt and exit
- `--allow-all` — auto-approve all tool invocations, file access, and URLs
- `--no-ask-user` — agent never pauses to ask questions (avoids stalls)
- `--autopilot` — multi-step autonomous continuation
- `--max-autopilot-continues` — safety cap on continuation steps
- `--share` — save session transcript for audit/debugging
- `-s` — silent mode (clean output for parsing)

Copilot CLI is a full coding agent with built-in tools for file read/write, shell execution, codebase search (bundled ripgrep), URL fetch, and a built-in GitHub MCP server for issue/PR operations.

### 8.3 Claude Code Adapter

Secondary adapter for Claude Code CLI:

```
Command:
  claude --dangerously-skip-permissions \
    --print .gopilot-prompt.md

Working directory: workspace path
Environment:
  GITHUB_TOKEN={token}
  GH_TOKEN={token}
  PATH includes: gh, git, standard dev tools
```

The `--print` flag runs Claude Code non-interactively with the prompt file. The `--dangerously-skip-permissions` flag allows autonomous file and command execution.

### 8.4 Agent Output Processing

The agent runner captures:
- **stdout**: Agent's work output, tool calls, results
- **stderr**: Diagnostic/error output
- **Exit code**: 0 = success, non-zero = failure
- **Session transcript**: Saved to `{workspace}/.gopilot-session.md` (Copilot `--share` flag)

Events emitted to orchestrator:
- `agent_started` — process launched
- `agent_output` — stdout/stderr line received (updates `last_event_at`)
- `agent_completed` — process exited with code 0
- `agent_failed` — process exited with non-zero code
- `agent_timeout` — turn or stall timeout reached

### 8.5 Timeouts

| Timeout | Config Key | Default | Behavior |
|---------|-----------|---------|----------|
| **Turn** | `agent.turn_timeout_ms` | 30 min | Total time for one agent session |
| **Stall** | `agent.stall_timeout_ms` | 5 min | Time since last stdout/stderr output |

On timeout: SIGTERM → wait 10s → SIGKILL.

### 8.6 Future Agent Adapters

Additional adapters can be added for:
- Codex CLI
- Aider
- OpenCode
- Any CLI tool that accepts a prompt and works in a directory

Each adapter implements the `AgentRunner` interface and translates gopilot's prompt + workspace into the agent's native invocation pattern.

---

## 9. Skill System

### 9.1 Design Philosophy

Adapted from obra/superpowers: skills are structured Markdown documents that define behavioral contracts for AI agents. They are injected into the agent's prompt to enforce disciplined development practices.

The key insight from superpowers: **AI agents WILL try to skip steps.** Every rigid skill must include anti-rationalization defenses.

### 9.2 Skill Structure

```
skills/
├── tdd/
│   └── SKILL.md
├── verification/
│   └── SKILL.md
├── code-review/
│   ├── SKILL.md
│   └── reviewer-prompt.md
├── debugging/
│   ├── SKILL.md
│   └── root-cause-tracing.md
└── pr-workflow/
    └── SKILL.md
```

### 9.3 Required Skills (Shipped with Gopilot)

#### `tdd` — Test-Driven Development
- **Type**: Rigid
- **Iron Law**: Never write implementation before a failing test
- **Workflow**: RED (write failing test) → GREEN (minimal implementation) → REFACTOR
- **Red Flags**: "I'll add tests later", "This is too simple to test", "The tests would just duplicate the implementation"
- **Verification**: Test output showing red→green transition must be captured

#### `verification` — Verification Before Completion
- **Type**: Rigid
- **Iron Law**: Never claim work is done without evidence
- **Requirements**: Full test suite passing, build succeeds, linter clean
- **Red Flags**: "It should work", "I tested it manually", "The change is trivial"
- **Verification**: Captured command output proving all checks pass

#### `pr-workflow` — Pull Request Workflow
- **Type**: Rigid
- **Iron Law**: Never push to main, never merge your own PR
- **Workflow**: Create branch → commit → push → open PR → add issue comment → set status to In Review
- **Requirements**: PR description references issue, includes test plan, has passing CI status
- **Red Flags**: "I'll just push to main since it's a small change"

#### `debugging` — Systematic Debugging
- **Type**: Technique
- **Workflow**: Reproduce → Isolate → Root cause → Fix → Verify fix → Verify no regression
- **Requirements**: Root cause identified before any fix attempted

#### `code-review` — Code Review (for multi-agent setups)
- **Type**: Flexible
- **Used when**: A separate review agent is dispatched to check implementation
- **Reviewer prompt**: Adversarial — assume the implementer cut corners

### 9.4 Custom Skills

Users can add project-specific skills to the skills directory:

```yaml
---
name: database-migrations
description: Use when the issue involves database schema changes
type: rigid
---

## Iron Law
Never modify a migration file that has already been applied to production.

## Workflow
1. Create a new migration file with `make migration name=<descriptive_name>`
2. Write the UP migration
3. Write the DOWN migration
4. Run `make migrate-up` to apply
5. Run `make migrate-down` to verify rollback
6. Run `make migrate-up` again to verify re-apply
7. Run the full test suite

## Red Flags
- "I'll just edit the existing migration"
- "We don't need a down migration for this"
- "I'll test the migration in production"
```

### 9.5 Skill Resolution

1. Load all `SKILL.md` files from `skills.dir` (max depth 3)
2. Parse YAML frontmatter for `name`, `description`, `type`
3. Required skills (from `skills.required`) are always injected into the prompt
4. Optional skills are injected when the skill description matches the issue context
5. All supporting files in the skill directory are available for inclusion

---

## 10. Web Dashboard

### 10.1 Technology Stack

```
Go (net/http or chi)
├── templ          — server-side HTML rendering (.templ → Go)
├── templUI        — pre-built UI components (Sidebar, Table, Card, Badge, etc.)
├── HTMX           — partial page updates, SSE for real-time
├── Tailwind CSS v4 — styling with dark mode
└── Chart.js       — metrics visualization (via templUI Charts)
```

Build chain: `templ generate` + `tailwindcss` CLI + `go build`. No Node.js runtime.

### 10.2 Pages

#### Dashboard (`/`)
- **Active Agents**: Table showing running agent sessions
  - Columns: Issue, Repo, Status, Duration, Turn Count, Last Activity, Tokens
  - Each row: Badge for status (Running/Stalled/Retrying), live-updating via SSE
- **Retry Queue**: Table of issues waiting for retry
  - Columns: Issue, Repo, Attempt, Next Retry, Error
- **Summary Cards**:
  - Total issues processed today
  - Active agents / max slots
  - Total tokens used / estimated cost
  - Success rate (completed / attempted)

#### Issue Detail (`/issues/{repo}/{id}`)
- Issue title, description, labels, priority
- Agent session history (all attempts)
- Per-attempt: duration, turns, tokens, exit status, error
- Live agent output (streaming via SSE when running)
- Workspace path and branch info
- Links to GitHub issue and PR

#### Sprint View (`/sprint`)
- Current iteration from Projects v2
- Issues grouped by status: Todo, In Progress, In Review, Done
- Progress bar (done / total)
- Token cost for the sprint
- Burn-down or throughput chart

#### Settings (`/settings`)
- Current `gopilot.yaml` config (read-only display)
- Loaded skills list
- GitHub connection status
- Agent command validation

### 10.3 Real-Time Updates

Use HTMX SSE extension for live dashboard updates:

```go
// Server: SSE endpoint
func handleSSE(w http.ResponseWriter, r *http.Request) {
    flusher := w.(http.Flusher)
    for event := range orchestrator.Events() {
        // Render templ fragment for the changed component
        fragment := renderFragment(event)
        fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, fragment)
        flusher.Flush()
    }
}
```

```html
<!-- Client: HTMX SSE -->
<div hx-ext="sse" sse-connect="/api/events">
    <div sse-swap="agent-update" hx-swap="innerHTML">
        <!-- Agent table rows update in real-time -->
    </div>
</div>
```

### 10.4 JSON API

In addition to the HTML dashboard, expose a JSON API for programmatic access:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `GET /api/v1/state` | GET | Current orchestrator state (running, retrying, totals) |
| `GET /api/v1/issues/{repo}/{id}` | GET | Issue detail with session history |
| `GET /api/v1/sprint` | GET | Current sprint summary |
| `POST /api/v1/refresh` | POST | Trigger immediate poll+reconcile |
| `GET /api/v1/events` | GET (SSE) | Real-time event stream |

---

## 11. Configuration

### 11.1 Config File

Primary config: `gopilot.yaml` (or path via `--config` flag).

Watched for changes — hot-reload without restart. Invalid reload keeps last-known-good config and logs error.

### 11.2 Environment Variable Resolution

Config values starting with `$` are resolved from environment:
- `$GITHUB_TOKEN` → value of `GITHUB_TOKEN` env var
- `$HOME/workspaces` → expanded

### 11.3 CLI Interface

```
gopilot                           # start with ./gopilot.yaml
gopilot --config /path/to.yaml    # explicit config
gopilot --port 8080               # override dashboard port
gopilot --dry-run                 # validate config, list eligible issues, exit
gopilot version                   # print version
gopilot init                      # create starter gopilot.yaml + skills/
```

### 11.4 Defaults

| Config | Default |
|--------|---------|
| `polling.interval_ms` | 30000 (30s) |
| `polling.max_concurrent_agents` | 3 |
| `agent.max_autopilot_continues` | 20 |
| `agent.turn_timeout_ms` | 1800000 (30 min) |
| `agent.stall_timeout_ms` | 300000 (5 min) |
| `agent.max_retry_backoff_ms` | 300000 (5 min) |
| `agent.max_retries` | 3 |
| `workspace.hook_timeout_ms` | 60000 (60s) |
| Dashboard port | 3000 |

---

## 12. Failure Model

### 12.1 Failure Classes

| Class | Examples | Behavior |
|-------|----------|----------|
| **Config** | Bad YAML, missing token, missing agent binary | Fail startup (or skip dispatch on hot-reload failure) |
| **GitHub API** | Rate limit, network error, auth failure | Skip this tick, retry next tick |
| **Workspace** | Clone failure, hook timeout, disk full | Fail attempt, schedule retry |
| **Agent** | Crash, timeout, stall, non-zero exit | Schedule retry with backoff |
| **Dashboard** | Template error, SSE connection drop | Log, don't crash orchestrator |

### 12.2 Retry Strategy

```
On error/timeout:
  delay = min(10s * 2^attempt, max_retry_backoff_ms)
  schedule retry at now + delay

On normal completion:
  schedule continuation at now + 10s (re-check issue state)

On max_retries exceeded:
  release claim
  set issue Status back to Todo
  add comment: "Gopilot failed after {N} attempts. Last error: {error}"
  add label: "gopilot-failed"
```

### 12.3 Restart Recovery

Orchestrator state is in-memory only. On restart:
1. Clean up workspaces for issues in terminal state
2. Fresh poll of all eligible issues
3. Re-dispatch eligible work
4. No retry timers are restored (by design — simplicity over durability)

### 12.4 Operator Controls

Operators control behavior through GitHub:
- **Stop an agent**: Move issue to terminal state (Done/Canceled) — reconciliation kills the agent
- **Pause an agent**: Remove the `gopilot` label — reconciliation stops the agent (keeps workspace)
- **Retry manually**: Move issue Status back to `Todo` with `gopilot` label
- **Block dispatch**: Add `blocked` label
- **Emergency stop**: Kill the gopilot process (restart will recover cleanly)

---

## 13. Observability

### 13.1 Structured Logging

All logs include context fields:

```json
{
  "level": "info",
  "msg": "agent dispatched",
  "issue_id": 42,
  "issue": "owner/repo#42",
  "session_id": "sess-abc123",
  "workspace": "/var/gopilot/workspaces/repo/issue-42",
  "attempt": 1,
  "ts": "2026-03-05T14:30:00Z"
}
```

### 13.2 Metrics

Track and expose:
- Issues dispatched, completed, failed (counters)
- Agent session duration (histogram)
- Token usage: input, output, total (counters)
- Estimated cost in USD (counter)
- GitHub API calls and rate limit remaining (gauges)
- Active agents / max slots (gauge)
- Retry queue depth (gauge)

### 13.3 Cost Estimation

```
cost_per_issue = (input_tokens * input_price + output_tokens * output_price)

# Default prices (configurable):
# Claude Sonnet: $3/M input, $15/M output
# Claude Opus: $15/M input, $75/M output
```

Cost is tracked per-issue, per-sprint, and in aggregate. Updated after each agent session ends.

---

## 14. Security

### 14.1 Trust Model

Gopilot runs in a **trusted environment**. The operator trusts:
- The repositories being worked on
- The issues being processed (from their own project)
- The agent (Copilot CLI or configured adapter) to operate within the workspace

### 14.2 Boundaries

- Agent subprocess runs as the same OS user as gopilot
- Agent CWD is restricted to the workspace directory
- GitHub token is passed via environment variable (never logged)
- Workspace paths are sanitized and must stay under workspace root
- The orchestrator never executes arbitrary code from issues — only the agent does, within its sandbox

### 14.3 Recommendations for Production

- Run gopilot under a dedicated OS user
- Use a GitHub token with minimal required scopes
- Mount workspace root on a dedicated volume
- Use the agent's built-in sandboxing where available
- Monitor token usage and set cost alerts
- Review agent PRs before merging (the agent opens PRs, humans merge)

---

## 15. Project Structure

```
gopilot/
├── cmd/
│   └── gopilot/
│       └── main.go                 # CLI entry point
├── internal/
│   ├── config/
│   │   ├── config.go               # gopilot.yaml parser + validation
│   │   └── watcher.go              # File watch + hot-reload
│   ├── github/
│   │   ├── client.go               # GitHub REST + GraphQL client
│   │   ├── issues.go               # Issue fetching + normalization
│   │   └── projects.go             # Projects v2 field operations
│   ├── orchestrator/
│   │   ├── orchestrator.go         # Core poll-dispatch-reconcile loop
│   │   ├── state.go                # Runtime state management
│   │   ├── dispatch.go             # Issue dispatch logic
│   │   └── reconcile.go            # Reconciliation + stall detection
│   ├── workspace/
│   │   ├── manager.go              # Workspace creation/cleanup
│   │   └── hooks.go                # Hook execution with timeout
│   ├── agent/
│   │   ├── runner.go               # Agent runner interface
│   │   ├── copilot.go              # Copilot CLI adapter (primary)
│   │   ├── claude.go               # Claude Code adapter
│   │   └── process.go              # Process lifecycle management
│   ├── skills/
│   │   ├── loader.go               # Skill discovery + parsing
│   │   └── injector.go             # Skill injection into prompts
│   └── web/
│       ├── server.go               # HTTP server setup
│       ├── handlers.go             # Route handlers
│       ├── sse.go                   # SSE event streaming
│       └── templates/
│           ├── layouts/
│           │   └── base.templ      # Base layout with sidebar
│           ├── pages/
│           │   ├── dashboard.templ  # Main dashboard
│           │   ├── issue.templ      # Issue detail
│           │   ├── sprint.templ     # Sprint view
│           │   └── settings.templ   # Settings
│           └── components/          # templUI components (installed via CLI)
├── skills/                          # Default skill definitions
│   ├── tdd/
│   │   └── SKILL.md
│   ├── verification/
│   │   └── SKILL.md
│   ├── pr-workflow/
│   │   └── SKILL.md
│   ├── debugging/
│   │   ├── SKILL.md
│   │   └── root-cause-tracing.md
│   └── code-review/
│       ├── SKILL.md
│       └── reviewer-prompt.md
├── gopilot.yaml.example             # Starter config
├── go.mod
├── go.sum
├── Taskfile.yml                     # Build tasks (templ generate, tailwind, go build)
└── README.md
```

---

## 16. Build & Distribution

### 16.1 Build Chain

```bash
# Development
task dev          # templ generate --watch + tailwindcss --watch + go run

# Production
task build        # templ generate + tailwindcss --minify + go build -o gopilot

# The result is a single binary with embedded templates and assets
```

### 16.2 Dependencies

| Dependency | Purpose | Type |
|-----------|---------|------|
| `github.com/a-h/templ` | HTML templating | Go module |
| `github.com/google/go-github/v68` | GitHub API client | Go module |
| `golang.org/x/oauth2` | GitHub auth | Go module |
| `github.com/shurcooL/githubv4` | GitHub GraphQL client | Go module |
| `github.com/fsnotify/fsnotify` | Config file watching | Go module |
| `gopkg.in/yaml.v3` | YAML parsing | Go module |
| `github.com/go-chi/chi/v5` | HTTP router | Go module |
| `log/slog` | Structured logging | Go stdlib |
| Tailwind CSS v4 CLI | CSS compilation | Build-time only |
| templUI | UI components | Copied into project |

Zero runtime dependencies outside the Go binary. Tailwind CSS is build-time only.

### 16.3 Distribution

- Single binary: `gopilot` (Linux amd64/arm64, macOS amd64/arm64)
- `go install github.com/bketelsen/gopilot/cmd/gopilot@latest`
- GitHub Releases with checksums
- Optional: Homebrew tap, Docker image

---

## 17. Phased Implementation Plan

### Phase 1: Core Loop (MVP)
- Config parser (`gopilot.yaml`)
- GitHub client (fetch issues, read Projects v2 fields, set Status)
- Orchestrator (poll-dispatch-reconcile, in-memory state)
- Workspace manager (create, hooks, cleanup)
- Agent runner interface + Copilot CLI adapter (subprocess, timeouts, autopilot)
- Prompt rendering (Go templates with issue data)
- CLI (`gopilot`, `gopilot --dry-run`, `gopilot init`)
- Structured logging to stderr
- **No dashboard, no skills, no retry queue**

### Phase 2: Reliability
- Retry queue with exponential backoff
- Stall detection
- Reconciliation against GitHub state
- Max retries with failure comments
- Hot-reload of `gopilot.yaml`
- Agent failure labeling (`gopilot-failed`)

### Phase 3: Skills
- Skill loader (discover SKILL.md files)
- Skill injection into agent prompts
- Ship default skills: tdd, verification, pr-workflow, debugging
- Custom skill support

### Phase 4: Dashboard
- Web server with templ + templUI
- Dashboard page (active agents, retry queue, summary cards)
- Issue detail page with session history
- SSE for real-time updates
- JSON API (`/api/v1/state`, `/api/v1/issues/{repo}/{id}`)

### Phase 5: Sprint & Analytics
- Sprint view using Projects v2 iterations
- Token usage tracking and cost estimation
- Per-issue, per-sprint, aggregate metrics
- Charts (Chart.js via templUI)
- Cost alerts

### Phase 6: Multi-Agent & Extensions
- Claude Code adapter
- Codex adapter
- Multi-stage review (implementer → reviewer agents)
- Sub-issue hierarchy awareness
- Cross-repo orchestration
- Webhook-driven dispatch (supplement polling)

---

## 18. Design Decisions & Rationale

### Why Copilot CLI as primary agent?
- **GitHub-native**: Copilot CLI has a built-in GitHub MCP server — it can interact with issues, PRs, and repos natively without `gh` CLI workarounds
- **Model-agnostic**: Supports Claude, GPT, and Gemini models via `--model` flag — the agent backend is decoupled from the model provider
- **Automation-ready**: `-p` (non-interactive) + `--autopilot` + `--allow-all` + `--no-ask-user` is purpose-built for subprocess orchestration
- **Session transcripts**: `--share` flag saves full session output for audit/debugging
- **Ecosystem alignment**: gopilot targets GitHub-native workflows; Copilot CLI is the GitHub-native agent

### Why an adapter interface?
- Different agents have different invocation patterns, permission models, and output formats
- Teams may prefer Claude Code, Codex, or Aider for specific workloads
- The adapter interface (`AgentRunner`) isolates gopilot's orchestration logic from agent-specific details
- Copilot CLI ships as the default; Claude Code adapter follows in Phase 6

### Why Go?
- Single binary distribution, no runtime dependencies
- Excellent concurrency (goroutines for agent processes, SSE connections)
- Strong GitHub ecosystem (`go-github`, `githubv4`)
- templ + templUI for server-side web UI without Node.js

### Why `gopilot.yaml` instead of `WORKFLOW.md`?
- YAML is more natural for structured config than Markdown frontmatter
- Go template syntax for the prompt section (consistent with Go ecosystem)
- Clear separation: config (YAML) vs. behavioral contracts (skills as Markdown)
- Symphony uses WORKFLOW.md because its entire contract is one file; gopilot separates concerns

### Why orchestrator-reads, agent-writes?
Adopted from Symphony. The orchestrator polls GitHub for eligible issues (read-only) and dispatches agents. The agent itself creates branches, opens PRs, adds comments, and transitions issue state using `gh` CLI. This keeps the orchestrator simple and stateless regarding GitHub writes.

The one exception: the orchestrator sets Status to "In Progress" when dispatching, to prevent double-dispatch. This is a minimal, safe write.

### Why skills as separate Markdown files?
Adopted from Superpowers. Skills are behavioral contracts, not config. They are:
- Readable and editable by humans
- Version-controlled in git
- Customizable per-project without forking gopilot
- Testable (write a failing test → write the skill → verify agent follows it)

### Why polling instead of webhooks?
Phase 1 uses polling for simplicity (no public endpoint needed, works behind NAT/firewall). Webhook support is planned for Phase 6 as a supplement to reduce latency.

### Why in-memory state?
Adopted from Symphony. Orchestrator state does not survive restarts, by design:
- Simplicity: no database, no state file, no corruption risk
- Recovery is clean: poll GitHub, re-dispatch eligible work
- The source of truth is always GitHub, never gopilot's memory

### Why templUI for the dashboard?
- Pure Go stack (no Node.js build chain)
- shadcn/ui model means we own the component code
- HTMX gives us real-time updates without a SPA framework
- 42 components cover all dashboard needs (sidebar, table, card, badge, charts, etc.)
- Dark mode built-in

---

## Appendix A: Inspiration & Prior Art

| System | What Gopilot Adopts | What Gopilot Does Differently |
|--------|--------------------|-----------------------------|
| **Symphony** (OpenAI) | Poll-dispatch-reconcile loop, per-issue workspaces, lifecycle hooks, state machine, WORKFLOW.md pattern, orchestrator-reads/agent-writes boundary, retry with backoff, stall detection | GitHub instead of Linear, Copilot CLI instead of Codex, Go instead of Elixir, skills system, web dashboard, sprint/project integration |
| **Superpowers** (obra) | Skill-as-behavioral-contract, anti-rationalization patterns, TDD enforcement, verification-before-completion, subagent orchestration model | Runtime orchestrator (not just prompts), GitHub integration, persistent daemon |
| **GitHub Projects v2** | Status/Priority/Iteration custom fields, board/table/roadmap views, sub-issues for hierarchy | Used as the project management layer (gopilot adds the automation) |
| **templUI** | Component library for Go web UI | Used for the dashboard (gopilot adds SSE real-time updates) |
