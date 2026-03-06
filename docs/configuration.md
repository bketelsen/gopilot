# Configuration Reference

Gopilot is configured via a `gopilot.yaml` file in the project root. Run `gopilot init` to generate a starter config with sensible defaults.

!!! note "Hot-reload support"
    Gopilot watches `gopilot.yaml` for changes using fsnotify. Updates to **polling**, **concurrency**, **agent**, **skills**, and **prompt** settings take effect immediately without restarting. Changes to **token** and **repos** require a restart.

---

## `github`

GitHub authentication, repository targeting, and issue filtering.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `token` | string | — | GitHub personal access token or app token. Supports `$ENV_VAR` syntax (e.g., `$GITHUB_TOKEN`) to read from environment variables. |
| `repos` | list of strings | — | Repositories to monitor, in `owner/repo` format. |
| `project.owner` | string | — | GitHub Projects v2 owner. Use `"@me"` for the authenticated user. |
| `project.number` | int | — | GitHub Projects v2 project number. |
| `eligible_labels` | list of strings | — | Issues must have at least one of these labels to be picked up by Gopilot. |
| `excluded_labels` | list of strings | — | Issues with any of these labels are skipped, even if they have an eligible label. |

```yaml
github:
  token: $GITHUB_TOKEN
  repos:
    - myorg/backend
    - myorg/frontend
  project:
    owner: "@me"
    number: 1
  eligible_labels:
    - gopilot
  excluded_labels:
    - blocked
    - needs-design
    - wontfix
```

---

## `polling`

Controls how often Gopilot polls for new issues and how many agents can run simultaneously.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `interval_ms` | int | `30000` (30s) | Polling interval in milliseconds. |
| `max_concurrent_agents` | int | `3` | Maximum number of agent subprocesses running at once. |

```yaml
polling:
  interval_ms: 30000
  max_concurrent_agents: 3
```

---

## `workspace`

Workspace directory and lifecycle hook settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `root` | string | — | Base directory where per-issue workspaces are created. |
| `hook_timeout_ms` | int | `60000` (60s) | Maximum time in milliseconds for a single hook to run before it is killed. |

```yaml
workspace:
  root: ./workspaces
  hook_timeout_ms: 60000
```

### `workspace.hooks`

Shell commands executed at specific points in the workspace lifecycle. Each hook runs inside the workspace directory.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `after_create` | string | `""` | Runs after the workspace directory is created. Typically used to clone the repository. |
| `before_run` | string | `""` | Runs before the agent subprocess starts. Typically used to set up a fresh branch. |
| `before_pr_fix` | string | `""` | Runs before a PR fix agent starts. Falls back to `before_run` if empty. Use `{{branch}}` for the PR's head branch. |
| `after_run` | string | `""` | Runs after the agent subprocess completes. |
| `before_remove` | string | `""` | Runs before the workspace directory is removed. |

#### Variable interpolation

Hooks support the following interpolation variables:

| Variable | Description | Example value |
|----------|-------------|---------------|
| `{{repo}}` | Full repository name | `myorg/backend` |
| `{{issue_id}}` | Issue number | `42` |
| `{{branch}}` | Branch name (PR head ref when set, otherwise `gopilot/issue-{id}`) | `gopilot/issue-42` |
| `${GITHUB_TOKEN}` | Environment variable (standard shell expansion) | *(token value)* |

#### Hook recipe: clone and branch

This is the typical pattern for workspace hooks — clone the repo on creation, then create a fresh branch before each agent run:

```yaml
workspace:
  root: ./workspaces
  hook_timeout_ms: 60000
  hooks:
    after_create: |
      git clone --branch main https://x-access-token:${GITHUB_TOKEN}@github.com/{{repo}}.git .
    before_run: |
      git fetch origin
      git checkout -B gopilot/issue-{{issue_id}} origin/main
    after_run: ""
    before_remove: ""
```

#### Hook recipe: PR fix branch checkout

When PR monitoring dispatches a fix agent, `before_pr_fix` checks out the existing PR branch instead of creating a new one:

```yaml
workspace:
  hooks:
    before_pr_fix: |
      git fetch origin
      git checkout {{branch}}
      git pull origin {{branch}}
```

If `before_pr_fix` is not set, the `before_run` hook is used as a fallback.

---

## `agent`

Agent subprocess configuration, timeouts, and retry behavior.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `command` | string | — | Agent CLI to invoke. Supported values: `copilot`, `claude`, `claude-code`. |
| `model` | string | — | Model identifier passed to the agent (e.g., `claude-sonnet-4.6`). |
| `max_autopilot_continues` | int | `20` | Maximum number of autopilot continuation turns the agent may take. |
| `turn_timeout_ms` | int | `1800000` (30min) | Maximum total wall-clock time for a single agent run in milliseconds. |
| `stall_timeout_ms` | int | `300000` (5min) | If the agent emits no events for this many milliseconds, it is killed as stalled. |
| `max_retry_backoff_ms` | int | `300000` (5min) | Upper bound on exponential backoff between retries, in milliseconds. |
| `max_retries` | int | `3` | Maximum retry attempts for a failed agent run. After exhausting retries, the issue is reset with a failure label. |

```yaml
agent:
  command: copilot
  model: claude-sonnet-4.6
  max_autopilot_continues: 20
  turn_timeout_ms: 1800000
  stall_timeout_ms: 300000
  max_retry_backoff_ms: 300000
  max_retries: 3
```

### `agent.overrides`

A list of per-repository or per-label overrides for the agent command. Each entry can match by `repos`, `labels`, or both. The first matching override wins; if none match, the top-level `agent.command` is used.

| Field | Type | Description |
|-------|------|-------------|
| `repos` | list of strings | Repository names (in `owner/repo` format) this override applies to. |
| `labels` | list of strings | Issue labels this override applies to. Matching is case-insensitive. |
| `command` | string | Agent command to use for matching issues. |

```yaml
agent:
  command: copilot
  overrides:
    - repos:
        - myorg/ml-pipeline
        - myorg/data-science
      command: claude-code
    - labels:
        - complex
      command: claude-code
```

---

## `skills`

Skill injection configuration. Skills are SKILL.md files that provide domain-specific instructions to the agent via the rendered prompt.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `dir` | string | — | Directory containing SKILL.md files. |
| `required` | list of strings | — | Skill names always injected into every prompt. |
| `optional` | list of strings | — | Skill names injected based on context (e.g., issue labels). |

For details on authoring skills, see the [Skills documentation](skills.md).

```yaml
skills:
  dir: ./skills
  required:
    - tdd
    - verification
  optional:
    - debugging
    - code-review
```

---

## `dashboard`

Real-time web dashboard settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Whether to start the web dashboard. |
| `addr` | string | `":3000"` | Address to bind the HTTP server to. |

```yaml
dashboard:
  enabled: true
  addr: ":3000"
```

---

## `planning`

Interactive planning mode configuration. When an issue has the planning label, Gopilot enters a conversational planning phase — asking clarifying questions via issue comments before generating a detailed implementation plan.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `label` | string | `"gopilot:plan"` | Label that triggers planning mode on an issue. |
| `completed_label` | string | `"gopilot:planned"` | Label applied to the issue once planning completes. |
| `approve_command` | string | `"/approve"` | Comment text that approves the proposed plan and triggers issue creation. |
| `max_questions` | int | `10` | Maximum number of clarifying questions before Gopilot forces a plan proposal. |
| `agent` | string | — | Optional agent command override for planning (defaults to the top-level `agent.command`). |
| `model` | string | — | Optional model override for planning (defaults to the top-level `agent.model`). |

```yaml
planning:
  label: "gopilot:plan"
  completed_label: "gopilot:planned"
  approve_command: "/approve"
  max_questions: 10
  agent: claude-code
  model: claude-sonnet-4-6
```

---

## `prompt`

A Go `text/template` string that is rendered for each agent run. The rendered output is passed to the agent as its system instructions.

### Template variables

| Variable | Type | Description |
|----------|------|-------------|
| `.Issue.Repo` | string | Repository name in `owner/repo` format. |
| `.Issue.ID` | int | Issue number. |
| `.Issue.Title` | string | Issue title. |
| `.Issue.Labels` | []string | List of labels on the issue. |
| `.Issue.Body` | string | Full issue body (Markdown). |
| `.Issue.Priority` | string | Issue priority (from GitHub Projects). |

### Template functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `joinStrings` | `joinStrings([]string, sep string) string` | Joins a string slice with a separator. |

```yaml
prompt: |
  You are an AI software engineer working on a GitHub issue.

  ## Issue
  - Repository: {{ .Issue.Repo }}
  - Issue: #{{ .Issue.ID }} — {{ .Issue.Title }}
  - Labels: {{ joinStrings .Issue.Labels ", " }}
  - Priority: {{ .Issue.Priority }}

  ## Description
  {{ .Issue.Body }}

  ## Your Workflow
  1. Read and understand the issue requirements
  2. Explore the codebase to understand relevant code
  3. Write failing tests (TDD red)
  4. Implement minimum code to pass (TDD green)
  5. Refactor if needed
  6. Run the full test suite
  7. Create a branch, commit, and open a pull request
  8. Add a comment to the issue summarizing what you did

  ## Rules
  - NEVER push directly to main
  - ALWAYS write tests before implementation
  - ALWAYS run the full test suite before opening a PR
  - If stuck, add a comment asking for clarification and stop
```

---

## Full Example

Below is a complete `gopilot.yaml` with all sections and default values:

```yaml
github:
  token: $GITHUB_TOKEN
  repos:
    - owner/repo
  project:
    owner: "@me"
    number: 1
  eligible_labels:
    - gopilot
  excluded_labels:
    - blocked
    - needs-design
    - wontfix

polling:
  interval_ms: 30000
  max_concurrent_agents: 3

workspace:
  root: ./workspaces
  hook_timeout_ms: 60000
  hooks:
    after_create: |
      git clone --branch main https://x-access-token:${GITHUB_TOKEN}@github.com/{{repo}}.git .
    before_run: |
      git fetch origin
      git checkout -B gopilot/issue-{{issue_id}} origin/main
    after_run: ""
    before_remove: ""

agent:
  command: copilot
  model: claude-sonnet-4.6
  max_autopilot_continues: 20
  turn_timeout_ms: 1800000
  stall_timeout_ms: 300000
  max_retry_backoff_ms: 300000
  max_retries: 3

skills:
  dir: ./skills
  required:
    - tdd
    - verification
  optional:
    - debugging
    - code-review

dashboard:
  enabled: true
  addr: ":3000"

planning:
  label: "gopilot:plan"
  completed_label: "gopilot:planned"
  approve_command: "/approve"
  max_questions: 10

prompt: |
  You are an AI software engineer working on a GitHub issue.

  ## Issue
  - Repository: {{ .Issue.Repo }}
  - Issue: #{{ .Issue.ID }} — {{ .Issue.Title }}
  - Labels: {{ joinStrings .Issue.Labels ", " }}
  - Priority: {{ .Issue.Priority }}

  ## Description
  {{ .Issue.Body }}

  ## Your Workflow
  1. Read and understand the issue requirements
  2. Explore the codebase to understand relevant code
  3. Write failing tests (TDD red)
  4. Implement minimum code to pass (TDD green)
  5. Refactor if needed
  6. Run the full test suite
  7. Create a branch, commit, and open a pull request
  8. Add a comment to the issue summarizing what you did

  ## Rules
  - NEVER push directly to main
  - ALWAYS write tests before implementation
  - ALWAYS run the full test suite before opening a PR
  - If stuck, add a comment asking for clarification and stop
```
