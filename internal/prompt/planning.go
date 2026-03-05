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
	b.WriteString(fmt.Sprintf("## Issue: %s (#%d)\n\n", issue.Title, issue.ID))
	b.WriteString(issue.Body)
	b.WriteString("\n\n")

	if len(comments) > 0 {
		b.WriteString("## Conversation History\n\n")
		for _, c := range comments {
			b.WriteString(fmt.Sprintf("%s: %s\n\n", c.Author, c.Body))
		}
	}

	if skill != "" {
		b.WriteString("## Instructions\n\n")
		b.WriteString(skill)
		b.WriteString("\n")
	}

	return b.String()
}
