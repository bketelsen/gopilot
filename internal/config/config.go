package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration parsed from gopilot.yaml.
type Config struct {
	GitHub    GitHubConfig    `yaml:"github"`
	Polling   PollingConfig   `yaml:"polling"`
	Workspace WorkspaceConfig `yaml:"workspace"`
	Agent     AgentConfig     `yaml:"agent"`
	Skills    SkillsConfig    `yaml:"skills"`
	Prompt    string          `yaml:"prompt"`
}

type SkillsConfig struct {
	Dirs    []string `yaml:"dirs"`    // directories to search for skill .md files
	Enabled []string `yaml:"enabled"` // skill names to inject (empty = all)
}

type GitHubConfig struct {
	Token          string        `yaml:"token"`
	Repos          []string      `yaml:"repos"`
	Project        ProjectConfig `yaml:"project"`
	EligibleLabels []string      `yaml:"eligible_labels"`
	ExcludedLabels []string      `yaml:"excluded_labels"`
}

type ProjectConfig struct {
	Owner  string `yaml:"owner"`
	Number int    `yaml:"number"`
}

type PollingConfig struct {
	IntervalMS    int `yaml:"interval_ms"`
	MaxConcurrent int `yaml:"max_concurrent_agents"`
}

type WorkspaceConfig struct {
	Root          string      `yaml:"root"`
	Hooks         HooksConfig `yaml:"hooks"`
	HookTimeoutMS int         `yaml:"hook_timeout_ms"`
}

type HooksConfig struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
}

type AgentConfig struct {
	Command               string `yaml:"command"`
	Model                 string `yaml:"model"`
	MaxAutopilotContinues int    `yaml:"max_autopilot_continues"`
	TurnTimeoutMS         int    `yaml:"turn_timeout_ms"`
	StallTimeoutMS        int    `yaml:"stall_timeout_ms"`
	MaxRetryBackoffMS     int    `yaml:"max_retry_backoff_ms"`
	MaxRetries            int    `yaml:"max_retries"`
}

// Load reads and parses a gopilot.yaml file, resolves env vars, applies defaults.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Expand environment variables in the raw YAML
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(c *Config) {
	if c.Polling.IntervalMS == 0 {
		c.Polling.IntervalMS = 30000
	}
	if c.Polling.MaxConcurrent == 0 {
		c.Polling.MaxConcurrent = 3
	}
	if c.Agent.Command == "" {
		c.Agent.Command = "copilot"
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

	// Resolve token from common env vars if not set
	if c.GitHub.Token == "" {
		if t := os.Getenv("GITHUB_TOKEN"); t != "" {
			c.GitHub.Token = t
		} else if t := os.Getenv("GH_TOKEN"); t != "" {
			c.GitHub.Token = t
		}
	}
}

func validate(c *Config) error {
	if c.GitHub.Token == "" {
		return fmt.Errorf("github.token is required (set in config or via $GITHUB_TOKEN / $GH_TOKEN)")
	}
	if len(c.GitHub.Repos) == 0 {
		return fmt.Errorf("github.repos must have at least one repository")
	}
	if c.GitHub.Project.Number == 0 {
		return fmt.Errorf("github.project.number is required")
	}
	if c.GitHub.Project.Owner == "" {
		return fmt.Errorf("github.project.owner is required")
	}
	if len(c.GitHub.EligibleLabels) == 0 {
		return fmt.Errorf("github.eligible_labels must have at least one label")
	}
	if c.Workspace.Root == "" {
		return fmt.Errorf("workspace.root is required")
	}
	if c.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	return nil
}
