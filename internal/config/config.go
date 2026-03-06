package config

import (
	"strings"
	"time"
)

// Config is the top-level gopilot configuration loaded from gopilot.yaml.
type Config struct {
	GitHub    GitHubConfig    `yaml:"github"`
	Polling   PollingConfig   `yaml:"polling"`
	Workspace WorkspaceConfig `yaml:"workspace"`
	Agent     AgentConfig     `yaml:"agent"`
	Skills    SkillsConfig    `yaml:"skills"`
	Dashboard DashboardConfig `yaml:"dashboard"`
	Planning  PlanningConfig  `yaml:"planning"`
	Prompt    string          `yaml:"prompt"`
}

// GitHubConfig holds GitHub authentication, repository list, and label filters.
type GitHubConfig struct {
	Token          string        `yaml:"token"`
	Repos          []string      `yaml:"repos"`
	Project        ProjectConfig `yaml:"project"`
	EligibleLabels []string      `yaml:"eligible_labels"`
	ExcludedLabels []string      `yaml:"excluded_labels"`
}

// ProjectConfig identifies a GitHub Projects v2 board by owner and number.
type ProjectConfig struct {
	Owner  string `yaml:"owner"`
	Number int    `yaml:"number"`
}

// PollingConfig controls the issue polling interval and concurrency limits.
type PollingConfig struct {
	IntervalMS          int `yaml:"interval_ms"`
	MaxConcurrentAgents int `yaml:"max_concurrent_agents"`
}

// WorkspaceConfig defines the workspace root directory, hook timeout, and lifecycle hooks.
type WorkspaceConfig struct {
	Root          string      `yaml:"root"`
	HookTimeoutMS int         `yaml:"hook_timeout_ms"`
	Hooks         HooksConfig `yaml:"hooks"`
}

// HooksConfig specifies shell scripts to run at workspace lifecycle events.
type HooksConfig struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
}

// AgentConfig configures the agent subprocess command, timeouts, and retry behavior.
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

// AgentOverride selects an alternative agent command for specific repos or labels.
type AgentOverride struct {
	Repos   []string `yaml:"repos"`
	Labels  []string `yaml:"labels"`
	Command string   `yaml:"command"`
}

// SkillsConfig specifies the skills directory and which skills to inject into prompts.
type SkillsConfig struct {
	Dir      string   `yaml:"dir"`
	Required []string `yaml:"required"`
	Optional []string `yaml:"optional"`
}

// DashboardConfig controls the web dashboard server.
type DashboardConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Addr        string `yaml:"addr"`
	ExternalURL string `yaml:"external_url"`
}

// PlanningConfig controls the interactive planning workflow labels and limits.
type PlanningConfig struct {
	Label          string `yaml:"label"`
	CompletedLabel string `yaml:"completed_label"`
	ApproveCommand string `yaml:"approve_command"`
	MaxQuestions   int    `yaml:"max_questions"`
	Agent          string `yaml:"agent"`
	Model          string `yaml:"model"`
}

// ApplyDefaults fills in zero-valued fields with sensible defaults.
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
	if c.Planning.Label == "" {
		c.Planning.Label = "gopilot:plan"
	}
	if c.Planning.CompletedLabel == "" {
		c.Planning.CompletedLabel = "gopilot:planned"
	}
	if c.Planning.ApproveCommand == "" {
		c.Planning.ApproveCommand = "/approve"
	}
	if c.Planning.MaxQuestions == 0 {
		c.Planning.MaxQuestions = 10
	}
}

// PollInterval returns the polling interval as a time.Duration.
func (c *Config) PollInterval() time.Duration {
	return time.Duration(c.Polling.IntervalMS) * time.Millisecond
}

// StallTimeout returns the agent stall detection timeout as a time.Duration.
func (c *Config) StallTimeout() time.Duration {
	return time.Duration(c.Agent.StallTimeoutMS) * time.Millisecond
}

// TurnTimeout returns the maximum agent turn duration as a time.Duration.
func (c *Config) TurnTimeout() time.Duration {
	return time.Duration(c.Agent.TurnTimeoutMS) * time.Millisecond
}

// HookTimeout returns the workspace hook execution timeout as a time.Duration.
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
