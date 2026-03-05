package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "tdd")
	os.MkdirAll(skillDir, 0755)

	content := `---
name: tdd
description: Use when implementing any code change
type: rigid
---

## Iron Law
Never write implementation before a failing test.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)

	skills, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "tdd" {
		t.Errorf("name = %q, want %q", skills[0].Name, "tdd")
	}
	if skills[0].Type != "rigid" {
		t.Errorf("type = %q, want %q", skills[0].Type, "rigid")
	}
	if skills[0].Description != "Use when implementing any code change" {
		t.Errorf("description = %q", skills[0].Description)
	}
	if skills[0].Content == "" {
		t.Error("content should not be empty")
	}
}

func TestLoadSkillOverride(t *testing.T) {
	base := t.TempDir()
	custom := t.TempDir()

	baseDir := filepath.Join(base, "tdd")
	os.MkdirAll(baseDir, 0755)
	os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte(`---
name: tdd
description: base version
type: rigid
---
Base content.
`), 0644)

	customDir := filepath.Join(custom, "tdd")
	os.MkdirAll(customDir, 0755)
	os.WriteFile(filepath.Join(customDir, "SKILL.md"), []byte(`---
name: tdd
description: custom version
type: rigid
---
Custom content.
`), 0644)

	skills, err := LoadFromDirs([]string{base, custom})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1 (override)", len(skills))
	}
	if skills[0].Description != "custom version" {
		t.Errorf("expected custom override, got %q", skills[0].Description)
	}
}

func TestLoadSkillMaxDepth(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "d")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "SKILL.md"), []byte(`---
name: deep
description: too deep
type: rigid
---
Content.
`), 0644)

	skills, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0 (too deep)", len(skills))
	}
}
