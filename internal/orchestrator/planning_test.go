package orchestrator

import (
	"context"
	"testing"
	"time"

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
