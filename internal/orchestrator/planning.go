package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bketelsen/gopilot/internal/domain"
)

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

// PlanningCommentMarker is a hidden HTML comment added to all planning agent comments.
// Used to distinguish agent-posted comments from human comments by the same user.
const PlanningCommentMarker = "<!-- gopilot-planning-agent -->"

// isBotComment returns true if the comment was posted by a bot account or by the planning agent.
func isBotComment(c domain.Comment) bool {
	if strings.HasSuffix(c.Author, "[bot]") {
		return true
	}
	return strings.Contains(c.Body, PlanningCommentMarker)
}

// processPlanningIssues handles planning-labeled issues by posting a redirect
// comment pointing users to the interactive dashboard planning UI.
func (o *Orchestrator) processPlanningIssues(ctx context.Context, issues []domain.Issue) {
	for _, issue := range issues {
		// Skip if we've already redirected this issue.
		if o.state.IsPlanning(issue.ID) {
			continue
		}

		baseURL := o.cfg.Dashboard.ExternalURL
		if baseURL == "" {
			addr := o.cfg.Dashboard.Addr
			if addr == "" {
				addr = ":3000"
			}
			baseURL = fmt.Sprintf("http://localhost%s", addr)
		}
		body := fmt.Sprintf(
			"Planning sessions are now interactive in the dashboard.\n\n"+
				"Start one at: %s/planning/new?repo=%s&issue=%d\n\n"+
				"%s",
			baseURL, issue.Repo, issue.ID, PlanningCommentMarker,
		)
		if err := o.github.AddComment(ctx, issue.Repo, issue.ID, body); err != nil {
			slog.Error("failed to post planning redirect", "issue", issue.Identifier(), "error", err)
			continue
		}

		o.state.AddPlanning(issue.ID, &PlanningEntry{
			IssueID: issue.ID,
			Repo:    issue.Repo,
			Phase:   PlanningPhaseComplete,
		})

		slog.Info("posted planning redirect", "issue", issue.Identifier())
	}
}
