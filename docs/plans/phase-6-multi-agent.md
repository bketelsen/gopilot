# Phase 6: Multi-Agent & Extensions

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Claude Code adapter, agent selection config, sub-issue hierarchy, settings page.

**Prerequisite:** Phase 5 complete.

---

### Task 6.1: Claude Code Adapter

**Files:**
- Create: `internal/agent/claude.go`
- Test: `internal/agent/claude_test.go`

**Step 1: Write the failing test**

```go
// internal/agent/claude_test.go
package agent

import (
	"strings"
	"testing"
)

func TestClaudeBuildArgs(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	args := runner.buildArgs("/tmp/ws/.gopilot-prompt.md")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--dangerously-skip-permissions") {
		t.Error("missing --dangerously-skip-permissions flag")
	}
	if !strings.Contains(joined, "--print") {
		t.Error("missing --print flag")
	}
	if !strings.Contains(joined, ".gopilot-prompt.md") {
		t.Error("missing prompt file path")
	}
}

func TestClaudeName(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	if runner.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", runner.Name(), "claude")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/agent/... -run TestClaude`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/agent/claude.go
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// ClaudeRunner implements Runner for Claude Code CLI.
type ClaudeRunner struct {
	Command string
	Token   string
}

func (r *ClaudeRunner) Name() string { return "claude" }

func (r *ClaudeRunner) Start(ctx context.Context, workspace string, prompt string, opts AgentOpts) (*Session, error) {
	// Write prompt to file (Claude Code uses --print with a file)
	promptPath := filepath.Join(workspace, ".gopilot-prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0644); err != nil {
		return nil, fmt.Errorf("writing prompt file: %w", err)
	}

	args := r.buildArgs(promptPath)
	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, r.Command, args...)
	cmd.Dir = workspace
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(),
		"GITHUB_TOKEN="+r.Token,
		"GH_TOKEN="+r.Token,
	)
	for _, e := range opts.Env {
		cmd.Env = append(cmd.Env, e)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting claude: %w", err)
	}

	done := make(chan struct{})
	sess := &Session{
		ID:     fmt.Sprintf("claude-%d-%d", cmd.Process.Pid, time.Now().Unix()),
		PID:    cmd.Process.Pid,
		Cancel: cancel,
		Done:   done,
	}

	go func() {
		defer close(done)
		err := cmd.Wait()
		sess.ExitErr = err
		if cmd.ProcessState != nil {
			sess.ExitCode = cmd.ProcessState.ExitCode()
		}
	}()

	return sess, nil
}

func (r *ClaudeRunner) Stop(sess *Session) error {
	if sess.Cancel != nil {
		sess.Cancel()
	}
	proc, err := os.FindProcess(sess.PID)
	if err != nil {
		return nil
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		slog.Debug("SIGTERM failed", "pid", sess.PID, "error", err)
		return nil
	}
	select {
	case <-sess.Done:
		return nil
	case <-time.After(10 * time.Second):
		proc.Signal(syscall.SIGKILL)
		<-sess.Done
		return nil
	}
}

func (r *ClaudeRunner) buildArgs(promptPath string) []string {
	return []string{
		"--dangerously-skip-permissions",
		"--print", promptPath,
	}
}

var _ Runner = (*ClaudeRunner)(nil)
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/agent/... -run TestClaude`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/claude.go internal/agent/claude_test.go
git commit -m "feat: Claude Code CLI agent adapter"
```

---

### Task 6.2: Agent Selection Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/orchestrator/orchestrator.go`

**Step 1: Write the failing test**

```go
// Append to config_test.go

func TestAgentOverrides(t *testing.T) {
	yaml := `
github:
  token: tok
  repos: [owner/repo-a, owner/repo-b]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
workspace: {root: /tmp}
agent:
  command: copilot
  overrides:
    - repos: [owner/repo-b]
      command: claude
    - labels: [use-claude]
      command: claude
`
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Agent.Overrides) != 2 {
		t.Fatalf("overrides = %d, want 2", len(cfg.Agent.Overrides))
	}
	if cfg.Agent.Overrides[0].Command != "claude" {
		t.Errorf("override command = %q", cfg.Agent.Overrides[0].Command)
	}
}

func TestResolveAgentForIssue(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{
			Command: "copilot",
			Overrides: []AgentOverride{
				{Repos: []string{"owner/repo-b"}, Command: "claude"},
				{Labels: []string{"use-claude"}, Command: "claude"},
			},
		},
	}

	if cmd := cfg.AgentCommandForIssue("owner/repo-a", nil); cmd != "copilot" {
		t.Errorf("repo-a should use copilot, got %q", cmd)
	}
	if cmd := cfg.AgentCommandForIssue("owner/repo-b", nil); cmd != "claude" {
		t.Errorf("repo-b should use claude, got %q", cmd)
	}
	if cmd := cfg.AgentCommandForIssue("owner/repo-a", []string{"use-claude"}); cmd != "claude" {
		t.Errorf("use-claude label should use claude, got %q", cmd)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/config/... -run TestAgent`
Expected: FAIL

**Step 3: Add override types and resolution logic**

```go
// Add to config.go

type AgentOverride struct {
	Repos   []string `yaml:"repos"`
	Labels  []string `yaml:"labels"`
	Command string   `yaml:"command"`
}

// Add Overrides field to AgentConfig:
// Overrides []AgentOverride `yaml:"overrides"`

// AgentCommandForIssue returns the agent command for a given issue based on overrides.
func (c *Config) AgentCommandForIssue(repo string, labels []string) string {
	for _, override := range c.Agent.Overrides {
		for _, r := range override.Repos {
			if r == repo {
				return override.Command
			}
		}
		for _, ol := range override.Labels {
			for _, il := range labels {
				if strings.EqualFold(ol, il) {
					return override.Command
				}
			}
		}
	}
	return c.Agent.Command
}
```

**Step 4: Update orchestrator dispatch to select agent based on issue**

In `dispatch()`, resolve the agent command and pick the right runner:

```go
agentCmd := o.cfg.AgentCommandForIssue(issue.Repo, issue.Labels)
runner := o.agentForCommand(agentCmd) // returns CopilotRunner or ClaudeRunner
```

**Step 5: Run test to verify it passes**

Run: `go test -race ./internal/config/... -run TestAgent`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/config/ internal/orchestrator/
git commit -m "feat: agent selection config with per-repo and per-label overrides"
```

---

### Task 6.3: Sub-Issue Hierarchy and Blocking

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/types_test.go`

**Step 1: Write the failing test**

```go
// Append to types_test.go

func TestIsBlockedBy(t *testing.T) {
	issue := Issue{
		ID:        3,
		BlockedBy: []int{1, 2},
		Status:    "Todo",
		Labels:    []string{"gopilot"},
	}

	// All blockers done
	resolved := map[int]bool{1: true, 2: true}
	if issue.IsBlocked(resolved) {
		t.Error("should not be blocked when all blockers are resolved")
	}

	// One blocker still open
	partial := map[int]bool{1: true, 2: false}
	if !issue.IsBlocked(partial) {
		t.Error("should be blocked when some blockers are unresolved")
	}
}

func TestParseBlockedByFromBody(t *testing.T) {
	body := `This feature depends on:
- blocked by #42
- Blocked By #99
Some other text.`

	blockers := ParseBlockedBy(body)
	if len(blockers) != 2 {
		t.Fatalf("got %d blockers, want 2", len(blockers))
	}
	if blockers[0] != 42 || blockers[1] != 99 {
		t.Errorf("blockers = %v, want [42, 99]", blockers)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/domain/... -run TestIsBlocked`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// Add to types.go

import "regexp"

// IsBlocked returns true if any issue in BlockedBy is not resolved.
func (i Issue) IsBlocked(resolved map[int]bool) bool {
	for _, blocker := range i.BlockedBy {
		if !resolved[blocker] {
			return true
		}
	}
	return false
}

var blockedByRegex = regexp.MustCompile(`(?i)blocked\s+by\s+#(\d+)`)

// ParseBlockedBy extracts "blocked by #N" references from issue body text.
func ParseBlockedBy(body string) []int {
	matches := blockedByRegex.FindAllStringSubmatch(body, -1)
	var ids []int
	for _, m := range matches {
		if len(m) >= 2 {
			var id int
			fmt.Sscanf(m[1], "%d", &id)
			if id > 0 {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
```

**Step 4: Update eligibility in orchestrator**

In `Tick()`, after fetching candidates, parse `BlockedBy` from body and filter:

```go
// Parse blocked-by from body
for i := range candidates {
	candidates[i].BlockedBy = append(candidates[i].BlockedBy, domain.ParseBlockedBy(candidates[i].Body)...)
}

// Build resolved map from terminal issues
resolved := make(map[int]bool)
// ... query terminal state for all blocker IDs ...

// Filter blocked
var unblocked []domain.Issue
for _, c := range candidates {
	if !c.IsBlocked(resolved) {
		unblocked = append(unblocked, c)
	}
}
```

**Step 5: Run test to verify it passes**

Run: `go test -race ./internal/domain/...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/domain/ internal/orchestrator/
git commit -m "feat: sub-issue blocking with body text parsing"
```

---

### Task 6.4: Settings Page

**Files:**
- Create: `internal/web/templates/pages/settings.templ`
- Modify: `internal/web/server.go`

**Step 1: Create settings template**

```go
// internal/web/templates/pages/settings.templ
package pages

import (
	"fmt"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/skills"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

type SettingsData struct {
	Config       *config.Config
	Skills       []*skills.Skill
	GitHubStatus string // "connected" or error message
	AgentStatus  string // "found" or "not found"
}

templ Settings(data SettingsData) {
	@layouts.Base("Settings") {
		<div class="space-y-6">
			<h2 class="text-2xl font-bold">Settings</h2>

			<!-- GitHub Connection -->
			<div class="rounded-lg border border-border bg-card p-4">
				<h3 class="font-semibold mb-2">GitHub Connection</h3>
				<div class="flex items-center gap-2">
					if data.GitHubStatus == "connected" {
						<span class="h-2 w-2 rounded-full bg-green-500"></span>
						<span class="text-sm">Connected</span>
					} else {
						<span class="h-2 w-2 rounded-full bg-red-500"></span>
						<span class="text-sm text-destructive">{ data.GitHubStatus }</span>
					}
				</div>
				<p class="text-sm text-muted-foreground mt-1">
					Repos: { fmt.Sprintf("%v", data.Config.GitHub.Repos) }
				</p>
			</div>

			<!-- Agent -->
			<div class="rounded-lg border border-border bg-card p-4">
				<h3 class="font-semibold mb-2">Agent</h3>
				<p class="text-sm">Command: <code>{ data.Config.Agent.Command }</code></p>
				<p class="text-sm">Model: <code>{ data.Config.Agent.Model }</code></p>
				<p class="text-sm">Status: { data.AgentStatus }</p>
			</div>

			<!-- Loaded Skills -->
			<div class="rounded-lg border border-border bg-card p-4">
				<h3 class="font-semibold mb-2">Loaded Skills</h3>
				<div class="space-y-1">
					for _, skill := range data.Skills {
						<div class="flex items-center gap-2 text-sm">
							<span class="font-mono">{ skill.Name }</span>
							<span class={ "inline-flex items-center rounded-full px-2 py-0.5 text-xs", skillBadgeClass(skill.Type) }>
								{ skill.Type }
							</span>
							<span class="text-muted-foreground">{ skill.Description }</span>
						</div>
					}
				</div>
			</div>

			<!-- Config Summary -->
			<div class="rounded-lg border border-border bg-card p-4">
				<h3 class="font-semibold mb-2">Configuration</h3>
				<dl class="grid grid-cols-2 gap-2 text-sm">
					<dt class="text-muted-foreground">Poll interval</dt>
					<dd>{ fmt.Sprintf("%dms", data.Config.Polling.IntervalMS) }</dd>
					<dt class="text-muted-foreground">Max concurrent agents</dt>
					<dd>{ fmt.Sprintf("%d", data.Config.Polling.MaxConcurrentAgents) }</dd>
					<dt class="text-muted-foreground">Max retries</dt>
					<dd>{ fmt.Sprintf("%d", data.Config.Agent.MaxRetries) }</dd>
					<dt class="text-muted-foreground">Stall timeout</dt>
					<dd>{ fmt.Sprintf("%dms", data.Config.Agent.StallTimeoutMS) }</dd>
					<dt class="text-muted-foreground">Turn timeout</dt>
					<dd>{ fmt.Sprintf("%dms", data.Config.Agent.TurnTimeoutMS) }</dd>
				</dl>
			</div>
		</div>
	}
}

func skillBadgeClass(skillType string) string {
	switch skillType {
	case "rigid":
		return "bg-red-500/10 text-red-500"
	case "flexible":
		return "bg-blue-500/10 text-blue-500"
	case "technique":
		return "bg-yellow-500/10 text-yellow-500"
	default:
		return "bg-muted text-muted-foreground"
	}
}
```

**Step 2: Add settings route handler**

```go
r.Get("/settings", s.handleSettingsPage)

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	// Check agent binary exists
	agentStatus := "not found"
	if _, err := exec.LookPath(s.cfg.Agent.Command); err == nil {
		agentStatus = "found"
	}

	data := pages.SettingsData{
		Config:       s.cfg,
		Skills:       s.skills,
		GitHubStatus: "connected", // TODO: validate token
		AgentStatus:  agentStatus,
	}
	pages.Settings(data).Render(r.Context(), w)
}
```

**Step 3: Generate and build**

Run: `templ generate && go build ./...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add internal/web/
git commit -m "feat: settings page with config, skills, and status display"
```

---

## Phase 6 Milestone

Run: `go test -race ./...` — all tests pass.
Run: `go build -o gopilot ./cmd/gopilot/` — builds.

Multi-agent:
- Claude Code adapter with `--dangerously-skip-permissions --print`
- Per-repo and per-label agent selection overrides
- Sub-issue blocking via `BlockedBy` field and body text parsing
- Settings page with system health overview

---

## Final Verification

After all 6 phases:

```bash
# All tests pass
go test -race ./...

# Binary builds
task build

# Lint clean
task lint

# Version works
./gopilot version

# Init works
./gopilot init
cat gopilot.yaml
rm gopilot.yaml
```

The full gopilot system is now:
1. A long-running orchestrator that polls GitHub for eligible issues
2. Dispatches Copilot CLI (or Claude Code) agents to isolated workspaces
3. Enforces behavioral contracts via injected skills
4. Retries failed agents with exponential backoff
5. Detects stalls and reconciles against GitHub state
6. Provides a real-time web dashboard with SSE updates
7. Tracks token usage and estimated costs
8. Supports sprint views from GitHub Projects v2
9. Respects sub-issue blocking dependencies
10. Hot-reloads configuration without restart
