package prompt

import (
	"strings"
	"testing"

	"github.com/bketelsen/gopilot/internal/domain"
)

func TestRender(t *testing.T) {
	tmpl := `Issue: {{ .Issue.Repo }}#{{ .Issue.ID }} — {{ .Issue.Title }}
Labels: {{ joinStrings .Issue.Labels ", " }}
Attempt: {{ .Attempt }}`

	issue := domain.Issue{
		ID:     42,
		Repo:   "owner/repo",
		Title:  "Fix the bug",
		Labels: []string{"gopilot", "bug"},
	}

	result, err := Render(tmpl, issue, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "owner/repo#42") {
		t.Errorf("missing issue identifier in: %s", result)
	}
	if !strings.Contains(result, "gopilot, bug") {
		t.Errorf("missing labels in: %s", result)
	}
	if !strings.Contains(result, "Attempt: 1") {
		t.Errorf("missing attempt in: %s", result)
	}
}

func TestRenderWithSkills(t *testing.T) {
	tmpl := `Do the work.
{{ .Skills }}`

	issue := domain.Issue{ID: 1, Repo: "o/r"}
	skills := "## TDD\nWrite tests first."

	result, err := Render(tmpl, issue, 1, skills)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "## TDD") {
		t.Errorf("missing skills in: %s", result)
	}
}

func TestRenderRetryContext(t *testing.T) {
	tmpl := `{{ if gt .Attempt 1 }}RETRY attempt {{ .Attempt }}{{ end }}`

	issue := domain.Issue{ID: 1, Repo: "o/r"}

	result1, _ := Render(tmpl, issue, 1, "")
	if strings.Contains(result1, "RETRY") {
		t.Error("attempt 1 should not show retry context")
	}

	result2, _ := Render(tmpl, issue, 3, "")
	if !strings.Contains(result2, "RETRY attempt 3") {
		t.Errorf("attempt 3 should show retry: %s", result2)
	}
}

func TestRenderPRFix(t *testing.T) {
	pr := domain.PullRequest{
		Number:  22,
		Repo:    "owner/repo",
		HeadRef: "gopilot/issue-11",
		URL:     "https://github.com/owner/repo/pull/22",
		Title:   "feat: add linting",
		IssueID: 11,
	}
	failedChecks := []domain.CheckRun{
		{
			Name:       "lint",
			Status:     "completed",
			Conclusion: "failure",
			DetailsURL: "https://github.com/owner/repo/actions/runs/123",
			Output:     "golangci-lint: command not found",
		},
	}

	result, err := RenderPRFix(pr, failedChecks, 1, "")
	if err != nil {
		t.Fatal(err)
	}

	checks := []string{
		"gopilot/issue-11",
		"#11",
		"lint",
		"golangci-lint: command not found",
		"feat: add linting",
		"Push fixes to the existing branch",
	}
	for _, want := range checks {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in rendered PR fix prompt:\n%s", want, result)
		}
	}
}

func TestRenderPRFixNoIssueID(t *testing.T) {
	pr := domain.PullRequest{
		Number:  5,
		Repo:    "o/r",
		HeadRef: "feature-branch",
		URL:     "https://github.com/o/r/pull/5",
		Title:   "some PR",
	}
	result, err := RenderPRFix(pr, nil, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "Original Issue") {
		t.Error("should not show Original Issue when IssueID is 0")
	}
}
