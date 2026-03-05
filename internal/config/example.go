package config

const ExampleConfig = `# gopilot.yaml — project configuration

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
`
