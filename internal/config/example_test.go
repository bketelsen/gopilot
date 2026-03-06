package config_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bketelsen/gopilot/internal/config"
)

func ExampleLoad() {
	yaml := `github:
  token: ghp_example_token
  repos: [owner/my-repo]
  eligible_labels: [gopilot]
agent:
  command: copilot
workspace:
  root: /tmp/workspaces`

	dir, _ := os.MkdirTemp("", "example-config")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := config.Load(path)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("repos:", cfg.GitHub.Repos)
	fmt.Println("agent:", cfg.Agent.Command)
	fmt.Println("max_retries:", cfg.Agent.MaxRetries)
	// Output:
	// repos: [owner/my-repo]
	// agent: copilot
	// max_retries: 3
}

func ExampleConfig_AgentCommandForIssue() {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			Command: "copilot",
			Overrides: []config.AgentOverride{
				{Repos: []string{"owner/special-repo"}, Command: "claude"},
				{Labels: []string{"use-claude"}, Command: "claude"},
			},
		},
	}

	fmt.Println(cfg.AgentCommandForIssue("owner/normal-repo", nil))
	fmt.Println(cfg.AgentCommandForIssue("owner/special-repo", nil))
	fmt.Println(cfg.AgentCommandForIssue("owner/normal-repo", []string{"use-claude"}))
	// Output:
	// copilot
	// claude
	// claude
}
