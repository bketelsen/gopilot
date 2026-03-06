package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

func TestRenderPlanningPrompt(t *testing.T) {
	issue := domain.Issue{
		ID:    1,
		Repo:  "o/r",
		Title: "Build auth system",
		Body:  "We need user authentication with OAuth",
	}
	comments := []domain.Comment{
		{ID: 100, Author: "gopilot[bot]", Body: "What providers?", CreatedAt: time.Now().Add(-time.Minute)},
		{ID: 101, Author: "user", Body: "Google and GitHub", CreatedAt: time.Now()},
	}
	skill := "## Skill: planning (rigid)\n\nYou are a planning agent."

	result := RenderPlanning(issue, comments, skill)

	if !strings.Contains(result, "Build auth system") {
		t.Error("missing issue title")
	}
	if !strings.Contains(result, "We need user authentication with OAuth") {
		t.Error("missing issue body")
	}
	if !strings.Contains(result, "**gopilot[bot]:** What providers?") {
		t.Error("missing bot comment")
	}
	if !strings.Contains(result, "**user:** Google and GitHub") {
		t.Error("missing user comment")
	}
	if !strings.Contains(result, "planning agent") {
		t.Error("missing skill content")
	}
	if !strings.Contains(result, "gh issue comment 1 --repo o/r") {
		t.Error("missing gh CLI instruction")
	}
	if !strings.Contains(result, "You do NOT write code") {
		t.Error("missing planning-only constraint")
	}
	if !strings.Contains(result, "<!-- gopilot-planning-agent -->") {
		t.Error("missing planning comment marker instruction")
	}
}

func TestRenderPlanningNoComments(t *testing.T) {
	issue := domain.Issue{ID: 1, Repo: "o/r", Title: "Test", Body: "Body"}
	result := RenderPlanning(issue, nil, "skill")
	if strings.Contains(result, "Conversation History") {
		t.Error("should not contain conversation history when no comments")
	}
	if !strings.Contains(result, "gh issue comment 1 --repo o/r") {
		t.Error("should always contain gh CLI instruction")
	}
}
