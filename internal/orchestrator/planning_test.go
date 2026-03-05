package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

func TestPartitionPlanningIssues(t *testing.T) {
	issues := []domain.Issue{
		{ID: 1, Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo"},
		{ID: 2, Labels: []string{"gopilot"}, Status: "Todo"},
		{ID: 3, Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo"},
	}

	planning, coding := partitionPlanningIssues(issues, "gopilot:plan")

	if len(planning) != 2 {
		t.Errorf("planning = %d, want 2", len(planning))
	}
	if len(coding) != 1 {
		t.Errorf("coding = %d, want 1", len(coding))
	}
	if coding[0].ID != 2 {
		t.Errorf("coding[0].ID = %d, want 2", coding[0].ID)
	}
}

func TestHasNewHumanComment(t *testing.T) {
	client := &stubCommentClient{
		comments: []domain.Comment{
			{ID: 100, Author: "gopilot[bot]", Body: "What is the goal?", CreatedAt: time.Now().Add(-time.Minute)},
			{ID: 101, Author: "user", Body: "Build a feature", CreatedAt: time.Now()},
		},
	}

	hasNew, lastID := hasNewHumanComment(client, context.Background(), "o/r", 1, 100)
	if !hasNew {
		t.Error("hasNew = false, want true")
	}
	if lastID != 101 {
		t.Errorf("lastID = %d, want 101", lastID)
	}
}

func TestHasNewHumanCommentBotOnly(t *testing.T) {
	client := &stubCommentClient{
		comments: []domain.Comment{
			{ID: 100, Author: "gopilot[bot]", Body: "What is the goal?", CreatedAt: time.Now()},
		},
	}

	hasNew, _ := hasNewHumanComment(client, context.Background(), "o/r", 1, 100)
	if hasNew {
		t.Error("hasNew = true, want false (only bot comment)")
	}
}

func TestIsBot(t *testing.T) {
	if !isBot("gopilot[bot]") {
		t.Error("gopilot[bot] should be a bot")
	}
	if isBot("alice") {
		t.Error("alice should not be a bot")
	}
}

// stubCommentClient implements just FetchIssueComments for testing.
type stubCommentClient struct {
	comments []domain.Comment
}

func (s *stubCommentClient) FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error) {
	return s.comments, nil
}

// mockPlanningGitHub extends mockGitHub with comment tracking.
type mockPlanningGitHub struct {
	mockGitHub
	comments map[int][]domain.Comment
}

func (m *mockPlanningGitHub) FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error) {
	return m.comments[id], nil
}

func TestOrchestratorPartitionsPlanningIssues(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 5},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Planning: config.PlanningConfig{
			Label: "gopilot:plan", CompletedLabel: "gopilot:planned",
			ApproveCommand: "/approve", MaxQuestions: 10, Agent: "mock",
		},
		Prompt: "Work",
	}

	gh := &mockPlanningGitHub{
		mockGitHub: mockGitHub{
			issues: []domain.Issue{
				{ID: 1, Repo: "o/r", Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
				{ID: 2, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
			},
		},
		comments: map[int][]domain.Comment{},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Issue 1 should be in planning state
	if !orch.state.IsPlanning(1) {
		t.Error("issue 1 should be in planning state")
	}

	// Wait for mock agents to register
	time.Sleep(50 * time.Millisecond)

	// Both should have been dispatched (1 as planning, 2 as coding)
	if ag.started < 2 {
		t.Errorf("agents started = %d, want >= 2", ag.started)
	}
}
