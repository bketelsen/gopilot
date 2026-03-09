package embedded

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSkills(t *testing.T) {
	skills, err := ListSkills()
	if err != nil {
		t.Fatal(err)
	}

	if len(skills) == 0 {
		t.Fatal("expected at least one embedded skill")
	}

	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
		if s.Description == "" {
			t.Errorf("skill %q has empty description", s.Name)
		}
	}

	for _, expected := range []string{"code-review", "debugging", "planning", "pr-workflow", "tdd", "verification"} {
		if !names[expected] {
			t.Errorf("missing expected skill %q", expected)
		}
	}
}

func TestExtractSkill(t *testing.T) {
	dest := t.TempDir()

	if err := ExtractSkill("verification", dest); err != nil {
		t.Fatal(err)
	}

	skillPath := filepath.Join(dest, "verification", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("SKILL.md not extracted: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("extracted SKILL.md is empty")
	}
}

func TestExtractSkillNotFound(t *testing.T) {
	dest := t.TempDir()
	err := ExtractSkill("nonexistent", dest)
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}
