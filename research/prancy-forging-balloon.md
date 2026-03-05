# Gopilot Phase 1 MVP — Implementation Plan

## Context

Brian is building gopilot, a Go-based orchestrator that watches GitHub Issues and dispatches AI coding agents (Copilot CLI) to work on them autonomously. The full spec is at `~/gopilot/research/SPEC-DRAFT.md`. This plan covers Phase 1: the core poll-dispatch-reconcile loop with no dashboard, no skills, and no retry queue.

**Current state**: Go is not installed, no code exists, no GitHub repo. Only research docs in `~/gopilot/research/`.

## Prerequisites (before coding)

1. `brew install go task`
2. `gh repo create bketelsen/gopilot --private --source ~/gopilot`
3. `cd ~/gopilot && go mod init github.com/bketelsen/gopilot`

## Implementation Tasks (14 tasks, ordered so project compiles at every step)

### Task 0: Project Scaffold
- Create directory structure: `cmd/gopilot/`, `internal/{config,github,orchestrator,workspace,agent}/`
- Create minimal `cmd/gopilot/main.go` (prints "gopilot")
- Create `.gitignore`, `Taskfile.yml` (build, test, lint tasks)
- `git init`, initial commit
- **Verify**: `task build && ./gopilot` prints "gopilot"

### Task 1: Core Domain Types
- `internal/config/config.go` — Config, GitHubConfig, PollingConfig, WorkspaceConfig, AgentConfig structs with yaml tags
- `internal/github/issues.go` — Issue struct (ID, NodeID, Repo, URL, Title, Body, Labels, Status, Priority, CreatedAt, etc.)
- **Verify**: `go build ./...`

### Task 2: Config Parser
- Extend `internal/config/config.go` — `Load(path)`, env var resolution (`os.ExpandEnv`), defaults, validation
- `internal/config/config_test.go` — test valid parse, defaults, env vars, missing required fields
- **Dep**: `go get gopkg.in/yaml.v3`
- **Verify**: `go test ./internal/config/...`

### Task 3: Example Config + `gopilot init`
- `gopilot.yaml.example` at project root
- `internal/config/example.go` — embedded example config as string constant
- `cmd/gopilot/init.go` — writes example config, creates workspaces dir
- Add `version` subcommand to main.go
- **Verify**: `./gopilot init` creates gopilot.yaml, `./gopilot version` prints version

### Task 4: Structured Logging
- `internal/config/logging.go` — `SetupLogging(level)` using `slog.NewJSONHandler(os.Stderr, ...)`
- Update main.go: call SetupLogging, add `--debug` flag
- **Verify**: `./gopilot` outputs JSON logs to stderr

### Task 5: GitHub Client — REST
- `go get github.com/google/go-github/v68 golang.org/x/oauth2`
- `internal/github/client.go` — `Client` struct, `NewClient(token)`, `FetchIssues(ctx, owner, repo, labels)`, `FetchIssue(ctx, owner, repo, number)`, `AddComment(...)`
- `internal/github/issues.go` — `normalizeIssue()` (labels lowercase, repo format, timestamps)
- `internal/github/client_test.go` — test normalizeIssue with mock data
- **Verify**: `go test ./internal/github/...`

### Task 6: GitHub Client — Projects v2 GraphQL
- `internal/github/graphql.go` — `graphqlRequest(ctx, query, vars)` helper using raw GraphQL strings + `json.Unmarshal`
- `internal/github/projects.go` — `ProjectMeta` struct (field IDs, option mappings), `DiscoverProject(ctx, owner, number)`, `FetchProjectFields(ctx, meta, nodeID)`, `SetProjectStatus(ctx, meta, nodeID, status)`
- **Verify**: `go build ./...` (manual test with real project later)

### Task 7: Issue Eligibility + Candidate Fetching
- Extend `internal/github/issues.go` — `CandidateOpts` struct, `FetchCandidates(ctx, opts)` (fetch + filter + sort)
- Eligibility: open, has eligible label, no excluded label, status=Todo, not running/claimed
- Sort: priority ascending (1 first, 0 last), then created_at ascending
- `internal/github/issues_test.go` — test filtering and sorting with mock data
- **Verify**: `go test ./internal/github/...`

### Task 8: Workspace Manager
- `internal/workspace/manager.go` — `Manager` struct, `Ensure(ctx, repo, issueID)`, `PrepareForRun(...)`, `FinishRun(...)`, `Cleanup(...)`, `WorkspacePath(...)`
- `internal/workspace/hooks.go` — `runHook(ctx, script, workDir, vars, timeout)` using `exec.CommandContext("bash", "-c", script)`, template var expansion (`{{repo}}`, `{{issue_id}}`, `{{branch}}`)
- Path safety: sanitize repo name, validate under root
- `internal/workspace/manager_test.go` — test path determinism, sanitization, hook expansion, timeout
- **Verify**: `go test ./internal/workspace/...`

### Task 9: Agent Runner + Copilot Adapter
- `internal/agent/runner.go` — `AgentRunner` interface (`Start`, `Stop`, `Name`), `Session` struct, `AgentOpts` struct
- `internal/agent/copilot.go` — `CopilotRunner` implementing AgentRunner. Builds command: `copilot -p <prompt> --allow-all --no-ask-user --autopilot --max-autopilot-continues N --model M --share workspace/.gopilot-session.md -s`
- `internal/agent/process.go` — `stopProcess(pid)` (SIGTERM → 10s → SIGKILL), `randomHex(n)`
- `internal/agent/copilot_test.go` — test command construction, session ID uniqueness
- **Verify**: `go test ./internal/agent/...`

### Task 10: Prompt Rendering
- `internal/orchestrator/prompt.go` — `PromptData` struct, `RenderPrompt(tmplStr, data)` using `text/template` with custom `join` func
- `internal/orchestrator/prompt_test.go` — test basic fields, label join, attempt display, invalid template
- **Verify**: `go test ./internal/orchestrator/...`

### Task 11: Orchestrator State
- `internal/orchestrator/state.go` — `State` struct (mutex-protected maps), `RunEntry` struct, methods: `Claim`, `AddRunning`, `RemoveRunning`, `GetRunning`, `AllRunning`, `RunningCount`, `IsRunningOrClaimed`, `RunningIssueIDs`
- `internal/orchestrator/state_test.go` — test claim/release, concurrent access with `-race`
- **Verify**: `go test -race ./internal/orchestrator/...`

### Task 12: Orchestrator Dispatch
- `internal/orchestrator/dispatch.go` — `Dispatcher` struct, `Dispatch(ctx, issue)` method
- Dispatch steps: set status "In Progress" → ensure workspace → before_run hook → render prompt → launch agent → record in state → goroutine waits for exit
- On failure at any step: release claim, log error
- **Verify**: `go build ./...`

### Task 13: Orchestrator Core Loop + CLI Wiring
- `internal/orchestrator/orchestrator.go` — `Orchestrator` struct, `New(cfg)`, `Run(ctx)` (ticker loop), `tick(ctx)` (reconcile → fetch → sort → dispatch), `reconcile(ctx)`, `DryRun(ctx)`, `shutdown()`
- `cmd/gopilot/main.go` — final wiring: flag parsing (`--config`, `--dry-run`, `--debug`), signal handling (SIGINT/SIGTERM), subcommand routing
- **Verify**: `task build`, `./gopilot version`, `./gopilot --dry-run --config gopilot.yaml`

### Task 14: End-to-End Test
- Create `bketelsen/gopilot-testbed` repo with a Projects v2 board
- Create test issue with `gopilot` label, Status=Todo
- Run `./gopilot --dry-run` — verify issue listed
- Run `./gopilot` — verify dispatch, agent runs, cleanup works
- Test SIGINT graceful shutdown
- Push gopilot code to `bketelsen/gopilot`

## Key Architecture Decisions (Phase 1)

- **No external CLI framework** — stdlib `flag` + manual subcommand routing
- **Raw GraphQL over shurcooL/githubv4** — Projects v2 queries are deeply nested; raw strings + json.Unmarshal is more pragmatic
- **Interface-based GitHub client** — define interface in orchestrator package for testability
- **No retry queue** — agent runs once; human resets status to Todo to retry
- **Prompt via `-p` flag** — pass rendered prompt directly to copilot's `-p` argument
- **In-memory state only** — source of truth is always GitHub

## Files Created (29 total)

```
gopilot/
├── cmd/gopilot/
│   ├── main.go                          # CLI entry + signal handling
│   └── init.go                          # gopilot init subcommand
├── internal/
│   ├── config/
│   │   ├── config.go                    # Types + parser + validation
│   │   ├── config_test.go
│   │   ├── logging.go                   # slog setup
│   │   └── example.go                   # Embedded example config
│   ├── github/
│   │   ├── client.go                    # REST client + auth
│   │   ├── client_test.go
│   │   ├── issues.go                    # Issue type + fetch + filter + sort
│   │   ├── issues_test.go
│   │   ├── projects.go                  # Projects v2 GraphQL operations
│   │   └── graphql.go                   # Raw GraphQL helper
│   ├── orchestrator/
│   │   ├── orchestrator.go              # Core poll-dispatch-reconcile loop
│   │   ├── orchestrator_test.go
│   │   ├── state.go                     # In-memory state (mutex-protected)
│   │   ├── state_test.go
│   │   ├── dispatch.go                  # Issue dispatch logic
│   │   └── prompt.go                    # Prompt rendering
│   │   └── prompt_test.go
│   ├── workspace/
│   │   ├── manager.go                   # Workspace CRUD + safety
│   │   ├── hooks.go                     # Hook execution with timeout
│   │   └── manager_test.go
│   └── agent/
│       ├── runner.go                    # AgentRunner interface
│       ├── copilot.go                   # Copilot CLI adapter
│       ├── process.go                   # Process lifecycle helpers
│       └── copilot_test.go
├── gopilot.yaml.example
├── go.mod
├── Taskfile.yml
└── .gitignore
```

## Verification

After all tasks complete:
1. `task test` — all unit tests pass (with `-race`)
2. `task build` — single binary produced
3. `./gopilot version` — prints version
4. `./gopilot init` — creates starter config
5. `./gopilot --dry-run` — lists eligible issues from GitHub
6. `./gopilot` — runs orchestrator, dispatches agents, handles SIGINT gracefully
