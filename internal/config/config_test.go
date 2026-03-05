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
