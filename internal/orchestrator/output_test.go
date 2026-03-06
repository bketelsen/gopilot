package orchestrator

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// mockAgentCapture records the AgentOpts passed to Start.
type mockAgentCapture struct {
	opts    agent.AgentOpts
	started int
}

func (m *mockAgentCapture) Name() string { return "mock-capture" }
func (m *mockAgentCapture) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	m.opts = opts
	m.started++

	// Write some output to the provided stdout writer to simulate agent output.
	if opts.Stdout != nil {
		go func() {
			opts.Stdout.Write([]byte("Thinking about the problem...\n"))
			opts.Stdout.Write([]byte("Reading file main.go\n"))
			opts.Stdout.Write([]byte("Editing file main.go\n"))
		}()
	}

	done := make(chan struct{})
	go func() {
		// Wait a bit to let output be captured, then exit.
		time.Sleep(200 * time.Millisecond)
		close(done)
	}()
	return &agent.Session{
		ID:     "capture-session",
		PID:    54321,
		Done:   done,
		Cancel: func() {},
	}, nil
}
func (m *mockAgentCapture) Stop(sess *agent.Session) error {
	sess.Cancel()
	return nil
}

func TestDispatchSetsStdout(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock-capture", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}
	ag := &mockAgentCapture{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-capture": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	if ag.started != 1 {
		t.Fatalf("started = %d, want 1", ag.started)
	}
	if ag.opts.Stdout == nil {
		t.Fatal("Stdout was not set on AgentOpts")
	}
}

func TestAgentOutputCapturedInRunEntry(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock-capture", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}
	ag := &mockAgentCapture{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-capture": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Wait for output to be processed.
	time.Sleep(150 * time.Millisecond)

	entry := orch.state.GetRunning(1)
	if entry == nil {
		t.Fatal("expected running entry for issue 1")
	}

	if entry.GetLastMessage() == "" {
		t.Error("LastMessage should be populated from agent output")
	}
	if entry.OutputBuffer == nil {
		t.Fatal("OutputBuffer should be set")
	}
	lines := entry.OutputBuffer.Lines()
	if len(lines) == 0 {
		t.Error("OutputBuffer should have captured lines")
	}
}

func TestRunEntryFieldsUpdatedDuringOutput(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock-capture", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}
	ag := &mockAgentCapture{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-capture": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Wait for output to be processed.
	time.Sleep(150 * time.Millisecond)

	entry := orch.state.GetRunning(1)
	if entry == nil {
		t.Fatal("expected running entry for issue 1")
	}

	// LastEventAt should have been updated recently (use IsStalled as a proxy).
	if entry.IsStalled(time.Second) {
		t.Error("LastEventAt should have been updated from agent output")
	}

	// TurnCount should be > 0.
	if entry.GetTurnCount() == 0 {
		t.Error("TurnCount should be incremented on output")
	}
}

func TestStallDetectionResetsWithOutput(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock-stall", TurnTimeoutMS: 60000,
			StallTimeoutMS: 100, // 100ms stall timeout
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1},
		},
	}

	// This agent writes output slowly enough that stall detection won't trigger.
	ag := &mockAgentSlowOutput{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock-stall": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Wait for some output to be processed.
	time.Sleep(150 * time.Millisecond)

	// Stall detection should NOT kill the agent because output keeps updating LastEventAt.
	orch.detectStalls(ctx)

	if orch.state.RunningCount() != 1 {
		t.Errorf("running = %d, want 1 (agent should not be stalled - it has recent output)", orch.state.RunningCount())
	}
}

// mockAgentSlowOutput writes output lines periodically.
type mockAgentSlowOutput struct{}

func (m *mockAgentSlowOutput) Name() string { return "mock-stall" }
func (m *mockAgentSlowOutput) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	done := make(chan struct{})
	if opts.Stdout != nil {
		go func() {
			for i := 0; i < 20; i++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				opts.Stdout.Write([]byte("working...\n"))
				time.Sleep(50 * time.Millisecond)
			}
		}()
	}
	go func() {
		<-ctx.Done()
		close(done)
	}()
	return &agent.Session{
		ID: "slow-session", PID: 11111, Done: done,
		Cancel: func() {},
	}, nil
}
func (m *mockAgentSlowOutput) Stop(sess *agent.Session) error {
	sess.Cancel()
	return nil
}

// mockAgentWithPipe lets us control the stdout writer for testing the pipe is passed.
type mockAgentWithPipe struct {
	stdout io.Writer
}

func (m *mockAgentWithPipe) Name() string { return "mock-pipe" }
func (m *mockAgentWithPipe) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	m.stdout = opts.Stdout
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(done)
	}()
	return &agent.Session{
		ID: "pipe-session", PID: 22222, Done: done,
		Cancel: func() {},
	}, nil
}
func (m *mockAgentWithPipe) Stop(sess *agent.Session) error {
	sess.Cancel()
	return nil
}
