package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
	// Defaults should be applied for unset fields
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

func TestPlanningConfigDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()

	if cfg.Planning.Label != "gopilot:plan" {
		t.Errorf("Planning.Label = %q, want %q", cfg.Planning.Label, "gopilot:plan")
	}
	if cfg.Planning.CompletedLabel != "gopilot:planned" {
		t.Errorf("Planning.CompletedLabel = %q, want %q", cfg.Planning.CompletedLabel, "gopilot:planned")
	}
	if cfg.Planning.ApproveCommand != "/approve" {
		t.Errorf("Planning.ApproveCommand = %q, want %q", cfg.Planning.ApproveCommand, "/approve")
	}
	if cfg.Planning.MaxQuestions != 10 {
		t.Errorf("Planning.MaxQuestions = %d, want 10", cfg.Planning.MaxQuestions)
	}
}

func TestPlanningConfigFromYAML(t *testing.T) {
	yaml := `
github:
  token: tok
  repos: [o/r]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
planning:
  label: "custom:plan"
  agent: "claude"
  model: "claude-sonnet-4-6"
  max_questions: 5
`
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Planning.Label != "custom:plan" {
		t.Errorf("Planning.Label = %q, want %q", cfg.Planning.Label, "custom:plan")
	}
	if cfg.Planning.Agent != "claude" {
		t.Errorf("Planning.Agent = %q, want %q", cfg.Planning.Agent, "claude")
	}
	if cfg.Planning.Model != "claude-sonnet-4-6" {
		t.Errorf("Planning.Model = %q, want %q", cfg.Planning.Model, "claude-sonnet-4-6")
	}
	if cfg.Planning.MaxQuestions != 5 {
		t.Errorf("Planning.MaxQuestions = %d, want 5", cfg.Planning.MaxQuestions)
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
