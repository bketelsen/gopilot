package prompt

import (
	"fmt"
	"strings"

	"github.com/bketelsen/gopilot/internal/domain"
)

// RenderPlanning builds the prompt for a planning agent invocation.
func RenderPlanning(issue domain.Issue, comments []domain.Comment, skill string) string {
	var b strings.Builder

	b.WriteString("# Planning Task\n\n")
	b.WriteString("You are a PLANNING agent. You do NOT write code. You do NOT create branches or PRs.\n")
	b.WriteString("Your ONLY job is to post a single comment on the GitHub issue using the `gh` CLI.\n\n")

	b.WriteString(fmt.Sprintf("## Issue: %s#%d — %s\n\n", issue.Repo, issue.ID, issue.Title))
	b.WriteString(issue.Body)
	b.WriteString("\n\n")

	if len(comments) > 0 {
		b.WriteString("## Conversation History\n\n")
		for _, c := range comments {
			b.WriteString(fmt.Sprintf("**%s:** %s\n\n", c.Author, c.Body))
		}
	}

	b.WriteString("## How to Respond\n\n")
	b.WriteString("Post your response as a comment on the issue using this exact command:\n\n")
	b.WriteString(fmt.Sprintf("```bash\ngh issue comment %d --repo %s --body \"YOUR RESPONSE HERE\"\n```\n\n", issue.ID, issue.Repo))
	b.WriteString("Rules:\n")
	b.WriteString("- Post EXACTLY ONE comment, then exit\n")
	b.WriteString("- Do NOT create files, branches, or PRs\n")
	b.WriteString("- Do NOT write any code\n")
	b.WriteString("- Do NOT modify any files\n")
	b.WriteString("- Your ONLY action is running the `gh issue comment` command above\n")
	b.WriteString("- CRITICAL: You MUST include this exact marker at the END of every comment body:\n")
	b.WriteString("  `<!-- gopilot-planning-agent -->`\n")
	b.WriteString("  This marker is used to distinguish your comments from human comments.\n\n")

	if skill != "" {
		b.WriteString("## Planning Instructions\n\n")
		b.WriteString(skill)
		b.WriteString("\n")
	}

	return b.String()
}
