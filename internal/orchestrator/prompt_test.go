package orchestrator

import (
	"strings"
	"testing"

	"github.com/bketelsen/gopilot/internal/github"
)

func TestRenderPrompt(t *testing.T) {
	tmpl := `Fix issue #{{.Issue.ID}} in {{.Repo}}.
Title: {{.Issue.Title}}
Labels: {{join .Issue.Labels ", "}}
Branch: {{.Branch}}`

	data := PromptData{
		Issue: github.Issue{
			ID:     42,
			Title:  "Fix the widget",
			Labels: []string{"bug", "gopilot"},
		},
		Repo:   "owner/repo",
		Branch: "gopilot/issue-42",
	}

	result, err := RenderPrompt(tmpl, data)
	if err != nil {
		t.Fatalf("RenderPrompt() error: %v", err)
	}

	if !strings.Contains(result, "#42") {
		t.Errorf("result missing issue ID: %s", result)
	}
	if !strings.Contains(result, "Fix the widget") {
		t.Errorf("result missing title: %s", result)
	}
	if !strings.Contains(result, "bug, gopilot") {
		t.Errorf("result missing labels: %s", result)
	}
	if !strings.Contains(result, "gopilot/issue-42") {
		t.Errorf("result missing branch: %s", result)
	}
}

func TestRenderPromptWithSkills(t *testing.T) {
	tmpl := `Fix issue #{{.Issue.ID}}`

	data := PromptData{
		Issue:  github.Issue{ID: 42},
		Skills: "\n\n## Skills\n\nWrite tests first.",
	}

	result, err := RenderPrompt(tmpl, data)
	if err != nil {
		t.Fatalf("RenderPrompt() error: %v", err)
	}

	if !strings.Contains(result, "Write tests first.") {
		t.Errorf("result missing skills: %s", result)
	}
}

func TestRenderPromptInvalidTemplate(t *testing.T) {
	_, err := RenderPrompt("{{.Bad", PromptData{})
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
}
