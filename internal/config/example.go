package config

// ExampleConfig is the starter gopilot.yaml content written by `gopilot init`.
const ExampleConfig = `# gopilot.yaml — configuration for the gopilot orchestrator
# See: https://github.com/bketelsen/gopilot

github:
  # GitHub token — uses $GITHUB_TOKEN or $GH_TOKEN from environment if not set here
  # token: $GITHUB_TOKEN

  # Repositories to watch for eligible issues
  repos:
    - owner/repo

  # GitHub Projects v2 board for status/priority tracking
  project:
    owner: "@me"        # GitHub username or org name
    number: 1           # Project number

  # Issues must have at least one of these labels to be eligible
  eligible_labels:
    - gopilot

  # Issues with any of these labels are never dispatched
  excluded_labels:
    - blocked
    - needs-design
    - wontfix

polling:
  interval_ms: 30000           # Poll every 30 seconds
  max_concurrent_agents: 3     # Max parallel agent sessions

workspace:
  root: ./workspaces           # Where per-issue workspaces are created
  hooks:
    after_create: |
      git clone --branch main https://x-access-token:${GITHUB_TOKEN}@github.com/{{repo}}.git .
    before_run: |
      git fetch origin
      git checkout -B gopilot/issue-{{issue_id}} origin/main
    after_run: ""
    before_remove: ""

skills:
  # Directories to search for skill .md files (later dirs override earlier)
  dirs:
    - ./skills                       # Built-in skills shipped with gopilot
    # - ./custom-skills              # Your custom skills
  # Specific skills to enable (empty = all skills in dirs)
  enabled:
    - tdd
    - verification
    - pr-workflow
    # - debugging                    # Uncomment to enable

agent:
  command: "copilot"                # Agent CLI binary
  model: "claude-sonnet-4.6"       # Model for the agent to use
  max_autopilot_continues: 20      # Max autonomous continuation steps
  turn_timeout_ms: 1800000         # 30 min max per session
  stall_timeout_ms: 300000         # 5 min inactivity timeout
  max_retry_backoff_ms: 300000     # Max 5 min between retries
  max_retries: 3                   # Give up after N failures

prompt: |
  You are an AI software engineer working on a GitHub issue.

  ## Issue
  - Repository: {{ .Issue.Repo }}
  - Issue: #{{ .Issue.ID }} — {{ .Issue.Title }}
  - Labels: {{ join ", " .Issue.Labels }}
  - Priority: {{ .Issue.Priority }}

  ## Description
  {{ .Issue.Body }}

  {{ if gt .Attempt 1 }}
  ## Retry
  This is attempt {{ .Attempt }}. Previous attempt failed or timed out.
  Review any existing work in this workspace before starting fresh.
  {{ end }}

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
  - gh CLI for GitHub operations
  - Standard development tools in the workspace
`
