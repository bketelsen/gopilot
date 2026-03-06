package prompt

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/bketelsen/gopilot/internal/domain"
)

// PromptData is the data passed to the prompt template.
type PromptData struct {
	Issue   domain.Issue
	Attempt int
	Skills  string
}

// Render executes the prompt template with the given data.
func Render(tmpl string, issue domain.Issue, attempt int, skills string) (string, error) {
	funcMap := template.FuncMap{
		"joinStrings": strings.Join,
	}

	t, err := template.New("prompt").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}

	data := PromptData{
		Issue:   issue,
		Attempt: attempt,
		Skills:  skills,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// PRFixData is the data passed to the PR fix prompt template.
type PRFixData struct {
	PR           domain.PullRequest
	FailedChecks []domain.CheckRun
	Attempt      int
	Skills       string
}

const prFixTemplate = `You are fixing a pull request that has failing CI checks.

## Pull Request
- PR: {{.PR.URL}}
- Branch: {{.PR.HeadRef}}
- Title: {{.PR.Title}}
{{- if .PR.IssueID}}
- Original Issue: #{{.PR.IssueID}}
{{- end}}

## Failed Checks
{{range .FailedChecks}}
### {{.Name}}
Status: {{.Conclusion}}
{{- if .Output}}
Failure output (truncated):
` + "```" + `
{{.Output}}
` + "```" + `
{{- end}}
Details: {{.DetailsURL}}
{{end}}

## Instructions
1. Check out the existing branch ` + "`{{.PR.HeadRef}}`" + `
2. Read the CI failure output carefully
3. Fix the root cause of the failure
4. Ensure all checks will pass
5. Commit and push to the same branch

Do NOT create a new PR. Push fixes to the existing branch.
{{if .Attempt | lt 1}}
This is fix attempt {{.Attempt}}. Previous attempts did not resolve all failures.
{{end}}
{{if .Skills}}
## Skills
{{.Skills}}
{{end}}`

// RenderPRFix renders the prompt for an agent that will fix a failing PR.
func RenderPRFix(pr domain.PullRequest, failedChecks []domain.CheckRun, attempt int, skills string) (string, error) {
	t, err := template.New("pr-fix").Parse(prFixTemplate)
	if err != nil {
		return "", err
	}

	data := PRFixData{
		PR:           pr,
		FailedChecks: failedChecks,
		Attempt:      attempt,
		Skills:       skills,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
