# Phase 1: Core Loop (MVP)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Config loading, GitHub client, workspace manager, agent runner, orchestrator loop, and CLI.

**Prerequisite:** Phase 0 complete (domain types, interfaces, dependencies).

---

### Task 1.1: Config Types and Defaults

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config

import "testing"

func TestDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()

	if cfg.Polling.IntervalMS != 30000 {
		t.Errorf("IntervalMS = %d, want 30000", cfg.Polling.IntervalMS)
	}
	if cfg.Polling.MaxConcurrentAgents != 3 {
		t.Errorf("MaxConcurrentAgents = %d, want 3", cfg.Polling.MaxConcurrentAgents)
	}
	if cfg.Agent.MaxAutopilotContinues != 20 {
		t.Errorf("MaxAutopilotContinues = %d, want 20", cfg.Agent.MaxAutopilotContinues)
	}
	if cfg.Agent.TurnTimeoutMS != 1800000 {
		t.Errorf("TurnTimeoutMS = %d, want 1800000", cfg.Agent.TurnTimeoutMS)
	}
	if cfg.Agent.StallTimeoutMS != 300000 {
		t.Errorf("StallTimeoutMS = %d, want 300000", cfg.Agent.StallTimeoutMS)
	}
	if cfg.Agent.MaxRetryBackoffMS != 300000 {
		t.Errorf("MaxRetryBackoffMS = %d, want 300000", cfg.Agent.MaxRetryBackoffMS)
	}
	if cfg.Agent.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.Agent.MaxRetries)
	}
	if cfg.Workspace.HookTimeoutMS != 60000 {
		t.Errorf("HookTimeoutMS = %d, want 60000", cfg.Workspace.HookTimeoutMS)
	}
	if cfg.Dashboard.Addr != ":3000" {
		t.Errorf("Addr = %q, want %q", cfg.Dashboard.Addr, ":3000")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/config/...`
Expected: FAIL — Config type not defined.

**Step 3: Write minimal implementation**

```go
// internal/config/config.go
package config

import "time"

// Config represents the gopilot.yaml configuration.
type Config struct {
	GitHub    GitHubConfig    `yaml:"github"`
	Polling   PollingConfig   `yaml:"polling"`
	Workspace WorkspaceConfig `yaml:"workspace"`
	Agent     AgentConfig     `yaml:"agent"`
	Skills    SkillsConfig    `yaml:"skills"`
	Dashboard DashboardConfig `yaml:"dashboard"`
	Prompt    string          `yaml:"prompt"`
}

type GitHubConfig struct {
	Token          string   `yaml:"token"`
	Repos          []string `yaml:"repos"`
	Project        ProjectConfig `yaml:"project"`
	EligibleLabels []string `yaml:"eligible_labels"`
	ExcludedLabels []string `yaml:"excluded_labels"`
}

type ProjectConfig struct {
	Owner  string `yaml:"owner"`
	Number int    `yaml:"number"`
}

type PollingConfig struct {
	IntervalMS          int `yaml:"interval_ms"`
	MaxConcurrentAgents int `yaml:"max_concurrent_agents"`
}

type WorkspaceConfig struct {
	Root          string       `yaml:"root"`
	HookTimeoutMS int          `yaml:"hook_timeout_ms"`
	Hooks         HooksConfig  `yaml:"hooks"`
}

type HooksConfig struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
}

type AgentConfig struct {
	Command              string `yaml:"command"`
	Model                string `yaml:"model"`
	MaxAutopilotContinues int   `yaml:"max_autopilot_continues"`
	TurnTimeoutMS        int    `yaml:"turn_timeout_ms"`
	StallTimeoutMS       int    `yaml:"stall_timeout_ms"`
	MaxRetryBackoffMS    int    `yaml:"max_retry_backoff_ms"`
	MaxRetries           int    `yaml:"max_retries"`
}

type SkillsConfig struct {
	Dir      string   `yaml:"dir"`
	Required []string `yaml:"required"`
	Optional []string `yaml:"optional"`
}

type DashboardConfig struct {
	Enabled bool   `yaml:"enabled"`
	Addr    string `yaml:"addr"`
}

// ApplyDefaults fills zero-value fields with spec defaults.
func (c *Config) ApplyDefaults() {
	if c.Polling.IntervalMS == 0 {
		c.Polling.IntervalMS = 30000
	}
	if c.Polling.MaxConcurrentAgents == 0 {
		c.Polling.MaxConcurrentAgents = 3
	}
	if c.Agent.MaxAutopilotContinues == 0 {
		c.Agent.MaxAutopilotContinues = 20
	}
	if c.Agent.TurnTimeoutMS == 0 {
		c.Agent.TurnTimeoutMS = 1800000
	}
	if c.Agent.StallTimeoutMS == 0 {
		c.Agent.StallTimeoutMS = 300000
	}
	if c.Agent.MaxRetryBackoffMS == 0 {
		c.Agent.MaxRetryBackoffMS = 300000
	}
	if c.Agent.MaxRetries == 0 {
		c.Agent.MaxRetries = 3
	}
	if c.Workspace.HookTimeoutMS == 0 {
		c.Workspace.HookTimeoutMS = 60000
	}
	if c.Dashboard.Addr == "" {
		c.Dashboard.Addr = ":3000"
	}
}

// PollInterval returns the polling interval as a time.Duration.
func (c *Config) PollInterval() time.Duration {
	return time.Duration(c.Polling.IntervalMS) * time.Millisecond
}

// StallTimeout returns the stall detection timeout as a time.Duration.
func (c *Config) StallTimeout() time.Duration {
	return time.Duration(c.Agent.StallTimeoutMS) * time.Millisecond
}

// TurnTimeout returns the per-turn timeout as a time.Duration.
func (c *Config) TurnTimeout() time.Duration {
	return time.Duration(c.Agent.TurnTimeoutMS) * time.Millisecond
}

// HookTimeout returns the hook execution timeout as a time.Duration.
func (c *Config) HookTimeout() time.Duration {
	return time.Duration(c.Workspace.HookTimeoutMS) * time.Millisecond
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/config/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: config types with defaults"
```

---

### Task 1.2: Config Loading — YAML Parsing and Env Resolution

**Files:**
- Create: `internal/config/loader.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
// Append to internal/config/config_test.go

import (
	"os"
	"path/filepath"
)

func TestLoadFromYAML(t *testing.T) {
	yaml := `
github:
  token: test-token
  repos:
    - owner/repo
  project:
    owner: "@me"
    number: 1
  eligible_labels:
    - gopilot
  excluded_labels:
    - blocked
polling:
  interval_ms: 15000
  max_concurrent_agents: 2
agent:
  command: copilot
  model: claude-sonnet-4.6
workspace:
  root: /tmp/workspaces
`
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub.Token != "test-token" {
		t.Errorf("Token = %q, want %q", cfg.GitHub.Token, "test-token")
	}
	if cfg.Polling.IntervalMS != 15000 {
		t.Errorf("IntervalMS = %d, want 15000", cfg.Polling.IntervalMS)
	}
	if cfg.Agent.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3 (default)", cfg.Agent.MaxRetries)
	}
}

func TestLoadEnvResolution(t *testing.T) {
	t.Setenv("GOPILOT_TEST_TOKEN", "secret-from-env")

	yaml := `
github:
  token: $GOPILOT_TEST_TOKEN
  repos:
    - owner/repo
  project:
    owner: "@me"
    number: 1
  eligible_labels:
    - gopilot
agent:
  command: copilot
workspace:
  root: /tmp/workspaces
`
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub.Token != "secret-from-env" {
		t.Errorf("Token = %q, want %q", cfg.GitHub.Token, "secret-from-env")
	}
}

func TestLoadValidation(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"missing token", `
github:
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
`},
		{"missing repos", `
github:
  token: tok
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
`},
		{"missing eligible_labels", `
github:
  token: tok
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
agent: {command: copilot}
workspace: {root: /tmp}
`},
		{"missing agent command", `
github:
  token: tok
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
workspace: {root: /tmp}
`},
		{"missing workspace root", `
github:
  token: tok
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "gopilot.yaml")
			os.WriteFile(path, []byte(tt.yaml), 0644)

			_, err := Load(path)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/config/...`
Expected: FAIL — `Load` not defined.

**Step 3: Write minimal implementation**

```go
// internal/config/loader.go
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a gopilot.yaml file, resolves env vars, applies defaults, and validates.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.GitHub.Token = resolveEnv(cfg.GitHub.Token)
	cfg.Workspace.Root = resolveEnv(cfg.Workspace.Root)

	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks that all required fields are present.
func (c *Config) Validate() error {
	if c.GitHub.Token == "" {
		return fmt.Errorf("github.token is required")
	}
	if len(c.GitHub.Repos) == 0 {
		return fmt.Errorf("github.repos must have at least one entry")
	}
	if len(c.GitHub.EligibleLabels) == 0 {
		return fmt.Errorf("github.eligible_labels must have at least one entry")
	}
	if c.Agent.Command == "" {
		return fmt.Errorf("agent.command is required")
	}
	if c.Workspace.Root == "" {
		return fmt.Errorf("workspace.root is required")
	}
	return nil
}

// resolveEnv expands a string that starts with $ as an environment variable.
func resolveEnv(s string) string {
	if strings.HasPrefix(s, "$") {
		name := strings.TrimPrefix(s, "$")
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
	}
	return s
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/config/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: config loading with env resolution and validation"
```

---

### Task 1.3: Example Config for `gopilot init`

**Files:**
- Create: `internal/config/example.go`

**Step 1: Write the example config**

```go
// internal/config/example.go
package config

// ExampleConfig is written by `gopilot init`.
const ExampleConfig = `# gopilot.yaml — project configuration

github:
  token: $GITHUB_TOKEN              # or $GH_TOKEN
  repos:
    - owner/repo
  project:
    owner: "@me"                    # or org name
    number: 1
  eligible_labels:
    - gopilot
  excluded_labels:
    - blocked
    - needs-design
    - wontfix

polling:
  interval_ms: 30000               # 30 seconds
  max_concurrent_agents: 3

workspace:
  root: ./workspaces
  hook_timeout_ms: 60000            # 60 seconds
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
  turn_timeout_ms: 1800000          # 30 minutes
  stall_timeout_ms: 300000          # 5 minutes
  max_retry_backoff_ms: 300000      # 5 minutes
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
  3. Write failing tests that verify the requirements (TDD red)
  4. Implement the minimum code to pass the tests (TDD green)
  5. Refactor if needed (TDD refactor)
  6. Run the full test suite — all tests must pass
  7. Create a branch, commit your changes, and open a pull request
  8. Add a comment to the issue summarizing what you did

  ## Rules
  - NEVER push directly to main
  - ALWAYS write tests before implementation
  - ALWAYS run the full test suite before opening a PR
  - If you are stuck, add a comment asking for clarification and stop
`

**Step 2: Commit**

```bash
git add internal/config/example.go
git commit -m "feat: example config for gopilot init"
```

---

### Task 1.4: Structured Logging Setup

**Files:**
- Create: `internal/logging/logging.go`

**Step 1: Write the implementation** (simple enough to not need TDD)

```go
// internal/logging/logging.go
package logging

import (
	"log/slog"
	"os"
)

// Setup configures the global slog logger to write JSON to stderr.
func Setup(level slog.Level) {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

// IssueLogger returns a logger with issue context fields.
func IssueLogger(repo string, id int, sessionID string) *slog.Logger {
	return slog.With(
		"issue_id", id,
		"issue", repo+"#"+slog.IntValue(id).String(),
		"session_id", sessionID,
	)
}
```

Wait — `slog.IntValue` doesn't exist. Let me fix:

```go
// internal/logging/logging.go
package logging

import (
	"fmt"
	"log/slog"
	"os"
)

// Setup configures the global slog logger to write JSON to stderr.
func Setup(level slog.Level) {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

// IssueLogger returns a logger with issue context fields.
func IssueLogger(repo string, id int, sessionID string) *slog.Logger {
	return slog.With(
		"issue_id", id,
		"issue", fmt.Sprintf("%s#%d", repo, id),
		"session_id", sessionID,
	)
}
```

**Step 2: Commit**

```bash
git add internal/logging/
git commit -m "feat: structured logging setup with issue context"
```

---

### Task 1.5: Workspace Manager Implementation

**Files:**
- Create: `internal/workspace/fs_manager.go`
- Test: `internal/workspace/fs_manager_test.go`

**Step 1: Write the failing test**

```go
// internal/workspace/fs_manager_test.go
package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

func TestFSManagerPath(t *testing.T) {
	mgr := NewFSManager(config.WorkspaceConfig{Root: "/tmp/workspaces"})
	issue := domain.Issue{ID: 42, Repo: "owner/my-repo"}
	got := mgr.Path(issue)
	want := "/tmp/workspaces/my-repo/issue-42"
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestFSManagerPathSafety(t *testing.T) {
	mgr := NewFSManager(config.WorkspaceConfig{Root: "/tmp/workspaces"})
	issue := domain.Issue{ID: 42, Repo: "owner/../../../etc"}
	path := mgr.Path(issue)
	if !strings.HasPrefix(path, "/tmp/workspaces/") {
		t.Errorf("Path() = %q, escapes root", path)
	}
	if strings.Contains(path, "..") {
		t.Errorf("Path() = %q, contains traversal", path)
	}
}

func TestFSManagerEnsure(t *testing.T) {
	root := t.TempDir()
	mgr := NewFSManager(config.WorkspaceConfig{Root: root})
	issue := domain.Issue{ID: 1, Repo: "owner/repo"}

	path, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("workspace dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("workspace path is not a directory")
	}

	// Second call reuses
	path2, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	if path2 != path {
		t.Errorf("second Ensure() = %q, want %q", path2, path)
	}
}

func TestFSManagerCleanup(t *testing.T) {
	root := t.TempDir()
	mgr := NewFSManager(config.WorkspaceConfig{Root: root})
	issue := domain.Issue{ID: 1, Repo: "owner/repo"}

	path, _ := mgr.Ensure(context.Background(), issue)
	// Create a file in the workspace
	os.WriteFile(filepath.Join(path, "test.txt"), []byte("hello"), 0644)

	err := mgr.Cleanup(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("workspace not cleaned up")
	}
}

func TestFSManagerHookExpansion(t *testing.T) {
	root := t.TempDir()
	cfg := config.WorkspaceConfig{
		Root:          root,
		HookTimeoutMS: 5000,
		Hooks: config.HooksConfig{
			AfterCreate: `echo "repo={{repo}} issue={{issue_id}}" > hook_output.txt`,
		},
	}
	mgr := NewFSManager(cfg)
	issue := domain.Issue{ID: 99, Repo: "myorg/myrepo"}

	path, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(path, "hook_output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	want := "repo=myorg/myrepo issue=99"
	if got != want {
		t.Errorf("hook output = %q, want %q", got, want)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/workspace/...`
Expected: FAIL — `NewFSManager` not defined.

**Step 3: Write minimal implementation**

```go
// internal/workspace/fs_manager.go
package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// FSManager implements Manager using the local filesystem.
type FSManager struct {
	cfg config.WorkspaceConfig
}

// NewFSManager creates a new filesystem-based workspace manager.
func NewFSManager(cfg config.WorkspaceConfig) *FSManager {
	return &FSManager{cfg: cfg}
}

func (m *FSManager) Path(issue domain.Issue) string {
	repoName := sanitizePath(repoShortName(issue.Repo))
	dirName := fmt.Sprintf("issue-%d", issue.ID)
	return filepath.Join(m.cfg.Root, repoName, dirName)
}

func (m *FSManager) Ensure(ctx context.Context, issue domain.Issue) (string, error) {
	path := m.Path(issue)

	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path, nil
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("creating workspace: %w", err)
	}

	if m.cfg.Hooks.AfterCreate != "" {
		if err := m.runHook(ctx, m.cfg.Hooks.AfterCreate, path, issue); err != nil {
			os.RemoveAll(path)
			return "", fmt.Errorf("after_create hook: %w", err)
		}
	}

	return path, nil
}

func (m *FSManager) RunHook(ctx context.Context, hook string, workspacePath string, issue domain.Issue) error {
	var script string
	switch hook {
	case "before_run":
		script = m.cfg.Hooks.BeforeRun
	case "after_run":
		script = m.cfg.Hooks.AfterRun
	case "before_remove":
		script = m.cfg.Hooks.BeforeRemove
	default:
		return fmt.Errorf("unknown hook: %s", hook)
	}
	if script == "" {
		return nil
	}
	return m.runHook(ctx, script, workspacePath, issue)
}

func (m *FSManager) Cleanup(ctx context.Context, issue domain.Issue) error {
	path := m.Path(issue)

	if m.cfg.Hooks.BeforeRemove != "" {
		if err := m.runHook(ctx, m.cfg.Hooks.BeforeRemove, path, issue); err != nil {
			slog.Warn("before_remove hook failed", "error", err, "path", path)
		}
	}

	return os.RemoveAll(path)
}

func (m *FSManager) runHook(ctx context.Context, script string, dir string, issue domain.Issue) error {
	expanded := expandHookVars(script, issue, dir)

	timeout := time.Duration(m.cfg.HookTimeoutMS) * time.Millisecond
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", expanded)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr // hooks log to stderr
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func expandHookVars(script string, issue domain.Issue, workspace string) string {
	r := strings.NewReplacer(
		"{{repo}}", issue.Repo,
		"{{issue_id}}", fmt.Sprintf("%d", issue.ID),
		"{{branch}}", fmt.Sprintf("gopilot/issue-%d", issue.ID),
		"{{workspace}}", workspace,
	)
	return r.Replace(script)
}

// repoShortName extracts "repo" from "owner/repo".
func repoShortName(repo string) string {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return repo
}

// sanitizePath removes path traversal characters.
func sanitizePath(s string) string {
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	return s
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/workspace/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/workspace/
git commit -m "feat: filesystem workspace manager with hooks and path safety"
```

---

### Task 1.6: Prompt Renderer

**Files:**
- Create: `internal/prompt/renderer.go`
- Test: `internal/prompt/renderer_test.go`

**Step 1: Write the failing test**

```go
// internal/prompt/renderer_test.go
package prompt

import (
	"strings"
	"testing"

	"github.com/bketelsen/gopilot/internal/domain"
)

func TestRender(t *testing.T) {
	tmpl := `Issue: {{ .Issue.Repo }}#{{ .Issue.ID }} — {{ .Issue.Title }}
Labels: {{ joinStrings .Issue.Labels ", " }}
Attempt: {{ .Attempt }}`

	issue := domain.Issue{
		ID:     42,
		Repo:   "owner/repo",
		Title:  "Fix the bug",
		Labels: []string{"gopilot", "bug"},
	}

	result, err := Render(tmpl, issue, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "owner/repo#42") {
		t.Errorf("missing issue identifier in: %s", result)
	}
	if !strings.Contains(result, "gopilot, bug") {
		t.Errorf("missing labels in: %s", result)
	}
	if !strings.Contains(result, "Attempt: 1") {
		t.Errorf("missing attempt in: %s", result)
	}
}

func TestRenderWithSkills(t *testing.T) {
	tmpl := `Do the work.
{{ .Skills }}`

	issue := domain.Issue{ID: 1, Repo: "o/r"}
	skills := "## TDD\nWrite tests first."

	result, err := Render(tmpl, issue, 1, skills)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "## TDD") {
		t.Errorf("missing skills in: %s", result)
	}
}

func TestRenderRetryContext(t *testing.T) {
	tmpl := `{{ if gt .Attempt 1 }}RETRY attempt {{ .Attempt }}{{ end }}`

	issue := domain.Issue{ID: 1, Repo: "o/r"}

	result1, _ := Render(tmpl, issue, 1, "")
	if strings.Contains(result1, "RETRY") {
		t.Error("attempt 1 should not show retry context")
	}

	result2, _ := Render(tmpl, issue, 3, "")
	if !strings.Contains(result2, "RETRY attempt 3") {
		t.Errorf("attempt 3 should show retry: %s", result2)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/prompt/...`
Expected: FAIL — package doesn't exist.

**Step 3: Write minimal implementation**

```go
// internal/prompt/renderer.go
package prompt

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/bketelsen/gopilot/internal/domain"
)

// PromptData is the data passed to the prompt template.
type PromptData struct {
	Issue   domain.Issue
	Attempt int
	Skills  string
}

// Render executes the prompt template with the given data.
func Render(tmpl string, issue domain.Issue, attempt int, skills string) (string, error) {
	funcMap := template.FuncMap{
		"joinStrings": strings.Join,
	}

	t, err := template.New("prompt").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}

	data := PromptData{
		Issue:   issue,
		Attempt: attempt,
		Skills:  skills,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/prompt/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/prompt/
git commit -m "feat: prompt renderer with Go templates and skill injection"
```

---

### Task 1.7: Agent Runner — Copilot CLI Adapter

**Files:**
- Create: `internal/agent/copilot.go`
- Test: `internal/agent/copilot_test.go`

**Step 1: Write the failing test**

We can't test actual copilot CLI invocation, so test the command construction and the process lifecycle using a mock script.

```go
// internal/agent/copilot_test.go
package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCopilotBuildArgs(t *testing.T) {
	runner := &CopilotRunner{
		Command: "copilot",
	}
	opts := AgentOpts{
		Model:            "claude-sonnet-4.6",
		MaxContinuations: 20,
	}
	args := runner.buildArgs("Do the work", "/tmp/ws", opts)

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-p") {
		t.Error("missing -p flag")
	}
	if !strings.Contains(joined, "--autopilot") {
		t.Error("missing --autopilot flag")
	}
	if !strings.Contains(joined, "--allow-all") {
		t.Error("missing --allow-all flag")
	}
	if !strings.Contains(joined, "--no-ask-user") {
		t.Error("missing --no-ask-user flag")
	}
	if !strings.Contains(joined, "--model claude-sonnet-4.6") {
		t.Error("missing --model flag")
	}
	if !strings.Contains(joined, "--max-autopilot-continues 20") {
		t.Error("missing --max-autopilot-continues flag")
	}
	if !strings.Contains(joined, "-s") {
		t.Error("missing -s flag")
	}
}

func TestCopilotStartStop(t *testing.T) {
	// Use a simple script that sleeps as a mock agent
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-agent.sh")
	os.WriteFile(script, []byte("#!/bin/bash\nsleep 30"), 0755)

	runner := &CopilotRunner{Command: script}
	ctx := context.Background()
	sess, err := runner.Start(ctx, dir, "test prompt", AgentOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if sess.PID == 0 {
		t.Error("PID should be non-zero")
	}

	err = runner.Stop(sess)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for process to exit
	select {
	case <-sess.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit after Stop")
	}
}

func TestCopilotName(t *testing.T) {
	runner := &CopilotRunner{Command: "copilot"}
	if runner.Name() != "copilot" {
		t.Errorf("Name() = %q, want %q", runner.Name(), "copilot")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/agent/...`
Expected: FAIL — `CopilotRunner` not defined.

**Step 3: Write minimal implementation**

```go
// internal/agent/copilot.go
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

// CopilotRunner implements Runner for GitHub Copilot CLI.
type CopilotRunner struct {
	Command string // path to copilot binary
	Token   string // GitHub token for agent env
}

func (r *CopilotRunner) Name() string {
	return "copilot"
}

func (r *CopilotRunner) Start(ctx context.Context, workspace string, prompt string, opts AgentOpts) (*Session, error) {
	args := r.buildArgs(prompt, workspace, opts)

	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, r.Command, args...)
	cmd.Dir = workspace
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Env = append(os.Environ(),
		"GITHUB_TOKEN="+r.Token,
		"COPILOT_GITHUB_TOKEN="+r.Token,
		"GH_TOKEN="+r.Token,
	)
	for _, e := range opts.Env {
		cmd.Env = append(cmd.Env, e)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting agent: %w", err)
	}

	done := make(chan struct{})
	sess := &Session{
		ID:     fmt.Sprintf("sess-%d-%d", cmd.Process.Pid, time.Now().Unix()),
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

func (r *CopilotRunner) Stop(sess *Session) error {
	if sess.Cancel != nil {
		sess.Cancel()
	}

	// Try SIGTERM first
	proc, err := os.FindProcess(sess.PID)
	if err != nil {
		return nil // already gone
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		slog.Debug("SIGTERM failed, process may already be gone", "pid", sess.PID, "error", err)
		return nil
	}

	// Wait up to 10s for graceful exit
	select {
	case <-sess.Done:
		return nil
	case <-time.After(10 * time.Second):
		slog.Warn("agent did not exit after SIGTERM, sending SIGKILL", "pid", sess.PID)
		proc.Signal(syscall.SIGKILL)
		<-sess.Done
		return nil
	}
}

func (r *CopilotRunner) buildArgs(prompt string, workspace string, opts AgentOpts) []string {
	sharePath := filepath.Join(workspace, ".gopilot-session.md")
	args := []string{
		"-p", prompt,
		"--allow-all",
		"--no-ask-user",
		"--autopilot",
		"--share", sharePath,
		"-s",
	}
	if opts.MaxContinuations > 0 {
		args = append(args, "--max-autopilot-continues", strconv.Itoa(opts.MaxContinuations))
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return args
}

// EventCallback is called when the agent produces output.
type EventCallback func(event domain.AgentEvent)

// Ensure CopilotRunner implements Runner.
var _ Runner = (*CopilotRunner)(nil)
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/agent/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat: Copilot CLI agent runner with process lifecycle"
```

---

### Task 1.8: GitHub REST Client — Issue Fetching

**Files:**
- Create: `internal/github/rest.go`
- Test: `internal/github/rest_test.go`

**Step 1: Write the failing test**

```go
// internal/github/rest_test.go
package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

func TestNormalizeLabels(t *testing.T) {
	labels := normalizeLabels([]string{"Gopilot", "BUG", "Feature"})
	want := []string{"gopilot", "bug", "feature"}
	for i, got := range labels {
		if got != want[i] {
			t.Errorf("label[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestFetchCandidateIssues(t *testing.T) {
	// Mock GitHub API
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		issues := []map[string]any{
			{
				"number": 1,
				"title":  "First issue",
				"body":   "Do something",
				"state":  "open",
				"html_url": "https://github.com/owner/repo/issues/1",
				"node_id": "MDU6SXNzdWUx",
				"labels": []map[string]any{
					{"name": "gopilot"},
					{"name": "bug"},
				},
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z",
			},
			{
				"number": 2,
				"title":  "Blocked issue",
				"body":   "",
				"state":  "open",
				"html_url": "https://github.com/owner/repo/issues/2",
				"node_id": "MDU6SXNzdWUy",
				"labels": []map[string]any{
					{"name": "gopilot"},
					{"name": "blocked"},
				},
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z",
			},
		}
		json.NewEncoder(w).Encode(issues)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{
		Token:          "test-token",
		Repos:          []string{"owner/repo"},
		EligibleLabels: []string{"gopilot"},
		ExcludedLabels: []string{"blocked"},
	}
	client := NewRESTClient(cfg, server.URL+"/")

	issues, err := client.FetchCandidateIssues(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should return only issue 1 (issue 2 has excluded label "blocked")
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}
	if issues[0].ID != 1 {
		t.Errorf("issue ID = %d, want 1", issues[0].ID)
	}
	if issues[0].Labels[0] != "gopilot" {
		t.Errorf("label = %q, want %q", issues[0].Labels[0], "gopilot")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/github/...`
Expected: FAIL — `NewRESTClient`, `normalizeLabels` not defined.

**Step 3: Write minimal implementation**

```go
// internal/github/rest.go
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// RESTClient implements GitHub REST API operations.
type RESTClient struct {
	cfg     config.GitHubConfig
	baseURL string
	http    *http.Client
}

// NewRESTClient creates a REST client. baseURL should end with "/".
// For production, use "https://api.github.com/".
func NewRESTClient(cfg config.GitHubConfig, baseURL string) *RESTClient {
	return &RESTClient{
		cfg:     cfg,
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *RESTClient) FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error) {
	var all []domain.Issue
	for _, repo := range c.cfg.Repos {
		issues, err := c.fetchRepoIssues(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", repo, err)
		}
		all = append(all, issues...)
	}
	return all, nil
}

func (c *RESTClient) fetchRepoIssues(ctx context.Context, repo string) ([]domain.Issue, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", repo)
	}
	owner, name := parts[0], parts[1]

	url := fmt.Sprintf("%srepos/%s/%s/issues?state=open&per_page=100", c.baseURL, owner, name)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var raw []ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding issues: %w", err)
	}

	var issues []domain.Issue
	for _, r := range raw {
		issue := r.toDomain(repo)
		if issue.IsEligible(c.cfg.EligibleLabels, c.cfg.ExcludedLabels) {
			issues = append(issues, issue)
		}
	}
	return issues, nil
}

func (c *RESTClient) FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d", c.baseURL, parts[0], parts[1], id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var raw ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	issue := raw.toDomain(repo)
	return &issue, nil
}

func (c *RESTClient) AddComment(ctx context.Context, repo string, id int, body string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/comments", c.baseURL, parts[0], parts[1], id)

	payload := fmt.Sprintf(`{"body":%q}`, body)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (c *RESTClient) AddLabel(ctx context.Context, repo string, id int, label string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/labels", c.baseURL, parts[0], parts[1], id)

	payload := fmt.Sprintf(`{"labels":[%q]}`, label)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ghIssue is the raw GitHub API response shape.
type ghIssue struct {
	Number    int       `json:"number"`
	NodeID    string    `json:"node_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	HTMLURL   string    `json:"html_url"`
	Labels    []ghLabel `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ghLabel struct {
	Name string `json:"name"`
}

func (g ghIssue) toDomain(repo string) domain.Issue {
	labels := make([]string, len(g.Labels))
	for i, l := range g.Labels {
		labels[i] = l.Name
	}
	return domain.Issue{
		ID:        g.Number,
		NodeID:    g.NodeID,
		Repo:      repo,
		URL:       g.HTMLURL,
		Title:     g.Title,
		Body:      g.Body,
		Labels:    normalizeLabels(labels),
		Status:    "Todo", // Default — overridden by Projects v2 enrichment
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}

func normalizeLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = strings.ToLower(l)
	}
	return out
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/github/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/github/
git commit -m "feat: GitHub REST client with issue fetching and normalization"
```

---

### Task 1.9: GitHub GraphQL Client — Projects v2

**Files:**
- Create: `internal/github/graphql.go`
- Test: `internal/github/graphql_test.go`

**Step 1: Write the failing test**

```go
// internal/github/graphql_test.go
package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/gopilot/internal/config"
)

func TestDiscoverProjectFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"id": "PVT_123",
						"fields": map[string]any{
							"nodes": []any{
								map[string]any{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "PVTSSF_status",
									"name":       "Status",
									"options": []any{
										map[string]any{"id": "opt_todo", "name": "Todo"},
										map[string]any{"id": "opt_ip", "name": "In Progress"},
										map[string]any{"id": "opt_ir", "name": "In Review"},
										map[string]any{"id": "opt_done", "name": "Done"},
									},
								},
								map[string]any{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "PVTSSF_priority",
									"name":       "Priority",
									"options": []any{
										map[string]any{"id": "opt_urgent", "name": "Urgent"},
										map[string]any{"id": "opt_high", "name": "High"},
										map[string]any{"id": "opt_med", "name": "Medium"},
										map[string]any{"id": "opt_low", "name": "Low"},
									},
								},
							},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{
		Token:   "test-token",
		Project: config.ProjectConfig{Owner: "@me", Number: 1},
	}
	gql := NewGraphQLClient(cfg, server.URL+"/graphql")
	meta, err := gql.DiscoverProjectFields(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProjectID != "PVT_123" {
		t.Errorf("ProjectID = %q, want %q", meta.ProjectID, "PVT_123")
	}
	if meta.StatusFieldID != "PVTSSF_status" {
		t.Errorf("StatusFieldID = %q", meta.StatusFieldID)
	}
	if meta.StatusOptions["In Progress"] != "opt_ip" {
		t.Errorf("In Progress option = %q", meta.StatusOptions["In Progress"])
	}
	if meta.PriorityFieldID != "PVTSSF_priority" {
		t.Errorf("PriorityFieldID = %q", meta.PriorityFieldID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/github/...`
Expected: FAIL — `NewGraphQLClient` not defined.

**Step 3: Write minimal implementation**

```go
// internal/github/graphql.go
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bketelsen/gopilot/internal/config"
)

// ProjectMeta holds discovered Projects v2 field IDs.
type ProjectMeta struct {
	ProjectID       string
	StatusFieldID   string
	StatusOptions   map[string]string // "In Progress" -> option ID
	PriorityFieldID string
	PriorityOptions map[string]string // "Urgent" -> option ID
}

// GraphQLClient handles GitHub GraphQL API calls.
type GraphQLClient struct {
	cfg      config.GitHubConfig
	endpoint string
	http     *http.Client
	meta     *ProjectMeta
}

// NewGraphQLClient creates a GraphQL client.
func NewGraphQLClient(cfg config.GitHubConfig, endpoint string) *GraphQLClient {
	return &GraphQLClient{
		cfg:      cfg,
		endpoint: endpoint,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// DiscoverProjectFields queries the project schema to find field IDs.
func (c *GraphQLClient) DiscoverProjectFields(ctx context.Context) (*ProjectMeta, error) {
	ownerField := "user"
	ownerLogin := c.cfg.Project.Owner
	if ownerLogin == "@me" {
		ownerField = "viewer"
	} else if !strings.HasPrefix(ownerLogin, "@") {
		ownerField = "organization"
	}

	var query string
	if ownerField == "viewer" {
		query = fmt.Sprintf(`{
			viewer {
				projectV2(number: %d) {
					id
					fields(first: 20) {
						nodes {
							__typename
							... on ProjectV2SingleSelectField {
								id
								name
								options { id name }
							}
							... on ProjectV2IterationField {
								id
								name
							}
							... on ProjectV2Field {
								id
								name
							}
						}
					}
				}
			}
		}`, c.cfg.Project.Number)
	} else {
		query = fmt.Sprintf(`{
			%s(login: %q) {
				projectV2(number: %d) {
					id
					fields(first: 20) {
						nodes {
							__typename
							... on ProjectV2SingleSelectField {
								id
								name
								options { id name }
							}
							... on ProjectV2IterationField {
								id
								name
							}
							... on ProjectV2Field {
								id
								name
							}
						}
					}
				}
			}
		}`, ownerField, ownerLogin, c.cfg.Project.Number)
	}

	result, err := c.execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	// Navigate to the projectV2 node regardless of owner type
	var projectNode map[string]any
	if data, ok := result["data"].(map[string]any); ok {
		for _, v := range data {
			if obj, ok := v.(map[string]any); ok {
				if pv2, ok := obj["projectV2"].(map[string]any); ok {
					projectNode = pv2
					break
				}
			}
		}
	}
	// Handle viewer case where it's directly under data
	if projectNode == nil {
		if data, ok := result["data"].(map[string]any); ok {
			if viewer, ok := data["viewer"].(map[string]any); ok {
				if pv2, ok := viewer["projectV2"].(map[string]any); ok {
					projectNode = pv2
				}
			}
			if user, ok := data["user"].(map[string]any); ok {
				if pv2, ok := user["projectV2"].(map[string]any); ok {
					projectNode = pv2
				}
			}
		}
	}
	if projectNode == nil {
		return nil, fmt.Errorf("project not found in response")
	}

	meta := &ProjectMeta{
		ProjectID:       projectNode["id"].(string),
		StatusOptions:   make(map[string]string),
		PriorityOptions: make(map[string]string),
	}

	fields := projectNode["fields"].(map[string]any)["nodes"].([]any)
	for _, f := range fields {
		field := f.(map[string]any)
		name, _ := field["name"].(string)
		id, _ := field["id"].(string)
		typename, _ := field["__typename"].(string)

		if typename == "ProjectV2SingleSelectField" {
			options, _ := field["options"].([]any)
			optMap := make(map[string]string)
			for _, o := range options {
				opt := o.(map[string]any)
				optMap[opt["name"].(string)] = opt["id"].(string)
			}

			switch name {
			case "Status":
				meta.StatusFieldID = id
				meta.StatusOptions = optMap
			case "Priority":
				meta.PriorityFieldID = id
				meta.PriorityOptions = optMap
			}
		}
	}

	c.meta = meta
	return meta, nil
}

// SetProjectStatus sets the Status field on a project item.
func (c *GraphQLClient) SetProjectStatus(ctx context.Context, itemID string, status string) error {
	if c.meta == nil {
		return fmt.Errorf("project metadata not discovered — call DiscoverProjectFields first")
	}
	optionID, ok := c.meta.StatusOptions[status]
	if !ok {
		return fmt.Errorf("unknown status %q", status)
	}

	mutation := `mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String!) {
		updateProjectV2ItemFieldValue(input: {
			projectId: $projectId
			itemId: $itemId
			fieldId: $fieldId
			value: { singleSelectOptionId: $optionId }
		}) {
			projectV2Item { id }
		}
	}`

	vars := map[string]any{
		"projectId": c.meta.ProjectID,
		"itemId":    itemID,
		"fieldId":   c.meta.StatusFieldID,
		"optionId":  optionID,
	}

	_, err := c.execute(ctx, mutation, vars)
	return err
}

func (c *GraphQLClient) execute(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL error %d: %s", resp.StatusCode, respBody)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	if errs, ok := result["errors"]; ok {
		return nil, fmt.Errorf("GraphQL errors: %v", errs)
	}

	return result, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/github/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/github/
git commit -m "feat: GraphQL client for Projects v2 field discovery and mutations"
```

---

### Task 1.10: Orchestrator State

**Files:**
- Create: `internal/orchestrator/state.go`
- Test: `internal/orchestrator/state_test.go`

**Step 1: Write the failing test**

```go
// internal/orchestrator/state_test.go
package orchestrator

import (
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

func TestStateClaimAndRelease(t *testing.T) {
	s := NewState()

	if !s.Claim(42) {
		t.Error("first claim should succeed")
	}
	if s.Claim(42) {
		t.Error("second claim should fail")
	}

	s.Release(42)
	if !s.Claim(42) {
		t.Error("claim after release should succeed")
	}
}

func TestStateRunning(t *testing.T) {
	s := NewState()

	entry := &domain.RunEntry{
		Issue:     domain.Issue{ID: 42, Repo: "o/r"},
		SessionID: "sess-1",
		StartedAt: time.Now(),
	}
	s.AddRunning(42, entry)

	if got := s.GetRunning(42); got != entry {
		t.Error("GetRunning should return the entry")
	}
	if s.RunningCount() != 1 {
		t.Errorf("RunningCount = %d, want 1", s.RunningCount())
	}

	s.RemoveRunning(42)
	if s.GetRunning(42) != nil {
		t.Error("GetRunning after remove should return nil")
	}
}

func TestStateSlotsAvailable(t *testing.T) {
	s := NewState()

	if !s.SlotsAvailable(3) {
		t.Error("should have slots when empty")
	}

	for i := 0; i < 3; i++ {
		s.AddRunning(i, &domain.RunEntry{})
	}
	if s.SlotsAvailable(3) {
		t.Error("should not have slots when full")
	}
}

func TestStateAllRunning(t *testing.T) {
	s := NewState()
	s.AddRunning(1, &domain.RunEntry{Issue: domain.Issue{ID: 1}})
	s.AddRunning(2, &domain.RunEntry{Issue: domain.Issue{ID: 2}})

	all := s.AllRunning()
	if len(all) != 2 {
		t.Errorf("AllRunning len = %d, want 2", len(all))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/...`
Expected: FAIL — `NewState` not defined.

**Step 3: Write minimal implementation**

```go
// internal/orchestrator/state.go
package orchestrator

import (
	"sync"

	"github.com/bketelsen/gopilot/internal/domain"
)

// State manages the orchestrator's runtime state. Thread-safe.
type State struct {
	mu       sync.RWMutex
	running  map[int]*domain.RunEntry
	claimed  map[int]bool
	retry    map[int]*domain.RetryEntry
	totals   domain.TokenTotals
}

// NewState creates an empty state.
func NewState() *State {
	return &State{
		running: make(map[int]*domain.RunEntry),
		claimed: make(map[int]bool),
		retry:   make(map[int]*domain.RetryEntry),
	}
}

func (s *State) Claim(issueID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed[issueID] {
		return false
	}
	s.claimed[issueID] = true
	return true
}

func (s *State) Release(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claimed, issueID)
}

func (s *State) IsClaimed(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.claimed[issueID]
}

func (s *State) AddRunning(issueID int, entry *domain.RunEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running[issueID] = entry
}

func (s *State) RemoveRunning(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, issueID)
}

func (s *State) GetRunning(issueID int) *domain.RunEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running[issueID]
}

func (s *State) RunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running)
}

func (s *State) SlotsAvailable(max int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running) < max
}

func (s *State) AllRunning() []*domain.RunEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.RunEntry, 0, len(s.running))
	for _, e := range s.running {
		entries = append(entries, e)
	}
	return entries
}

func (s *State) AddRetry(issueID int, entry *domain.RetryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retry[issueID] = entry
}

func (s *State) RemoveRetry(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.retry, issueID)
}

func (s *State) AllRetries() []*domain.RetryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.RetryEntry, 0, len(s.retry))
	for _, e := range s.retry {
		entries = append(entries, e)
	}
	return entries
}

func (s *State) IsInRetryQueue(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.retry[issueID]
	return ok
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat: thread-safe orchestrator state management"
```

---

### Task 1.11: Orchestrator Core Loop

**Files:**
- Create: `internal/orchestrator/orchestrator.go`
- Test: `internal/orchestrator/orchestrator_test.go`

This is the most complex task. The orchestrator ties everything together.

**Step 1: Write the failing test**

```go
// internal/orchestrator/orchestrator_test.go
package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// mockGitHub implements github.Client for testing.
type mockGitHub struct {
	issues []domain.Issue
}

func (m *mockGitHub) FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error) {
	return m.issues, nil
}
func (m *mockGitHub) FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error) {
	for _, i := range m.issues {
		if i.ID == id && i.Repo == repo {
			return &i, nil
		}
	}
	return nil, nil
}
func (m *mockGitHub) FetchIssueStates(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	return m.issues, nil
}
func (m *mockGitHub) SetProjectStatus(ctx context.Context, issue domain.Issue, status string) error {
	return nil
}
func (m *mockGitHub) AddComment(ctx context.Context, repo string, id int, body string) error {
	return nil
}
func (m *mockGitHub) AddLabel(ctx context.Context, repo string, id int, label string) error {
	return nil
}

// mockAgent implements agent.Runner for testing.
type mockAgent struct {
	started int
}

func (m *mockAgent) Name() string { return "mock" }
func (m *mockAgent) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	m.started++
	done := make(chan struct{})
	// Simulate long-running agent
	go func() {
		<-ctx.Done()
		close(done)
	}()
	return &agent.Session{
		ID:   "mock-session",
		PID:  12345,
		Done: done,
		Cancel: func() {},
	}, nil
}
func (m *mockAgent) Stop(sess *agent.Session) error {
	sess.Cancel()
	return nil
}

func TestOrchestratorDispatch(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token:          "tok",
			Repos:          []string{"o/r"},
			EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{
			IntervalMS:          1000,
			MaxConcurrentAgents: 2,
		},
		Agent: config.AgentConfig{
			Command:              "mock",
			TurnTimeoutMS:        60000,
			StallTimeoutMS:       60000,
			MaxRetries:           3,
			MaxRetryBackoffMS:    1000,
			MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{
			Root:          t.TempDir(),
			HookTimeoutMS: 5000,
		},
		Prompt: "Do work on {{ .Issue.Title }}",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Title: "Fix bug", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
			{ID: 2, Repo: "o/r", Title: "Add feature", Labels: []string{"gopilot"}, Status: "Todo", Priority: 2, CreatedAt: time.Now()},
		},
	}
	ag := &mockAgent{}

	orch := NewOrchestrator(cfg, gh, ag)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run one tick
	orch.Tick(ctx)

	if ag.started != 2 {
		t.Errorf("started = %d, want 2", ag.started)
	}
	if orch.state.RunningCount() != 2 {
		t.Errorf("running = %d, want 2", orch.state.RunningCount())
	}
}

func TestOrchestratorRespectsMaxConcurrency(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token:          "tok",
			Repos:          []string{"o/r"},
			EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{
			IntervalMS:          1000,
			MaxConcurrentAgents: 1, // Only 1 slot
		},
		Agent: config.AgentConfig{
			Command:              "mock",
			TurnTimeoutMS:        60000,
			StallTimeoutMS:       60000,
			MaxRetries:           3,
			MaxRetryBackoffMS:    1000,
			MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{
			Root:          t.TempDir(),
			HookTimeoutMS: 5000,
		},
		Prompt: "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
			{ID: 2, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 2, CreatedAt: time.Now()},
		},
	}
	ag := &mockAgent{}

	orch := NewOrchestrator(cfg, gh, ag)

	ctx := context.Background()
	orch.Tick(ctx)

	if ag.started != 1 {
		t.Errorf("started = %d, want 1 (max concurrency)", ag.started)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/orchestrator/...`
Expected: FAIL — `NewOrchestrator`, `Tick` not defined.

**Step 3: Write minimal implementation**

```go
// internal/orchestrator/orchestrator.go
package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	gh "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/prompt"
	"github.com/bketelsen/gopilot/internal/workspace"
)

// Orchestrator runs the poll-dispatch-reconcile loop.
type Orchestrator struct {
	cfg       *config.Config
	github    gh.Client
	agent     agent.Runner
	workspace workspace.Manager
	state     *State
}

// NewOrchestrator creates a new orchestrator.
func NewOrchestrator(cfg *config.Config, github gh.Client, agentRunner agent.Runner) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		github:    github,
		agent:     agentRunner,
		workspace: workspace.NewFSManager(cfg.Workspace),
		state:     NewState(),
	}
}

// Run starts the main loop until context is canceled.
func (o *Orchestrator) Run(ctx context.Context) error {
	slog.Info("orchestrator started",
		"poll_interval", o.cfg.PollInterval(),
		"max_agents", o.cfg.Polling.MaxConcurrentAgents,
	)

	ticker := time.NewTicker(o.cfg.PollInterval())
	defer ticker.Stop()

	// Initial tick
	o.Tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("orchestrator shutting down")
			o.shutdown()
			return nil
		case <-ticker.C:
			o.Tick(ctx)
		}
	}
}

// DryRun fetches and displays eligible issues without dispatching.
func (o *Orchestrator) DryRun(ctx context.Context) error {
	issues, err := o.github.FetchCandidateIssues(ctx)
	if err != nil {
		return err
	}
	domain.SortByPriority(issues)

	slog.Info("dry run", "eligible_issues", len(issues))
	for _, issue := range issues {
		slog.Info("eligible",
			"issue", issue.Identifier(),
			"title", issue.Title,
			"priority", issue.Priority,
			"status", issue.Status,
		)
	}
	return nil
}

// Tick runs one iteration of the poll-dispatch-reconcile loop.
func (o *Orchestrator) Tick(ctx context.Context) {
	// 1. FETCH CANDIDATES
	issues, err := o.github.FetchCandidateIssues(ctx)
	if err != nil {
		slog.Error("failed to fetch candidates", "error", err)
		return
	}

	// 2. FILTER — exclude already running, claimed, or in retry queue
	var candidates []domain.Issue
	for _, issue := range issues {
		if o.state.IsClaimed(issue.ID) || o.state.GetRunning(issue.ID) != nil || o.state.IsInRetryQueue(issue.ID) {
			continue
		}
		candidates = append(candidates, issue)
	}

	// 3. SORT
	domain.SortByPriority(candidates)

	// 4. DISPATCH
	for _, issue := range candidates {
		if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
			break
		}
		o.dispatch(ctx, issue, 1)
	}
}

func (o *Orchestrator) dispatch(ctx context.Context, issue domain.Issue, attempt int) {
	if !o.state.Claim(issue.ID) {
		return // already claimed
	}

	log := slog.With("issue", issue.Identifier(), "attempt", attempt)

	// Ensure workspace
	wsPath, err := o.workspace.Ensure(ctx, issue)
	if err != nil {
		log.Error("workspace ensure failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	// Run before_run hook
	if err := o.workspace.RunHook(ctx, "before_run", wsPath, issue); err != nil {
		log.Error("before_run hook failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	// Render prompt
	rendered, err := prompt.Render(o.cfg.Prompt, issue, attempt, "")
	if err != nil {
		log.Error("prompt render failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	// Set status to "In Progress"
	if err := o.github.SetProjectStatus(ctx, issue, "In Progress"); err != nil {
		log.Warn("failed to set status to In Progress", "error", err)
		// Non-fatal — continue dispatching
	}

	// Launch agent
	opts := agent.AgentOpts{
		Model:            o.cfg.Agent.Model,
		MaxContinuations: o.cfg.Agent.MaxAutopilotContinues,
	}
	sess, err := o.agent.Start(ctx, wsPath, rendered, opts)
	if err != nil {
		log.Error("agent start failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	now := time.Now()
	entry := &domain.RunEntry{
		Issue:       issue,
		SessionID:   sess.ID,
		ProcessPID:  sess.PID,
		StartedAt:   now,
		LastEventAt: now,
		Attempt:     attempt,
	}
	o.state.AddRunning(issue.ID, entry)

	log.Info("agent dispatched",
		"session_id", sess.ID,
		"pid", sess.PID,
		"workspace", wsPath,
	)

	// Monitor agent in background
	go o.monitorAgent(issue, sess, entry)
}

func (o *Orchestrator) monitorAgent(issue domain.Issue, sess *agent.Session, entry *domain.RunEntry) {
	<-sess.Done

	log := slog.With("issue", issue.Identifier(), "session_id", sess.ID)

	o.state.RemoveRunning(issue.ID)

	if sess.ExitCode == 0 {
		log.Info("agent completed successfully")
		o.state.Release(issue.ID)
	} else {
		log.Warn("agent exited with error", "exit_code", sess.ExitCode, "error", sess.ExitErr)
		o.state.Release(issue.ID)
		// Retry logic added in Phase 2
	}
}

func (o *Orchestrator) shutdown() {
	for _, entry := range o.state.AllRunning() {
		slog.Info("stopping agent", "issue", entry.Issue.Identifier())
		// In Phase 2, we'll properly stop agents
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/ internal/prompt/ internal/workspace/
git commit -m "feat: orchestrator core loop with poll-dispatch lifecycle"
```

---

### Task 1.12: Rewrite CLI main.go

**Files:**
- Modify: `cmd/gopilot/main.go`

**Step 1: Rewrite main.go to use new packages**

```go
// cmd/gopilot/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	ghclient "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/logging"
	"github.com/bketelsen/gopilot/internal/orchestrator"
)

var Version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Printf("gopilot %s\n", Version)
			return
		case "init":
			runInit()
			return
		}
	}

	configPath := flag.String("config", "gopilot.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "list eligible issues without dispatching")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logging.Setup(level)

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}

	// Create GitHub client
	restClient := ghclient.NewRESTClient(cfg.GitHub, "https://api.github.com/")

	// Create agent runner
	agentRunner := &agent.CopilotRunner{
		Command: cfg.Agent.Command,
		Token:   cfg.GitHub.Token,
	}

	// Create orchestrator
	orch := orchestrator.NewOrchestrator(cfg, restClient, agentRunner)

	// Signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	if *dryRun {
		if err := orch.DryRun(ctx); err != nil {
			slog.Error("dry run failed", "error", err)
			os.Exit(1)
		}
		return
	}

	slog.Info("starting gopilot", "version", Version)
	if err := orch.Run(ctx); err != nil {
		slog.Error("orchestrator exited with error", "error", err)
		os.Exit(1)
	}
}

func runInit() {
	path := "gopilot.yaml"
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists\n", path)
		os.Exit(1)
	}
	if err := os.WriteFile(path, []byte(config.ExampleConfig), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("Created %s — edit it with your GitHub token and repos.\n", path)
}
```

**Step 2: Verify compilation**

Run: `go build ./cmd/gopilot/`
Expected: BUILD SUCCESS

**Step 3: Run all tests**

Run: `go test -race ./...`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add cmd/gopilot/main.go
git commit -m "feat: rewrite CLI entry point for new package structure"
```

---

## Phase 1 Milestone

Run: `go test -race ./...` — all tests pass.
Run: `go build -o gopilot ./cmd/gopilot/` — binary builds.
Run: `./gopilot version` — prints version.
Run: `./gopilot init` — creates starter config.

The orchestrator can:
- Load and validate `gopilot.yaml`
- Fetch eligible issues from GitHub REST API
- Sort by priority
- Create per-issue workspaces with hooks
- Render prompts with issue data
- Launch Copilot CLI agents as subprocesses
- Track running agents in thread-safe state
- Handle graceful shutdown on SIGINT/SIGTERM

Not yet: retry queue, stall detection, reconciliation, dashboard, skills.
