package orchestrator

import (
	"context"
	"strings"
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

func TestIsBotComment(t *testing.T) {
	if !isBotComment(domain.Comment{Author: "gopilot[bot]", Body: "hello"}) {
		t.Error("gopilot[bot] should be a bot")
	}
	if isBotComment(domain.Comment{Author: "alice", Body: "hello"}) {
		t.Error("alice with no marker should not be a bot")
	}
	// Comment with planning marker should be treated as bot
	if !isBotComment(domain.Comment{Author: "bketelsen", Body: "some text\n" + PlanningCommentMarker}) {
		t.Error("comment with planning marker should be treated as bot")
	}
	// Human comment from same user without marker is NOT bot
	if isBotComment(domain.Comment{Author: "bketelsen", Body: "please add OAuth"}) {
		t.Error("human comment without marker should not be treated as bot")
	}
}

// commentCapturingGitHub wraps mockGitHub to capture AddComment calls.
type commentCapturingGitHub struct {
	mockGitHub
	addedComments []capturedComment
}

type capturedComment struct {
	Repo string
	ID   int
	Body string
}

func (m *commentCapturingGitHub) AddComment(ctx context.Context, repo string, id int, body string) error {
	m.addedComments = append(m.addedComments, capturedComment{Repo: repo, ID: id, Body: body})
	return nil
}

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
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
			ApproveCommand: "/approve", MaxQuestions: 10,
		},
		Dashboard: config.DashboardConfig{Addr: ":4000"},
		Prompt:    "Work",
	}
}

func TestProcessPlanningIssuesPostsRedirect(t *testing.T) {
	cfg := newTestConfig(t)

	planningIssue := domain.Issue{
		ID: 1, Repo: "o/r", Title: "Plan: Auth system",
		Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo",
		Body: "We need auth", CreatedAt: time.Now(),
	}

	gh := &commentCapturingGitHub{
		mockGitHub: mockGitHub{issues: []domain.Issue{planningIssue}},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	ctx := context.Background()
	orch.processPlanningIssues(ctx, []domain.Issue{planningIssue})

	// Should have posted exactly one comment.
	if len(gh.addedComments) != 1 {
		t.Fatalf("addedComments = %d, want 1", len(gh.addedComments))
	}

	comment := gh.addedComments[0]
	if comment.Repo != "o/r" {
		t.Errorf("comment repo = %q, want %q", comment.Repo, "o/r")
	}
	if comment.ID != 1 {
		t.Errorf("comment ID = %d, want 1", comment.ID)
	}
	if !strings.Contains(comment.Body, "http://localhost:4000/planning/new") {
		t.Errorf("comment body missing dashboard URL: %s", comment.Body)
	}
	if !strings.Contains(comment.Body, PlanningCommentMarker) {
		t.Errorf("comment body missing planning marker")
	}

	// Issue should be in planning state with PlanningPhaseComplete.
	if !orch.state.IsPlanning(1) {
		t.Error("issue 1 should be in planning state")
	}
	entry := orch.state.GetPlanning(1)
	if entry == nil {
		t.Fatal("planning entry is nil")
	}
	if entry.Phase != PlanningPhaseComplete {
		t.Errorf("phase = %q, want %q", entry.Phase, PlanningPhaseComplete)
	}

	// No agents should have been started.
	if ag.started != 0 {
		t.Errorf("agents started = %d, want 0", ag.started)
	}
}

func TestProcessPlanningIssuesSkipsAlreadyRedirected(t *testing.T) {
	cfg := newTestConfig(t)

	planningIssue := domain.Issue{
		ID: 1, Repo: "o/r", Title: "Plan: Auth system",
		Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo",
		Body: "We need auth", CreatedAt: time.Now(),
	}

	gh := &commentCapturingGitHub{
		mockGitHub: mockGitHub{issues: []domain.Issue{planningIssue}},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	// Pre-populate planning state to simulate already-redirected issue.
	orch.state.AddPlanning(1, &PlanningEntry{
		IssueID: 1,
		Repo:    "o/r",
		Phase:   PlanningPhaseComplete,
	})

	ctx := context.Background()
	orch.processPlanningIssues(ctx, []domain.Issue{planningIssue})

	// Should NOT have posted any comments.
	if len(gh.addedComments) != 0 {
		t.Errorf("addedComments = %d, want 0 (already redirected)", len(gh.addedComments))
	}
}

func TestProcessPlanningIssuesDefaultAddr(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Dashboard.Addr = "" // empty — should default to :3000

	planningIssue := domain.Issue{
		ID: 5, Repo: "o/r", Title: "Plan: something",
		Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo",
		CreatedAt: time.Now(),
	}

	gh := &commentCapturingGitHub{
		mockGitHub: mockGitHub{issues: []domain.Issue{planningIssue}},
	}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": &mockAgent{}})

	orch.processPlanningIssues(context.Background(), []domain.Issue{planningIssue})

	if len(gh.addedComments) != 1 {
		t.Fatalf("addedComments = %d, want 1", len(gh.addedComments))
	}
	if !strings.Contains(gh.addedComments[0].Body, "http://localhost:3000/planning/new") {
		t.Errorf("expected default :3000 addr in comment body: %s", gh.addedComments[0].Body)
	}
}

func TestOrchestratorPartitionsPlanningIssues(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Planning.Agent = "mock"

	gh := &commentCapturingGitHub{
		mockGitHub: mockGitHub{
			issues: []domain.Issue{
				{ID: 1, Repo: "o/r", Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
				{ID: 2, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
			},
		},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Issue 1 should be in planning state (redirect posted).
	if !orch.state.IsPlanning(1) {
		t.Error("issue 1 should be in planning state")
	}

	// A redirect comment should have been posted for issue 1.
	foundRedirect := false
	for _, c := range gh.addedComments {
		if c.ID == 1 && strings.Contains(c.Body, PlanningCommentMarker) {
			foundRedirect = true
		}
	}
	if !foundRedirect {
		t.Error("expected redirect comment for planning issue 1")
	}

	// Wait for mock agent to register (coding issue 2).
	time.Sleep(50 * time.Millisecond)

	// Only coding issue 2 should have been dispatched to an agent.
	if ag.started != 1 {
		t.Errorf("agents started = %d, want 1 (only coding issue)", ag.started)
	}
}
