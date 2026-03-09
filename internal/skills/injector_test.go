package skills

import (
	"strings"
	"testing"
)

func TestInjectRequired(t *testing.T) {
	skills := []*Skill{
		{Name: "tdd", Content: "Write tests first."},
		{Name: "verification", Content: "Verify before claiming done."},
		{Name: "debugging", Content: "Debug systematically.", Location: "/skills/debugging/SKILL.md"},
	}

	result := InjectSkills(skills, []string{"tdd", "verification"}, nil)
	if !strings.Contains(result, "Write tests first.") {
		t.Error("missing tdd content")
	}
	if !strings.Contains(result, "Verify before claiming done.") {
		t.Error("missing verification content")
	}
	if strings.Contains(result, "Debug systematically.") {
		t.Error("debugging should not be included")
	}
}

func TestInjectOptionalAsCatalog(t *testing.T) {
	skills := []*Skill{
		{Name: "tdd", Content: "Write tests first."},
		{Name: "debugging", Description: "Systematic debugging process", Content: "Debug systematically.", Location: "/skills/debugging/SKILL.md"},
	}

	result := InjectSkills(skills, []string{"tdd"}, []string{"debugging"})
	// Required skill should have full content
	if !strings.Contains(result, "Write tests first.") {
		t.Error("required tdd content missing")
	}
	// Optional skill should NOT have full content
	if strings.Contains(result, "Debug systematically.") {
		t.Error("optional skill should not have full content injected")
	}
	// Optional skill should appear in catalog
	if !strings.Contains(result, "debugging") {
		t.Error("optional skill name missing from catalog")
	}
	if !strings.Contains(result, "Systematic debugging process") {
		t.Error("optional skill description missing from catalog")
	}
	if !strings.Contains(result, "/skills/debugging/SKILL.md") {
		t.Error("optional skill location missing from catalog")
	}
}

func TestInjectFormattingNoType(t *testing.T) {
	skills := []*Skill{
		{Name: "tdd", Content: "TDD content here."},
	}
	result := InjectSkills(skills, []string{"tdd"}, nil)

	if !strings.Contains(result, "## Skill: tdd") {
		t.Error("missing skill section header")
	}
	if strings.Contains(result, "(rigid)") {
		t.Error("type annotation should not be present")
	}
}

func TestInjectCatalogInstruction(t *testing.T) {
	skills := []*Skill{
		{Name: "debugging", Description: "Debug stuff", Location: "/skills/debugging/SKILL.md"},
	}
	result := InjectSkills(skills, nil, []string{"debugging"})

	if !strings.Contains(result, "file-read") {
		t.Error("catalog should include file-read activation instruction")
	}
	if !strings.Contains(result, "Available Skills") {
		t.Error("catalog should have Available Skills header")
	}
}

func TestInjectNoSkills(t *testing.T) {
	result := InjectSkills(nil, nil, nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestInjectOnlyOptional(t *testing.T) {
	skills := []*Skill{
		{Name: "debugging", Description: "Debug stuff", Location: "/skills/debugging/SKILL.md"},
	}
	result := InjectSkills(skills, nil, []string{"debugging"})

	// Should have catalog but no "# Skills" section
	if strings.Contains(result, "# Skills\n") {
		t.Error("should not have Skills header when no required skills")
	}
	if !strings.Contains(result, "# Available Skills") {
		t.Error("should have Available Skills header")
	}
}
