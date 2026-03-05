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
