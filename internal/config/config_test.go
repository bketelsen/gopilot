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
