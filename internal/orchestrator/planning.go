package orchestrator

import (
	"context"
	"strings"

	"github.com/bketelsen/gopilot/internal/domain"
)

// commentFetcher is the subset of github.Client needed for comment checking.
type commentFetcher interface {
	FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error)
}

// partitionPlanningIssues splits issues into planning and coding sets.
func partitionPlanningIssues(issues []domain.Issue, planningLabel string) (planning, coding []domain.Issue) {
	for _, issue := range issues {
		isPlan := false
		for _, label := range issue.Labels {
			if strings.EqualFold(label, planningLabel) {
				isPlan = true
				break
			}
		}
		if isPlan {
			planning = append(planning, issue)
		} else {
			coding = append(coding, issue)
		}
	}
	return
}

// hasNewHumanComment checks if there are new non-bot comments after lastCommentID.
func hasNewHumanComment(client commentFetcher, ctx context.Context, repo string, issueID, lastCommentID int) (bool, int) {
	comments, err := client.FetchIssueComments(ctx, repo, issueID)
	if err != nil {
		return false, lastCommentID
	}

	latestID := lastCommentID
	hasNew := false
	for _, c := range comments {
		if c.ID > lastCommentID && !isBot(c.Author) {
			hasNew = true
		}
		if c.ID > latestID {
			latestID = c.ID
		}
	}
	return hasNew, latestID
}

// isBot returns true if the author looks like a bot account.
func isBot(author string) bool {
	return strings.HasSuffix(author, "[bot]")
}
