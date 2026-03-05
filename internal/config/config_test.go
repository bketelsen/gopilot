package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")

	yaml := `
github:
  token: "test-token-123"
  repos:
    - "owner/repo"
  project:
    owner: "@me"
    number: 1
  eligible_labels:
    - "gopilot"
workspace:
  root: "/tmp/workspaces"
prompt: "Fix issue {{.Issue.Title}}"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.GitHub.Token != "test-token-123" {
		t.Errorf("token = %q, want %q", cfg.GitHub.Token, "test-token-123")
	}
	if cfg.Polling.IntervalMS != 30000 {
		t.Errorf("interval = %d, want %d", cfg.Polling.IntervalMS, 30000)
	}
	if cfg.Polling.MaxConcurrent != 3 {
		t.Errorf("max_concurrent = %d, want %d", cfg.Polling.MaxConcurrent, 3)
	}
	if cfg.Agent.Command != "copilot" {
		t.Errorf("agent command = %q, want %q", cfg.Agent.Command, "copilot")
	}
	if cfg.Agent.MaxAutopilotContinues != 20 {
		t.Errorf("max_autopilot_continues = %d, want %d", cfg.Agent.MaxAutopilotContinues, 20)
	}
}

func TestLoadEnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")

	t.Setenv("TEST_GH_TOKEN", "env-token-456")

	yaml := `
github:
  token: "$TEST_GH_TOKEN"
  repos:
    - "owner/repo"
  project:
    owner: "@me"
    number: 1
  eligible_labels:
    - "gopilot"
workspace:
  root: "/tmp/workspaces"
prompt: "Fix it"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.GitHub.Token != "env-token-456" {
		t.Errorf("token = %q, want %q", cfg.GitHub.Token, "env-token-456")
	}
}

func TestLoadMissingToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")

	// Unset any token env vars
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	yaml := `
github:
  repos:
    - "owner/repo"
  project:
    owner: "@me"
    number: 1
  eligible_labels:
    - "gopilot"
workspace:
  root: "/tmp/workspaces"
prompt: "Fix it"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestLoadMissingRepos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")

	yaml := `
github:
  token: "tok"
  project:
    owner: "@me"
    number: 1
  eligible_labels:
    - "gopilot"
workspace:
  root: "/tmp/workspaces"
prompt: "Fix it"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing repos")
	}
}
