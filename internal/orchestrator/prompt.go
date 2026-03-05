package orchestrator

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/bketelsen/gopilot/internal/github"
)

// PromptData is the data available to prompt templates.
type PromptData struct {
	Issue  github.Issue
	Repo   string
	Branch string
}

// RenderPrompt renders the user's prompt template with issue data.
func RenderPrompt(tmplStr string, data PromptData) (string, error) {
	funcMap := template.FuncMap{
		"join": strings.Join,
	}

	tmpl, err := template.New("prompt").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse prompt template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute prompt template: %w", err)
	}

	return buf.String(), nil
}
