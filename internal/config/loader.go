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
