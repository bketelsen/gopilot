package orchestrator

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/skill"
)

// PromptData is the data available to prompt templates.
type PromptData struct {
	Issue  github.Issue
	Repo   string
	Branch string
	Skills string // formatted skill content appended to the prompt
}

// RenderPrompt renders the user's prompt template with issue data and appends skills.
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

	result := buf.String()

	// Append skills if present
	if data.Skills != "" {
		result += data.Skills
	}

	return result, nil
}

// LoadSkills loads skills from config and formats them for prompt injection.
func LoadSkills(dirs []string, enabled []string) ([]skill.Skill, error) {
	loader := skill.NewLoader(dirs)

	if len(enabled) > 0 {
		return loader.LoadByNames(enabled)
	}

	return loader.LoadAll()
}
