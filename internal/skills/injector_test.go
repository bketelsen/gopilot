package skills

import (
	"strings"
	"testing"
)

func TestInjectRequired(t *testing.T) {
	skills := []*Skill{
		{Name: "tdd", Content: "Write tests first."},
		{Name: "verification", Content: "Verify before claiming done."},
		{Name: "debugging", Content: "Debug systematically."},
	}
	required := []string{"tdd", "verification"}

	result := InjectSkills(skills, required, nil)
	if !strings.Contains(result, "Write tests first.") {
		t.Error("missing tdd content")
	}
	if !strings.Contains(result, "Verify before claiming done.") {
		t.Error("missing verification content")
	}
	if strings.Contains(result, "Debug systematically.") {
		t.Error("debugging should not be included (not required)")
	}
}

func TestInjectOptional(t *testing.T) {
	skills := []*Skill{
		{Name: "tdd", Content: "Write tests first."},
		{Name: "debugging", Content: "Debug systematically."},
	}
	required := []string{"tdd"}
	optional := []string{"debugging"}

	result := InjectSkills(skills, required, optional)
	if !strings.Contains(result, "Debug systematically.") {
		t.Error("optional debugging should be included")
	}
}

func TestInjectFormatting(t *testing.T) {
	skills := []*Skill{
		{Name: "tdd", Content: "TDD content here."},
	}
	result := InjectSkills(skills, []string{"tdd"}, nil)

	if !strings.Contains(result, "## Skill: tdd") {
		t.Error("missing skill section header")
	}
}
