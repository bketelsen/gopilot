package planning

import (
	"fmt"
	"regexp"
	"strings"
)

// Plan represents a structured plan parsed from agent output.
type Plan struct {
	Title  string  `json:"title"`
	Phases []Phase `json:"phases"`
}

// Phase is a named group of tasks.
type Phase struct {
	Name  string `json:"name"`
	Tasks []Task `json:"tasks"`
}

// Task is a single actionable item in a plan.
type Task struct {
	Description  string `json:"description"`
	Complexity   string `json:"complexity"`
	Dependencies string `json:"dependencies"`
	Checked      bool   `json:"checked"`
}

var (
	planTitleRe = regexp.MustCompile(`^##\s+Plan:\s+(.+)`)
	phaseRe     = regexp.MustCompile(`^###\s+(?:Phase\s+\d+:\s+)?(.+)`)
	taskRe      = regexp.MustCompile(`^-\s+\[([ xX])\]\s+(.+?)(?:\s+\(complexity:\s+(\w+)\))?$`)
	depRe       = regexp.MustCompile(`^\s+Dependencies:\s+(.+)`)
)

// ParsePlan extracts a structured plan from markdown text.
func ParsePlan(markdown string) (*Plan, error) {
	lines := strings.Split(markdown, "\n")
	plan := &Plan{}
	var currentPhase *Phase

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if m := planTitleRe.FindStringSubmatch(line); m != nil {
			plan.Title = strings.TrimSpace(m[1])
			continue
		}

		if m := phaseRe.FindStringSubmatch(line); m != nil {
			plan.Phases = append(plan.Phases, Phase{Name: strings.TrimSpace(m[1])})
			currentPhase = &plan.Phases[len(plan.Phases)-1]
			continue
		}

		if m := taskRe.FindStringSubmatch(line); m != nil && currentPhase != nil {
			task := Task{
				Description: strings.TrimSpace(m[2]),
				Complexity:  m[3],
				Checked:     m[1] != " ",
			}
			if i+1 < len(lines) {
				if dm := depRe.FindStringSubmatch(lines[i+1]); dm != nil {
					task.Dependencies = strings.TrimSpace(dm[1])
					i++
				}
			}
			currentPhase.Tasks = append(currentPhase.Tasks, task)
		}
	}

	if plan.Title == "" {
		return nil, fmt.Errorf("no plan title found")
	}
	// Remove phases with no tasks (e.g. non-phase ### headings like "Key Design Decisions")
	filtered := plan.Phases[:0]
	for _, p := range plan.Phases {
		if len(p.Tasks) > 0 {
			filtered = append(filtered, p)
		}
	}
	plan.Phases = filtered
	return plan, nil
}

// PlanToMarkdown converts a structured plan to a markdown document.
func PlanToMarkdown(plan *Plan) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Plan: %s\n\n", plan.Title))
	for i, phase := range plan.Phases {
		b.WriteString(fmt.Sprintf("### Phase %d: %s\n\n", i+1, phase.Name))
		for _, task := range phase.Tasks {
			check := " "
			if task.Checked {
				check = "x"
			}
			b.WriteString(fmt.Sprintf("- [%s] %s", check, task.Description))
			if task.Complexity != "" {
				b.WriteString(fmt.Sprintf(" (complexity: %s)", task.Complexity))
			}
			b.WriteString("\n")
			if task.Dependencies != "" {
				b.WriteString(fmt.Sprintf("  Dependencies: %s\n", task.Dependencies))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
