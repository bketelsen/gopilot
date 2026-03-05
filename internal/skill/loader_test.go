package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAll(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "tdd.md"), []byte("# TDD\nWrite tests first."), 0644)
	os.WriteFile(filepath.Join(dir, "review.md"), []byte("# Review\nReview code carefully."), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a skill"), 0644)

	loader := NewLoader([]string{dir})
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	if !names["tdd"] || !names["review"] {
		t.Errorf("expected tdd and review skills, got %v", names)
	}
}

func TestLoadByNames(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "tdd.md"), []byte("# TDD"), 0644)
	os.WriteFile(filepath.Join(dir, "review.md"), []byte("# Review"), 0644)

	loader := NewLoader([]string{dir})

	skills, err := loader.LoadByNames([]string{"tdd"})
	if err != nil {
		t.Fatalf("LoadByNames() error: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "tdd" {
		t.Errorf("expected [tdd], got %v", skills)
	}

	_, err = loader.LoadByNames([]string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
}

func TestOverride(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	os.WriteFile(filepath.Join(dir1, "tdd.md"), []byte("original"), 0644)
	os.WriteFile(filepath.Join(dir2, "tdd.md"), []byte("override"), 0644)

	loader := NewLoader([]string{dir1, dir2})
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Content != "override" {
		t.Errorf("expected override content, got %q", skills[0].Content)
	}
}

func TestFormatForPrompt(t *testing.T) {
	skills := []Skill{
		{Name: "tdd", Content: "Write tests first."},
		{Name: "review", Content: "Review carefully."},
	}

	result := FormatForPrompt(skills)
	if !strings.Contains(result, "### Skill: tdd") {
		t.Error("missing tdd header")
	}
	if !strings.Contains(result, "Write tests first.") {
		t.Error("missing tdd content")
	}
	if !strings.Contains(result, "### Skill: review") {
		t.Error("missing review header")
	}
}

func TestFormatForPromptEmpty(t *testing.T) {
	result := FormatForPrompt(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestMissingDir(t *testing.T) {
	loader := NewLoader([]string{"/nonexistent/path"})
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}
