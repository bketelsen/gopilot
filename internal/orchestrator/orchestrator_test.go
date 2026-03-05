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

// mockGitHub implements github.Client for testing.
type mockGitHub struct {
	issues []domain.Issue
}

func (m *mockGitHub) FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error) {
	return m.issues, nil
}
func (m *mockGitHub) FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error) {
	for _, i := range m.issues {
		if i.ID == id && i.Repo == repo {
			return &i, nil
		}
	}
	return nil, nil
}
func (m *mockGitHub) FetchIssueStates(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	return m.issues, nil
}
func (m *mockGitHub) SetProjectStatus(ctx context.Context, issue domain.Issue, status string) error {
	return nil
}
func (m *mockGitHub) AddComment(ctx context.Context, repo string, id int, body string) error {
	return nil
}
func (m *mockGitHub) AddLabel(ctx context.Context, repo string, id int, label string) error {
	return nil
}

// mockAgent implements agent.Runner for testing.
type mockAgent struct {
	started int
}

func (m *mockAgent) Name() string { return "mock" }
func (m *mockAgent) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	m.started++
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(done)
	}()
	return &agent.Session{
		ID:     "mock-session",
		PID:    12345,
		Done:   done,
		Cancel: func() {},
	}, nil
}
func (m *mockAgent) Stop(sess *agent.Session) error {
	sess.Cancel()
	return nil
}

func TestOrchestratorDispatch(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token:          "tok",
			Repos:          []string{"o/r"},
			EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{
			IntervalMS:          1000,
			MaxConcurrentAgents: 2,
		},
		Agent: config.AgentConfig{
			Command:               "mock",
			TurnTimeoutMS:         60000,
			StallTimeoutMS:        60000,
			MaxRetries:            3,
			MaxRetryBackoffMS:     1000,
			MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{
			Root:          t.TempDir(),
			HookTimeoutMS: 5000,
		},
		Prompt: "Do work on {{ .Issue.Title }}",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Title: "Fix bug", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
			{ID: 2, Repo: "o/r", Title: "Add feature", Labels: []string{"gopilot"}, Status: "Todo", Priority: 2, CreatedAt: time.Now()},
		},
	}
	ag := &mockAgent{}

	orch := NewOrchestrator(cfg, gh, ag)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run one tick
	orch.Tick(ctx)

	if ag.started != 2 {
		t.Errorf("started = %d, want 2", ag.started)
	}
	if orch.state.RunningCount() != 2 {
		t.Errorf("running = %d, want 2", orch.state.RunningCount())
	}
}

func TestOrchestratorRespectsMaxConcurrency(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token:          "tok",
			Repos:          []string{"o/r"},
			EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{
			IntervalMS:          1000,
			MaxConcurrentAgents: 1,
		},
		Agent: config.AgentConfig{
			Command:               "mock",
			TurnTimeoutMS:         60000,
			StallTimeoutMS:        60000,
			MaxRetries:            3,
			MaxRetryBackoffMS:     1000,
			MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{
			Root:          t.TempDir(),
			HookTimeoutMS: 5000,
		},
		Prompt: "Work",
	}

	gh := &mockGitHub{
		issues: []domain.Issue{
			{ID: 1, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
			{ID: 2, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 2, CreatedAt: time.Now()},
		},
	}
	ag := &mockAgent{}

	orch := NewOrchestrator(cfg, gh, ag)

	ctx := context.Background()
	orch.Tick(ctx)

	if ag.started != 1 {
		t.Errorf("started = %d, want 1 (max concurrency)", ag.started)
	}
}

type mockFailAgent struct{}

func (m *mockFailAgent) Name() string { return "mock-fail" }
func (m *mockFailAgent) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	done := make(chan struct{})
	close(done)
	return &agent.Session{
		ID: "fail-session", PID: 99999, Done: done,
		ExitCode: 1, ExitErr: fmt.Errorf("crashed"),
		Cancel: func() {},
	}, nil
}
func (m *mockFailAgent) Stop(sess *agent.Session) error { return nil }

func TestOrchestratorRetryOnAgentFailure(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
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

	failAgent := &mockFailAgent{}
	orch := NewOrchestrator(cfg, gh, failAgent)

	ctx := context.Background()
	orch.Tick(ctx)

	time.Sleep(100 * time.Millisecond)

	if orch.retryQueue.Len() != 1 {
		t.Errorf("retry queue len = %d, want 1", orch.retryQueue.Len())
	}
}

func TestOrchestratorStallDetection(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000,
			StallTimeoutMS: 1, // 1ms — everything is stalled immediately
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
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, ag)

	ctx := context.Background()
	orch.Tick(ctx) // dispatch

	time.Sleep(50 * time.Millisecond) // let it become "stalled"

	orch.detectStalls(ctx)

	if orch.state.RunningCount() != 0 {
		t.Errorf("running = %d, want 0 after stall detection", orch.state.RunningCount())
	}
}

// mockGitHubSplit allows separate control of candidate vs state issues.
type mockGitHubSplit struct {
	candidates []domain.Issue
	stateMap   map[int]*domain.Issue
}

func (m *mockGitHubSplit) FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error) {
	return m.candidates, nil
}
func (m *mockGitHubSplit) FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error) {
	if iss, ok := m.stateMap[id]; ok {
		return iss, nil
	}
	return nil, nil
}
func (m *mockGitHubSplit) FetchIssueStates(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	return m.candidates, nil
}
func (m *mockGitHubSplit) SetProjectStatus(ctx context.Context, issue domain.Issue, status string) error {
	return nil
}
func (m *mockGitHubSplit) AddComment(ctx context.Context, repo string, id int, body string) error {
	return nil
}
func (m *mockGitHubSplit) AddLabel(ctx context.Context, repo string, id int, label string) error {
	return nil
}

func TestRetrySkipsIneligibleIssue(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Prompt:    "Work",
	}

	// FetchIssueState returns issue with no eligible labels; FetchCandidateIssues returns nothing
	ineligible := domain.Issue{ID: 1, Repo: "o/r", Labels: []string{}, Status: "Todo", Priority: 1}
	gh := &mockGitHubSplit{
		candidates: nil,
		stateMap:   map[int]*domain.Issue{1: &ineligible},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, ag)

	// Manually enqueue a retry for issue 1
	orch.retryQueue.Enqueue(1, "o/r", "o/r#1", 2, "crashed", time.Second)

	// Set DueAt in the past so it's picked up
	orch.retryQueue.mu.Lock()
	orch.retryQueue.entries[1].DueAt = time.Now().Add(-time.Second)
	orch.retryQueue.mu.Unlock()

	ctx := context.Background()
	orch.Tick(ctx)

	if ag.started != 0 {
		t.Errorf("started = %d, want 0 (ineligible issue should not be retried)", ag.started)
	}
}

func TestReconcileTerminalIssue(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 3},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
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
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, ag)

	ctx := context.Background()
	orch.Tick(ctx) // dispatch issue 1

	// Simulate external state change: issue moved to Done
	gh.issues[0].Status = "Done"

	orch.reconcile(ctx)

	if orch.state.RunningCount() != 0 {
		t.Errorf("running = %d, want 0 after reconciling terminal issue", orch.state.RunningCount())
	}
}
