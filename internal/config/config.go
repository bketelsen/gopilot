package config

import (
	"strings"
	"time"
)

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
	IntervalMS          int `yaml:"interval_ms"`
	MaxConcurrentAgents int `yaml:"max_concurrent_agents"`
}

type WorkspaceConfig struct {
	Root          string      `yaml:"root"`
	HookTimeoutMS int         `yaml:"hook_timeout_ms"`
	Hooks         HooksConfig `yaml:"hooks"`
}

type HooksConfig struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
}

type AgentConfig struct {
	Command               string          `yaml:"command"`
	Model                 string          `yaml:"model"`
	MaxAutopilotContinues int             `yaml:"max_autopilot_continues"`
	TurnTimeoutMS         int             `yaml:"turn_timeout_ms"`
	StallTimeoutMS        int             `yaml:"stall_timeout_ms"`
	MaxRetryBackoffMS     int             `yaml:"max_retry_backoff_ms"`
	MaxRetries            int             `yaml:"max_retries"`
	Overrides             []AgentOverride `yaml:"overrides"`
}

type AgentOverride struct {
	Repos   []string `yaml:"repos"`
	Labels  []string `yaml:"labels"`
	Command string   `yaml:"command"`
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

func (c *Config) PollInterval() time.Duration {
	return time.Duration(c.Polling.IntervalMS) * time.Millisecond
}

func (c *Config) StallTimeout() time.Duration {
	return time.Duration(c.Agent.StallTimeoutMS) * time.Millisecond
}

func (c *Config) TurnTimeout() time.Duration {
	return time.Duration(c.Agent.TurnTimeoutMS) * time.Millisecond
}

func (c *Config) HookTimeout() time.Duration {
	return time.Duration(c.Workspace.HookTimeoutMS) * time.Millisecond
}

// AgentCommandForIssue returns the agent command to use for a given repo and set of labels,
// checking overrides in order and falling back to the default agent command.
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
