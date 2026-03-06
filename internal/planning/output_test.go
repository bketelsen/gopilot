package planning_test

import (
	"strings"
	"testing"

	"github.com/bketelsen/gopilot/internal/planning"
)

func TestParsePlan(t *testing.T) {
	markdown := `## Plan: Auth System Redesign
### Phase 1: Foundation
- [ ] Create auth middleware (complexity: M)
  Dependencies: none
- [ ] Add JWT token validation (complexity: S)
  Dependencies: none
### Phase 2: Integration
- [x] Wire up protected routes (complexity: L)
  Dependencies: Create auth middleware
`
	plan, err := planning.ParsePlan(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Title != "Auth System Redesign" {
		t.Errorf("expected title 'Auth System Redesign', got %q", plan.Title)
	}
	if len(plan.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(plan.Phases))
	}
	if plan.Phases[0].Name != "Foundation" {
		t.Errorf("expected phase name 'Foundation', got %q", plan.Phases[0].Name)
	}
	if len(plan.Phases[0].Tasks) != 2 {
		t.Errorf("expected 2 tasks in phase 1, got %d", len(plan.Phases[0].Tasks))
	}
	if plan.Phases[0].Tasks[0].Description != "Create auth middleware" {
		t.Errorf("unexpected task description: %q", plan.Phases[0].Tasks[0].Description)
	}
	if plan.Phases[0].Tasks[0].Complexity != "M" {
		t.Errorf("expected complexity M, got %q", plan.Phases[0].Tasks[0].Complexity)
	}
	if plan.Phases[0].Tasks[0].Dependencies != "none" {
		t.Errorf("expected deps 'none', got %q", plan.Phases[0].Tasks[0].Dependencies)
	}
	if plan.Phases[0].Tasks[0].Checked {
		t.Error("expected unchecked task")
	}
	if !plan.Phases[1].Tasks[0].Checked {
		t.Error("expected checked task")
	}
	if plan.Phases[1].Tasks[0].Dependencies != "Create auth middleware" {
		t.Errorf("expected deps 'Create auth middleware', got %q", plan.Phases[1].Tasks[0].Dependencies)
	}
}

func TestParsePlan_NoTitle(t *testing.T) {
	_, err := planning.ParsePlan("just some text")
	if err == nil {
		t.Error("expected error for missing title")
	}
}

func TestPlanToMarkdown(t *testing.T) {
	plan := &planning.Plan{
		Title: "Test Plan",
		Phases: []planning.Phase{
			{
				Name: "Setup",
				Tasks: []planning.Task{
					{Description: "Do thing", Complexity: "S", Checked: true},
					{Description: "Another thing", Complexity: "M", Dependencies: "Do thing"},
				},
			},
		},
	}
	doc := planning.PlanToMarkdown(plan)
	if !strings.Contains(doc, "## Plan: Test Plan") {
		t.Error("expected title in output")
	}
	if !strings.Contains(doc, "[x] Do thing") {
		t.Error("expected checked task")
	}
	if !strings.Contains(doc, "[ ] Another thing") {
		t.Error("expected unchecked task")
	}
	if !strings.Contains(doc, "Dependencies: Do thing") {
		t.Error("expected dependencies in output")
	}
}

func TestParsePlan_RoundTrip(t *testing.T) {
	original := &planning.Plan{
		Title: "Round Trip Test",
		Phases: []planning.Phase{
			{
				Name: "Phase One",
				Tasks: []planning.Task{
					{Description: "Task A", Complexity: "S", Checked: true},
					{Description: "Task B", Complexity: "L", Dependencies: "Task A"},
				},
			},
		},
	}
	md := planning.PlanToMarkdown(original)
	parsed, err := planning.ParsePlan(md)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Title != original.Title {
		t.Errorf("title mismatch: %q vs %q", parsed.Title, original.Title)
	}
	if len(parsed.Phases) != len(original.Phases) {
		t.Fatalf("phase count mismatch: %d vs %d", len(parsed.Phases), len(original.Phases))
	}
	if len(parsed.Phases[0].Tasks) != len(original.Phases[0].Tasks) {
		t.Fatalf("task count mismatch")
	}
}
