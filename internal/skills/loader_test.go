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
---
Base content.
`), 0644)

	customDir := filepath.Join(custom, "tdd")
	os.MkdirAll(customDir, 0755)
	os.WriteFile(filepath.Join(customDir, "SKILL.md"), []byte(`---
name: tdd
description: custom version
---
Custom content.
`), 0644)

	skills, err := Discover([]string{base, custom})
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
	deep := filepath.Join(dir, "a", "b", "c", "d", "e")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "SKILL.md"), []byte(`---
name: deep
description: too deep
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

func TestLoadSkillYAMLParsing(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "full")
	os.MkdirAll(skillDir, 0755)

	content := `---
name: full-skill
description: A skill with all fields
license: MIT
compatibility: copilot,claude
allowed-tools: Read,Grep
metadata:
  author: test
  version: "1.0"
---

Skill body content.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)

	skills, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	s := skills[0]
	if s.Name != "full-skill" {
		t.Errorf("name = %q, want %q", s.Name, "full-skill")
	}
	if s.Description != "A skill with all fields" {
		t.Errorf("description = %q", s.Description)
	}
	if s.License != "MIT" {
		t.Errorf("license = %q, want %q", s.License, "MIT")
	}
	if s.Compatibility != "copilot,claude" {
		t.Errorf("compatibility = %q, want %q", s.Compatibility, "copilot,claude")
	}
	if s.AllowedTools != "Read,Grep" {
		t.Errorf("allowed-tools = %q, want %q", s.AllowedTools, "Read,Grep")
	}
	if s.Metadata["author"] != "test" {
		t.Errorf("metadata[author] = %q, want %q", s.Metadata["author"], "test")
	}
	if s.Metadata["version"] != "1.0" {
		t.Errorf("metadata[version] = %q, want %q", s.Metadata["version"], "1.0")
	}
}

func TestLoadSkillColonInDescription(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "colon")
	os.MkdirAll(skillDir, 0755)

	content := `---
name: colon-test
description: "Use this: when you need colons"
---

Body.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)

	skills, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Description != "Use this: when you need colons" {
		t.Errorf("description = %q, want %q", skills[0].Description, "Use this: when you need colons")
	}
}

func TestLoadSkillMissingDescription(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "nodesc")
	os.MkdirAll(skillDir, 0755)

	content := `---
name: nodesc
---

Body.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)

	skills, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0 (missing description)", len(skills))
	}
}

func TestLoadSkillLocation(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "loc")
	os.MkdirAll(skillDir, 0755)

	content := `---
name: loc-test
description: location test
---

Body.
`
	skillPath := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(skillPath, []byte(content), 0644)

	skills, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}

	absPath, _ := filepath.Abs(skillPath)
	if skills[0].Location != absPath {
		t.Errorf("location = %q, want %q", skills[0].Location, absPath)
	}
}

func TestDiscoverSkipsEmptyDirs(t *testing.T) {
	skills, err := Discover([]string{"", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0", len(skills))
	}
}

func TestDiscoverSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()

	// Hidden dir should be skipped
	hiddenDir := filepath.Join(dir, ".hidden")
	os.MkdirAll(hiddenDir, 0755)
	os.WriteFile(filepath.Join(hiddenDir, "SKILL.md"), []byte(`---
name: hidden
description: hidden skill
---
Body.
`), 0644)

	// .agents dir should NOT be skipped
	agentsDir := filepath.Join(dir, ".agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "SKILL.md"), []byte(`---
name: agents-skill
description: agents skill
---
Body.
`), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "agents-skill" {
		t.Errorf("name = %q, want %q", skills[0].Name, "agents-skill")
	}
}

func TestDiscoverSkipsGitAndNodeModules(t *testing.T) {
	dir := t.TempDir()

	for _, skipDir := range []string{".git", "node_modules"} {
		d := filepath.Join(dir, skipDir)
		os.MkdirAll(d, 0755)
		os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(`---
name: `+skipDir+`
description: should be skipped
---
Body.
`), 0644)
	}

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0", len(skills))
	}
}

func TestDiscoverDepth5Allowed(t *testing.T) {
	dir := t.TempDir()
	// depth 5 = a/b/c/d should be allowed (5 components including "a")
	deep := filepath.Join(dir, "a", "b", "c", "d")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "SKILL.md"), []byte(`---
name: depth4
description: at depth 4 from root
---
Content.
`), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Errorf("got %d skills, want 1 (depth 4 should be within limit of 5)", len(skills))
	}
}

func TestLoadSkillFoldedScalar(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "pdf-processing")
	os.MkdirAll(skillDir, 0755)

	content := `---
name: pdf-processing
description: >-
  Extract text and tables from PDF files.
  Use when working with PDF documents.
license: Apache-2.0
compatibility: Requires poppler-utils
metadata:
  author: example-org
  version: "1.0"
allowed-tools: Bash(pdftotext:*) Read
---

## Instructions
Process PDFs here.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	s := skills[0]
	if s.Name != "pdf-processing" {
		t.Errorf("name = %q, want %q", s.Name, "pdf-processing")
	}
	// Folded scalar should join the two lines with a space
	wantDesc := "Extract text and tables from PDF files. Use when working with PDF documents."
	if s.Description != wantDesc {
		t.Errorf("description = %q, want %q", s.Description, wantDesc)
	}
	if s.License != "Apache-2.0" {
		t.Errorf("license = %q", s.License)
	}
	if s.Compatibility != "Requires poppler-utils" {
		t.Errorf("compatibility = %q", s.Compatibility)
	}
	if s.Metadata["author"] != "example-org" {
		t.Errorf("metadata author = %q", s.Metadata["author"])
	}
	if s.AllowedTools != "Bash(pdftotext:*) Read" {
		t.Errorf("allowed-tools = %q", s.AllowedTools)
	}
}
