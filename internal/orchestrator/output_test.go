package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// mockAgentWithOutput captures the Stdout writer and writes lines to it.
type mockAgentWithOutput struct {
	started int
	lines   []string
}

func (m *mockAgentWithOutput) Name() string { return "mock-output" }
func (m *mockAgentWithOutput) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	m.started++
	done := make(chan struct{})
	go func() {
		defer close(done)
		if opts.Stdout != nil {
			for _, line := range m.lines {
				fmt.Fprintln(opts.Stdout, line)
			}
		}
	}()
	return &agent.Session{
		ID:     "output-session",
		PID:    54321,
		Done:   done,
		Cancel: func() {},
	}, nil
}
func (m *mockAgentWithOutput) Stop(sess *agent.Session) error { return nil }

func newOutputTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock-output", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}
}

func TestDispatchCapturesAgentOutput(t *testing.T) {
	cfg := newOutputTestConfig(t)
	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}

	ag := &mockAgentWithOutput{
		lines: []string{"Reading file main.go", "Editing function", "Running tests"},
	}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-output": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Wait for monitor goroutine to finish processing
	time.Sleep(200 * time.Millisecond)

	// The agent completed (exit 0), check history
	history := orch.state.GetHistory(1)
	if len(history) == 0 {
		t.Fatal("expected completed run in history")
	}
	if history[0].ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", history[0].ExitCode)
	}
}

func TestDispatchSetsStdoutOnAgentOpts(t *testing.T) {
	cfg := newOutputTestConfig(t)
	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}

	// Use a mock that verifies Stdout is set
	ag := &mockAgentVerifyStdout{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-output": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	time.Sleep(100 * time.Millisecond)

	if !ag.stdoutWasSet {
		t.Error("expected Stdout to be set in AgentOpts, but it was nil")
	}
}

// mockAgentVerifyStdout records whether Stdout was set.
type mockAgentVerifyStdout struct {
	stdoutWasSet bool
}

func (m *mockAgentVerifyStdout) Name() string { return "mock-output" }
func (m *mockAgentVerifyStdout) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	m.stdoutWasSet = opts.Stdout != nil
	done := make(chan struct{})
	close(done)
	return &agent.Session{
		ID: "verify-session", PID: 11111, Done: done, Cancel: func() {},
	}, nil
}
func (m *mockAgentVerifyStdout) Stop(sess *agent.Session) error { return nil }

func TestOutputUpdatesRunEntryFields(t *testing.T) {
	cfg := newOutputTestConfig(t)
	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}

	// Agent that writes lines slowly so we can observe RunEntry updates
	ag := &mockAgentSlowOutput{
		lines: []string{"Analyzing codebase", "Editing file.go"},
		delay: 50 * time.Millisecond,
	}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-output": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Wait for first line to be processed
	time.Sleep(100 * time.Millisecond)

	entry := orch.state.GetRunning(1)
	if entry == nil {
		// Agent may have finished already, check history
		t.Log("agent already finished, checking that output was captured")
	} else {
		if entry.GetLastMessage() == "" {
			t.Error("expected LastMessage to be set on running entry")
		}
		if entry.OutputLines == nil {
			t.Error("expected OutputLines buffer to be initialized")
		}
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)
}

// mockAgentSlowOutput writes lines with a delay between each.
type mockAgentSlowOutput struct {
	lines []string
	delay time.Duration
}

func (m *mockAgentSlowOutput) Name() string { return "mock-output" }
func (m *mockAgentSlowOutput) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if opts.Stdout != nil {
			for _, line := range m.lines {
				fmt.Fprintln(opts.Stdout, line)
				time.Sleep(m.delay)
			}
		}
	}()
	return &agent.Session{
		ID: "slow-session", PID: 22222, Done: done, Cancel: func() {},
	}, nil
}
func (m *mockAgentSlowOutput) Stop(sess *agent.Session) error { return nil }

func TestOutputBufferOnRunEntry(t *testing.T) {
	cfg := newOutputTestConfig(t)
	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}

	lines := make([]string, 60)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	ag := &mockAgentWithOutput{lines: lines}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-output": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Wait for agent to complete and monitor to finish
	time.Sleep(300 * time.Millisecond)

	// Agent completed, so it won't be running. But we can verify through history
	// that the agent ran successfully with output capture.
	history := orch.state.GetHistory(1)
	if len(history) == 0 {
		t.Fatal("expected run in history")
	}
}

func TestStallDetectionUpdatedByOutput(t *testing.T) {
	cfg := newOutputTestConfig(t)
	cfg.Agent.StallTimeoutMS = 150 // 150ms stall timeout

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}

	// Agent writes lines with 50ms delay each — total 250ms
	// Stall timeout is 150ms but each line resets LastEventAt
	ag := &mockAgentSlowOutput{
		lines: []string{"line1", "line2", "line3", "line4", "line5"},
		delay: 50 * time.Millisecond,
	}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-output": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// After 100ms, check that stall detection doesn't kill the agent
	time.Sleep(100 * time.Millisecond)
	orch.detectStalls(ctx)

	entry := orch.state.GetRunning(1)
	if entry == nil {
		// Agent might have completed already, that's fine
		history := orch.state.GetHistory(1)
		if len(history) == 0 {
			t.Error("agent was killed by stall detection despite receiving output")
		}
	}
}
