package github

import (
	"context"

	"github.com/bketelsen/gopilot/internal/domain"
)

// Client defines the GitHub operations the orchestrator needs.
type Client interface {
	FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error)
	FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error)
	FetchIssueStates(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error)
	SetProjectStatus(ctx context.Context, issue domain.Issue, status string) error
	AddComment(ctx context.Context, repo string, id int, body string) error
	AddLabel(ctx context.Context, repo string, id int, label string) error
	EnrichIssues(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error)
	FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error)
	RemoveLabel(ctx context.Context, repo string, id int, label string) error
	CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*domain.Issue, error)
	AddSubIssue(ctx context.Context, repo string, parentID, childID int) error
}
