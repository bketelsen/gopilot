# agentskills.io Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate gopilot's skills system from a custom SKILL.md format to the agentskills.io standard, enabling cross-client skill interoperability with progressive disclosure (required skills injected eagerly, optional skills as a catalog for file-read activation).

**Architecture:** Replace the hand-rolled frontmatter parser with YAML unmarshalling. Refactor `LoadFromDir`/`LoadFromDirs` into `Discover` with multi-directory precedence. Split the injector output into full-content (required) and catalog (optional) sections. Update all 6 built-in skill files. Update dashboard templates to remove the `Type` badge.

**Tech Stack:** Go 1.25, `gopkg.in/yaml.v3` (already in go.mod), templ templates, Tailwind CSS

---

### Task 1: Update Skill Struct and YAML Frontmatter Parsing

**Files:**
- Modify: `internal/skills/loader.go:1-109`
- Test: `internal/skills/loader_test.go`

**Step 1: Write the failing tests**

Add to `internal/skills/loader_test.go`. These test the new struct fields and YAML-based parsing:

```go
func TestParseYAMLFrontmatter(t *testing.T) {
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
	if !strings.Contains(s.Description, "Extract text") {
		t.Errorf("description = %q", s.Description)
	}
	if s.License != "Apache-2.0" {
		t.Errorf("license = %q, want %q", s.License, "Apache-2.0")
	}
	if s.Compatibility != "Requires poppler-utils" {
		t.Errorf("compatibility = %q", s.Compatibility)
	}
	if s.Metadata["author"] != "example-org" {
		t.Errorf("metadata author = %q", s.Metadata["author"])
	}
	if s.Metadata["version"] != "1.0" {
		t.Errorf("metadata version = %q", s.Metadata["version"])
	}
	if s.AllowedTools != "Bash(pdftotext:*) Read" {
		t.Errorf("allowed-tools = %q", s.AllowedTools)
	}
	if s.Location == "" {
		t.Error("location should be set")
	}
	if !strings.Contains(s.Content, "Process PDFs here.") {
		t.Error("content should contain body")
	}
}

func TestParseColonInDescription(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "test-skill")
	os.MkdirAll(skillDir, 0755)

	// This YAML has a colon in the description value — the old line parser would break on this.
	content := `---
name: test-skill
description: "Use this skill when: the user asks about testing"
---

Body content.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Description != "Use this skill when: the user asks about testing" {
		t.Errorf("description = %q", skills[0].Description)
	}
}

func TestSkipSkillMissingDescription(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "no-desc")
	os.MkdirAll(skillDir, 0755)

	content := `---
name: no-desc
---

Body content.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0 (missing description)", len(skills))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -run "TestParseYAML|TestParseColon|TestSkipSkillMissing" ./internal/skills/...`
Expected: FAIL — `Discover` undefined, new fields don't exist

**Step 3: Update the Skill struct**

Replace the struct in `internal/skills/loader.go:11-18` with:

```go
// Skill represents a loaded SKILL.md definition per the agentskills.io spec.
type Skill struct {
	Name          string            // required — unique identifier
	Description   string            // required — what the skill does and when to use it
	Content       string            // markdown body after frontmatter
	Dir           string            // parent directory of SKILL.md
	Location      string            // absolute path to SKILL.md
	License       string            // optional
	Compatibility string            // optional — environment requirements
	Metadata      map[string]string // optional — arbitrary key-value pairs
	AllowedTools  string            // optional — space-delimited pre-approved tools
}
```

**Step 4: Replace parseFrontmatter with YAML unmarshalling**

Replace `parseFrontmatter` function at `internal/skills/loader.go:92-109` with:

```go
// frontmatter is the YAML structure at the top of SKILL.md files.
type frontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	Metadata      map[string]string `yaml:"metadata"`
	AllowedTools  string            `yaml:"allowed-tools"`
}

func parseFrontmatterYAML(data string, skill *Skill) error {
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(data), &fm); err != nil {
		return fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	skill.Name = fm.Name
	skill.Description = fm.Description
	skill.License = fm.License
	skill.Compatibility = fm.Compatibility
	skill.Metadata = fm.Metadata
	skill.AllowedTools = fm.AllowedTools
	return nil
}
```

Add `"gopkg.in/yaml.v3"` to the imports (replace `"bufio"`).

**Step 5: Update parseSkillFile to use YAML parsing and set Location**

Replace `parseSkillFile` at `internal/skills/loader.go:68-90`:

```go
func parseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	skill := &Skill{}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	skill.Location = absPath

	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content[3:], "---", 2)
		if len(parts) == 2 {
			if err := parseFrontmatterYAML(parts[0], skill); err != nil {
				slog.Warn("skill frontmatter parse error, skipping", "path", path, "error", err)
				return nil, err
			}
			skill.Content = strings.TrimSpace(parts[1])
		}
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("skill at %s has no name in frontmatter", path)
	}
	if skill.Description == "" {
		slog.Warn("skill has no description, skipping", "path", path, "name", skill.Name)
		return nil, fmt.Errorf("skill at %s has no description", path)
	}

	return skill, nil
}
```

Add `"log/slog"` to imports.

**Step 6: Rename LoadFromDirs to Discover, update LoadFromDir**

Replace `LoadFromDir` and `LoadFromDirs` at `internal/skills/loader.go:20-66`:

```go
// Discover loads all SKILL.md files from multiple directory trees.
// Later directories take precedence over earlier ones (by skill name).
// This enables workspace skills to override config-level skills.
func Discover(dirs []string) ([]*Skill, error) {
	byName := make(map[string]*Skill)

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			// Skip hidden directories (except .agents) and common non-skill dirs.
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" {
					return filepath.SkipDir
				}
				if strings.HasPrefix(name, ".") && name != ".agents" {
					return filepath.SkipDir
				}
			}

			rel, _ := filepath.Rel(dir, path)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if depth > 5 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() || d.Name() != "SKILL.md" {
				return nil
			}

			skill, err := parseSkillFile(path)
			if err != nil {
				slog.Warn("skipping skill", "path", path, "error", err)
				return nil // warn and continue, don't fail the whole discovery
			}
			skill.Dir = filepath.Dir(path)

			if existing, ok := byName[skill.Name]; ok {
				slog.Warn("skill shadowed", "name", skill.Name, "by", path, "was", existing.Location)
			}
			byName[skill.Name] = skill
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	result := make([]*Skill, 0, len(byName))
	for _, s := range byName {
		result = append(result, s)
	}
	return result, nil
}

// LoadFromDir loads all SKILL.md files from a single directory tree.
// Deprecated: Use Discover instead.
func LoadFromDir(dir string) ([]*Skill, error) {
	return Discover([]string{dir})
}
```

**Step 7: Run tests to verify they pass**

Run: `go test -race ./internal/skills/...`
Expected: PASS (new tests pass, existing tests may need updates — see next step)

**Step 8: Update existing tests for new struct**

In `internal/skills/loader_test.go`, update the existing tests:

- `TestLoadSkill`: Remove the `Type` assertion (line 35-37). Change `LoadFromDir` to `Discover([]string{dir})` or keep as-is since `LoadFromDir` wraps `Discover`. Check `Location` is set.
- `TestLoadSkillOverride`: Remove `type: rigid` from test data or leave it (YAML parser ignores unknown fields? No — `type` is not in the struct, but the YAML parser with `yaml.v3` will ignore unknown fields by default). Update to use `Discover`.
- `TestLoadSkillMaxDepth`: Update depth expectation — depth limit is now 5, not 4. The test creates depth 4 (`a/b/c/d`), so it should now be found. Either increase depth to 5 (`a/b/c/d/e`) or adjust the assertion.

**Step 9: Run all tests again**

Run: `go test -race ./internal/skills/...`
Expected: PASS

**Step 10: Commit**

```bash
git add internal/skills/loader.go internal/skills/loader_test.go
git commit -m "feat: migrate skill struct and parser to agentskills.io spec

Replace hand-rolled frontmatter parser with YAML unmarshalling.
Add Location, License, Compatibility, Metadata, AllowedTools fields.
Remove Type field. Rename LoadFromDirs to Discover with lenient validation."
```

---

### Task 2: Add Name Validation with Lenient Warnings

**Files:**
- Modify: `internal/skills/loader.go`
- Test: `internal/skills/loader_test.go`

**Step 1: Write the failing tests**

```go
func TestNameValidationWarnings(t *testing.T) {
	tests := []struct {
		name      string
		dirName   string
		skillName string
		wantLoad  bool
	}{
		{"valid name", "code-review", "code-review", true},
		{"uppercase warns but loads", "my-skill", "My-Skill", true},
		{"name mismatch warns but loads", "my-skill", "other-name", true},
		{"consecutive hyphens warns but loads", "my--skill", "my--skill", true},
		{"leading hyphen warns but loads", "-my-skill", "-my-skill", true},
		{"empty name skips", "empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			skillDir := filepath.Join(dir, tt.dirName)
			os.MkdirAll(skillDir, 0755)

			fm := fmt.Sprintf("---\nname: %s\ndescription: test skill\n---\nBody.", tt.skillName)
			os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(fm), 0644)

			skills, err := Discover([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantLoad && len(skills) != 1 {
				t.Errorf("got %d skills, want 1", len(skills))
			}
			if !tt.wantLoad && len(skills) != 0 {
				t.Errorf("got %d skills, want 0", len(skills))
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestNameValidation ./internal/skills/...`
Expected: Some cases may pass already, but the validation warnings aren't emitted yet

**Step 3: Add validateName function**

Add to `internal/skills/loader.go` after the `frontmatter` struct:

```go
// validateName checks the skill name against agentskills.io naming rules.
// Returns warnings for violations but does not reject the skill (lenient mode).
func validateName(name, parentDir string) {
	if len(name) > 64 {
		slog.Warn("skill name exceeds 64 characters", "name", name)
	}
	if name != strings.ToLower(name) {
		slog.Warn("skill name should be lowercase", "name", name)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		slog.Warn("skill name should not start or end with hyphen", "name", name)
	}
	if strings.Contains(name, "--") {
		slog.Warn("skill name should not contain consecutive hyphens", "name", name)
	}
	dirName := filepath.Base(parentDir)
	if dirName != name {
		slog.Warn("skill name does not match parent directory", "name", name, "dir", dirName)
	}
}
```

**Step 4: Call validateName in parseSkillFile**

In `parseSkillFile`, after the name check and before the return, add:

```go
validateName(skill.Name, filepath.Dir(path))
```

**Step 5: Run tests**

Run: `go test -race ./internal/skills/...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/skills/loader.go internal/skills/loader_test.go
git commit -m "feat: add lenient name validation per agentskills.io spec"
```

---

### Task 3: Update Injector for Required Content + Optional Catalog

**Files:**
- Modify: `internal/skills/injector.go:1-38`
- Test: `internal/skills/injector_test.go:1-54`

**Step 1: Write the failing tests**

Replace the contents of `internal/skills/injector_test.go`:

```go
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
	// Optional skill should appear in catalog with description and location
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
	// Type annotation should NOT be present
	if strings.Contains(result, "(rigid)") {
		t.Error("type annotation should be removed")
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
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -run "TestInject" ./internal/skills/...`
Expected: FAIL — optional skills still get full content, type annotation still present

**Step 3: Rewrite the injector**

Replace `internal/skills/injector.go` entirely:

```go
package skills

import (
	"fmt"
	"strings"
)

// InjectSkills renders required skills as full content and optional skills as a
// lightweight catalog with file paths for on-demand file-read activation.
func InjectSkills(allSkills []*Skill, required []string, optional []string) string {
	byName := make(map[string]*Skill)
	for _, s := range allSkills {
		byName[s.Name] = s
	}

	var sections []string

	// Required skills: full content inline.
	var requiredParts []string
	for _, name := range required {
		if skill, ok := byName[name]; ok {
			requiredParts = append(requiredParts, formatSkill(skill))
		}
	}
	if len(requiredParts) > 0 {
		sections = append(sections, "# Skills\n\n"+strings.Join(requiredParts, "\n\n---\n\n"))
	}

	// Optional skills: catalog with description + location for file-read activation.
	var catalogEntries []string
	for _, name := range optional {
		if skill, ok := byName[name]; ok {
			catalogEntries = append(catalogEntries, formatCatalogEntry(skill))
		}
	}
	if len(catalogEntries) > 0 {
		catalog := "# Available Skills\n\n" +
			"The following skills provide specialized instructions for specific tasks.\n" +
			"When a task matches a skill's description, use your file-read tool to load\n" +
			"the SKILL.md at the listed location before proceeding.\n\n" +
			strings.Join(catalogEntries, "\n")
		sections = append(sections, catalog)
	}

	return strings.Join(sections, "\n\n")
}

func formatSkill(skill *Skill) string {
	return fmt.Sprintf("## Skill: %s\n\n%s", skill.Name, skill.Content)
}

func formatCatalogEntry(skill *Skill) string {
	return fmt.Sprintf("- **%s** — %s\n  Location: %s", skill.Name, skill.Description, skill.Location)
}
```

**Step 4: Run tests**

Run: `go test -race ./internal/skills/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/skills/injector.go internal/skills/injector_test.go
git commit -m "feat: split injection into required content + optional catalog

Required skills get full content inline. Optional skills appear as a
lightweight catalog with description and file path for on-demand
file-read activation per agentskills.io progressive disclosure."
```

---

### Task 4: Multi-Directory Discovery with Workspace Support

**Files:**
- Create: `internal/skills/discover_test.go`
- Modify: `internal/skills/loader.go` (already has `Discover`, just testing)

**Step 1: Write tests for multi-directory discovery**

Create `internal/skills/discover_test.go`:

```go
package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverMultiDir(t *testing.T) {
	configDir := t.TempDir()
	workspaceDir := t.TempDir()

	// Config-level skill
	tddDir := filepath.Join(configDir, "tdd")
	os.MkdirAll(tddDir, 0755)
	os.WriteFile(filepath.Join(tddDir, "SKILL.md"), []byte(`---
name: tdd
description: Config-level TDD
---
Config TDD content.
`), 0644)

	// Workspace-level skill that overrides config
	tddOverride := filepath.Join(workspaceDir, "tdd")
	os.MkdirAll(tddOverride, 0755)
	os.WriteFile(filepath.Join(tddOverride, "SKILL.md"), []byte(`---
name: tdd
description: Workspace-level TDD
---
Workspace TDD content.
`), 0644)

	// Workspace-only skill
	extraDir := filepath.Join(workspaceDir, "repo-lint")
	os.MkdirAll(extraDir, 0755)
	os.WriteFile(filepath.Join(extraDir, "SKILL.md"), []byte(`---
name: repo-lint
description: Repo-specific linting rules
---
Lint content.
`), 0644)

	skills, err := Discover([]string{configDir, workspaceDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(skills))
	}

	byName := make(map[string]*Skill)
	for _, s := range skills {
		byName[s.Name] = s
	}

	// tdd should be the workspace override
	if tdd, ok := byName["tdd"]; !ok {
		t.Error("tdd skill not found")
	} else if tdd.Description != "Workspace-level TDD" {
		t.Errorf("tdd should be workspace override, got %q", tdd.Description)
	}

	// repo-lint should be present
	if _, ok := byName["repo-lint"]; !ok {
		t.Error("repo-lint skill not found")
	}
}

func TestDiscoverSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hiddenDir := filepath.Join(dir, ".hidden", "secret")
	os.MkdirAll(hiddenDir, 0755)
	os.WriteFile(filepath.Join(hiddenDir, "SKILL.md"), []byte(`---
name: secret
description: Hidden skill
---
Secret content.
`), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0 (hidden dir)", len(skills))
	}
}

func TestDiscoverAgentsDirNotHidden(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents", "skills", "my-skill")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "SKILL.md"), []byte(`---
name: my-skill
description: Agent skill
---
Agent content.
`), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1 (.agents should not be skipped)", len(skills))
	}
	if skills[0].Name != "my-skill" {
		t.Errorf("name = %q, want %q", skills[0].Name, "my-skill")
	}
}

func TestDiscoverSkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	nmDir := filepath.Join(dir, "node_modules", "some-pkg")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, "SKILL.md"), []byte(`---
name: npm-skill
description: NPM skill
---
Content.
`), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0 (node_modules)", len(skills))
	}
}

func TestDiscoverEmptyDir(t *testing.T) {
	skills, err := Discover([]string{""})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0 (empty dir)", len(skills))
	}
}

func TestDiscoverMaxDepth(t *testing.T) {
	dir := t.TempDir()
	// Depth 5 — should be found (at the limit)
	shallow := filepath.Join(dir, "a", "b", "c", "d")
	os.MkdirAll(shallow, 0755)
	os.WriteFile(filepath.Join(shallow, "SKILL.md"), []byte(`---
name: shallow
description: At depth limit
---
Content.
`), 0644)

	// Depth 6 — should NOT be found (beyond limit)
	deep := filepath.Join(dir, "a", "b", "c", "d", "e")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "SKILL.md"), []byte(`---
name: deep
description: Beyond depth limit
---
Content.
`), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
	}
	if !names["shallow"] {
		t.Error("shallow skill should be found (at depth limit)")
	}
	if names["deep"] {
		t.Error("deep skill should not be found (beyond depth limit)")
	}
}

func TestDiscoverLocationIsAbsolute(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: my-skill
description: Test skill
---
Content.
`), 0644)

	skills, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if !filepath.IsAbs(skills[0].Location) {
		t.Errorf("location should be absolute, got %q", skills[0].Location)
	}
	if !strings.HasSuffix(skills[0].Location, "SKILL.md") {
		t.Errorf("location should end with SKILL.md, got %q", skills[0].Location)
	}
}
```

**Step 2: Run tests**

Run: `go test -race ./internal/skills/...`
Expected: PASS (these test the `Discover` function from Task 1)

**Step 3: Commit**

```bash
git add internal/skills/discover_test.go
git commit -m "test: comprehensive discovery tests for multi-dir, hidden dirs, depth"
```

---

### Task 5: Update Orchestrator Integration

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:66-70` (startup)
- Modify: `internal/orchestrator/orchestrator.go:349` (dispatch)
- Modify: `internal/orchestrator/orchestrator.go:741` (PR fix dispatch)

**Step 1: Update startup to use Discover**

At `internal/orchestrator/orchestrator.go:66`, change:

```go
allSkills, err := skills.LoadFromDir(cfg.Skills.Dir)
```

to:

```go
allSkills, err := skills.Discover([]string{cfg.Skills.Dir})
```

**Step 2: Update dispatch to merge workspace skills**

At `internal/orchestrator/orchestrator.go:349`, replace:

```go
skillText := skills.InjectSkills(o.skills, o.cfg.Skills.Required, o.cfg.Skills.Optional)
```

with:

```go
// Discover skills from config dir + workspace .agents/skills/ if present.
skillDirs := []string{o.cfg.Skills.Dir}
wsAgentsSkills := filepath.Join(wsPath, ".agents", "skills")
if info, err := os.Stat(wsAgentsSkills); err == nil && info.IsDir() {
	skillDirs = append(skillDirs, wsAgentsSkills)
}
dispatchSkills, err := skills.Discover(skillDirs)
if err != nil {
	log.Warn("skill discovery failed, using config skills", "error", err)
	dispatchSkills = o.skills
}
skillText := skills.InjectSkills(dispatchSkills, o.cfg.Skills.Required, o.cfg.Skills.Optional)
```

Add `"os"` to imports if not already present.

**Step 3: Update PR fix dispatch similarly**

At `internal/orchestrator/orchestrator.go:741`, replace:

```go
skillText := skills.InjectSkills(o.skills, o.cfg.Skills.Required, o.cfg.Skills.Optional)
```

with:

```go
skillDirs := []string{o.cfg.Skills.Dir}
wsAgentsSkills := filepath.Join(wsPath, ".agents", "skills")
if info, err := os.Stat(wsAgentsSkills); err == nil && info.IsDir() {
	skillDirs = append(skillDirs, wsAgentsSkills)
}
dispatchSkills, err := skills.Discover(skillDirs)
if err != nil {
	log.Warn("skill discovery failed for PR fix, using config skills", "error", err)
	dispatchSkills = o.skills
}
skillText := skills.InjectSkills(dispatchSkills, o.cfg.Skills.Required, o.cfg.Skills.Optional)
```

**Step 4: Run tests**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: PASS (may reveal other places referencing `.Type`)

**Step 6: Commit**

```bash
git add internal/orchestrator/orchestrator.go
git commit -m "feat: workspace skill discovery at dispatch time

Discover skills from config dir + workspace .agents/skills/ at each
dispatch, allowing repos to ship their own agentskills.io skills."
```

---

### Task 6: Update Dashboard Templates

**Files:**
- Modify: `internal/web/templates/pages/settings.templ:17-19` (SkillDisplay struct)
- Modify: `internal/web/templates/pages/settings.templ:68-72` (skill list item)
- Modify: `internal/web/templates/pages/settings.templ:116-126` (SkillTypeBadge — remove)
- Modify: `internal/web/server.go:316-322` (skill display mapping)
- Test: `internal/web/server_test.go`

**Step 1: Update SkillDisplay struct**

In `internal/web/templates/pages/settings.templ:17-19`, replace:

```
type SkillDisplay struct {
	Name        string
	Type        string
	Description string
}
```

with:

```
type SkillDisplay struct {
	Name        string
	Description string
	Location    string
}
```

**Step 2: Update skill list item template**

In `internal/web/templates/pages/settings.templ:68-72`, replace the skill list item:

```
for _, skill := range data.Skills {
	<li class="px-4 py-3 flex items-center gap-3">
		<span class="font-medium">{ skill.Name }</span>
		@SkillTypeBadge(skill.Type)
		<span class="text-sm text-gray-500">{ skill.Description }</span>
	</li>
}
```

with:

```
for _, skill := range data.Skills {
	<li class="px-4 py-3">
		<div class="flex items-center gap-3">
			<span class="font-medium">{ skill.Name }</span>
			<span class="text-sm text-gray-500">{ skill.Description }</span>
		</div>
		if skill.Location != "" {
			<div class="text-xs text-gray-400 mt-0.5 font-mono truncate">{ skill.Location }</div>
		}
	</li>
}
```

**Step 3: Remove SkillTypeBadge**

Delete the `SkillTypeBadge` templ component at lines 116-126.

**Step 4: Update server.go skill mapping**

In `internal/web/server.go:316-322`, replace:

```go
skillDisplays = append(skillDisplays, pages.SkillDisplay{
	Name:        sk.Name,
	Type:        sk.Type,
	Description: sk.Description,
})
```

with:

```go
skillDisplays = append(skillDisplays, pages.SkillDisplay{
	Name:        sk.Name,
	Description: sk.Description,
	Location:    sk.Location,
})
```

**Step 5: Regenerate templ and CSS**

Run: `task generate && task css`
Expected: Generates `settings_templ.go` without `SkillTypeBadge`

**Step 6: Run tests**

Run: `go test -race ./internal/web/...`
Expected: PASS (update server_test.go if it references `.Type`)

**Step 7: Commit**

```bash
git add internal/web/templates/pages/settings.templ internal/web/templates/pages/settings_templ.go internal/web/server.go internal/web/server_test.go
git commit -m "feat: update dashboard skill display for agentskills.io

Remove Type badge, show skill location path instead."
```

---

### Task 7: Migrate Built-in Skill Files

**Files:**
- Modify: `skills/tdd/SKILL.md`
- Modify: `skills/verification/SKILL.md`
- Modify: `skills/debugging/SKILL.md`
- Modify: `skills/code-review/SKILL.md`
- Modify: `skills/pr-workflow/SKILL.md`
- Modify: `skills/planning/SKILL.md`

**Step 1: Update `skills/tdd/SKILL.md`**

```markdown
---
name: tdd
description: >-
  Test-driven development workflow for implementing code changes, features,
  or bug fixes. Use when writing any new code or modifying existing behavior.
---

> This is a strict workflow. Follow each step exactly as written.

## Iron Law

Never write implementation before a failing test.

## Workflow

1. **RED** — Write a failing test that defines the expected behavior
2. **GREEN** — Write the minimum code to make the test pass
3. **REFACTOR** — Clean up while keeping tests green

## Red Flags

| Thought | Reality |
|---------|---------|
| "I'll add tests later" | You won't. Write them now. |
| "This is too simple to test" | Simple code has simple tests. Write them. |
| "The tests would just duplicate the implementation" | Then the implementation is the test. Rethink your design. |
| "Let me just get it working first" | A failing test IS "getting it working." |

## Verification

You MUST capture test output showing the red-to-green transition. A test you never saw fail proves nothing.
```

**Step 2: Update `skills/verification/SKILL.md`**

```markdown
---
name: verification
description: >-
  Verification checklist for completing any task. Use before claiming work
  is done to ensure tests pass, code builds, and linter is clean.
---

> This is a strict workflow. Follow each step exactly as written.

## Iron Law

Never claim work is done without evidence.

## Requirements

Before marking work as complete, you MUST run and capture output from:
1. Full test suite — all tests pass
2. Build — compiles without errors
3. Linter — no warnings or errors

## Red Flags

| Thought | Reality |
|---------|---------|
| "It should work" | Show me the output. |
| "I tested it manually" | Manual testing is not evidence. |
| "The change is trivial" | Trivial changes break things. Run the tests. |
| "CI will catch it" | You are CI. Catch it now. |

## Evidence

Paste the actual command output. Do not paraphrase or summarize.
```

**Step 3: Update `skills/debugging/SKILL.md`**

```markdown
---
name: debugging
description: >-
  Systematic debugging process for any bug, test failure, or unexpected
  behavior. Use when encountering errors or when code does not behave as expected.
---

## Workflow

1. **Reproduce** — Confirm the bug exists with a minimal reproduction
2. **Isolate** — Narrow down to the specific component or line
3. **Root Cause** — Identify WHY it happens, not just WHERE
4. **Fix** — Apply the minimal fix for the root cause
5. **Verify** — Confirm the fix resolves the issue
6. **Regression** — Run the full test suite to ensure no regressions

## Requirements

- Root cause MUST be identified before any fix is attempted
- Do not guess and check — trace the execution path
- Write a test that reproduces the bug BEFORE fixing it
```

**Step 4: Update `skills/code-review/SKILL.md`**

```markdown
---
name: code-review
description: >-
  Code review checklist for reviewing changes made by another agent or
  developer. Use when evaluating pull requests or completed work.
---

Adapt this checklist to the context of the changes being reviewed.

## Approach

Assume the implementer cut corners. Verify, don't trust.

## Checklist

1. Does the code match the issue requirements?
2. Are there tests for all new behavior?
3. Do existing tests still pass?
4. Is error handling appropriate?
5. Are there security concerns (injection, auth, data exposure)?
6. Is the code readable and maintainable?
7. Does the PR description accurately reflect the changes?

## Review Comments

- Be specific: reference file and line number
- Explain why something is a problem, not just that it is
- Suggest a concrete fix when possible
```

**Step 5: Update `skills/pr-workflow/SKILL.md`**

```markdown
---
name: pr-workflow
description: >-
  Branch, commit, and pull request workflow for submitting code changes.
  Use when creating branches, making commits, or opening pull requests.
---

> This is a strict workflow. Follow each step exactly as written.

## Iron Law

Never push directly to main. Never merge your own PR.

## Workflow

1. Create a feature branch: `gopilot/issue-{id}`
2. Make atomic commits with clear messages
3. Push the branch to origin
4. Open a pull request referencing the issue
5. Add a comment to the issue with a summary of changes
6. Set issue status to "In Review" in the GitHub Project

## PR Requirements

- Title references the issue number
- Description includes what changed and why
- Test plan is included
- CI checks pass before requesting review

## Red Flags

| Thought | Reality |
|---------|---------|
| "I'll just push to main since it's a small change" | Small changes break things too. Use a PR. |
| "I'll clean up the commits later" | Make them clean now. |
```

**Step 6: Update `skills/planning/SKILL.md`**

```markdown
---
name: planning
description: >-
  Guide interactive planning conversations for decomposing feature ideas
  into actionable GitHub issues. Use for planning sessions on GitHub issues.
---

> This is a strict workflow. Follow each step exactly as written.

## Role

You are a planning agent. Your job is to help decompose a feature idea into actionable GitHub issues through structured conversation.

## Phase Detection

Read the full comment thread to determine your current phase:

1. **No agent comments exist** -> You are in the INTRODUCTION phase. Post a greeting and your first clarifying question.
2. **You asked a question, human replied** -> You are in the QUESTIONING phase. Ask the next question or, if you have enough context, propose a plan.
3. **A plan comment with checkboxes exists** -> You are in the PLAN_PROPOSED phase. If the human has feedback, revise the plan. If they commented the approve command, proceed to issue creation.

## Rules

- Ask ONE question per response. Never ask multiple questions at once.
- Prefer multiple-choice questions when possible.
- Focus on: purpose, constraints, success criteria, dependencies, testing strategy.
- Keep questions concise. Do not over-explain.
- After gathering enough context (typically 3-8 questions), propose a plan.

## Plan Format

When proposing a plan, use this exact format:

    ## Proposed Plan: [Feature Name]

    ### Phase 1: [Phase Name]
    - [ ] **Issue title** -- Description of what this issue covers. _Complexity: small/medium/large_

    ### Phase 2: [Phase Name]
    - [ ] **Issue title** -- Description. `blocked by #N` if applicable. _Complexity: small/medium/large_

    ### Dependencies
    - [List cross-phase dependencies]

    ### Notes
    - [Rationale for granularity decisions]
    - [Assumptions made]

    Reply `/approve` to create these issues, or uncheck items you want to remove and comment with your changes.

## Granularity

Decide issue granularity based on complexity:
- **Simple features**: Fewer, coarser issues (one per shippable unit)
- **Complex features**: More granular breakdown (infrastructure, API, UI, tests as separate issues)
- Always explain your rationale in the Notes section.

## Issue Creation

When the human approves (comments the approve command):
1. Parse checked items only (`- [x]`)
2. Create Phase 1 issues first
3. Create Phase 2+ issues with `blocked by #N` referencing real issue numbers
4. Add all created issues as sub-issues of the planning issue
5. Post a summary comment with links to all created issues
```

**Step 7: Run full test suite**

Run: `go test -race ./...`
Expected: PASS

**Step 8: Commit**

```bash
git add skills/
git commit -m "feat: migrate built-in skills to agentskills.io format

Remove type field, strengthen descriptions for catalog disclosure,
add rigidity/flexibility instructions in prose."
```

---

### Task 8: Update Example Test

**Files:**
- Modify: `internal/skills/example_test.go:1-38`

**Step 1: Update the example test**

Replace `internal/skills/example_test.go`:

```go
package skills_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bketelsen/gopilot/internal/skills"
)

func ExampleDiscover() {
	dir, _ := os.MkdirTemp("", "example-skills")
	defer os.RemoveAll(dir)

	skillDir := filepath.Join(dir, "tdd")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: tdd
description: Test-driven development workflow
---

Write tests before implementation.`), 0644)

	loaded, err := skills.Discover([]string{dir})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("count:", len(loaded))
	fmt.Println("name:", loaded[0].Name)
	// Output:
	// count: 1
	// name: tdd
}
```

**Step 2: Run tests**

Run: `go test -race ./internal/skills/...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/skills/example_test.go
git commit -m "test: update example test for Discover API"
```

---

### Task 9: Update Config Reload to Re-discover Skills

**Files:**
- Modify: `internal/orchestrator/orchestrator.go` (config reload handler)

**Step 1: Find the config reload handler**

Search for the config watcher callback in `orchestrator.go`. It should reference skill reloading.

**Step 2: Add skill re-discovery on config change**

In the config reload callback, after updating `o.cfg`, add:

```go
reloadedSkills, err := skills.Discover([]string{newCfg.Skills.Dir})
if err != nil {
	slog.Warn("failed to reload skills on config change", "error", err)
} else {
	o.skills = reloadedSkills
	slog.Info("skills reloaded", "count", len(reloadedSkills))
}
```

**Step 3: Run tests**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/orchestrator/orchestrator.go
git commit -m "feat: reload skills on config file change"
```

---

### Task 10: Lint, Build, and Final Verification

**Step 1: Run linter**

Run: `task lint`
Expected: 0 issues

**Step 2: Run full build**

Run: `task build`
Expected: Binary builds successfully

**Step 3: Run full test suite**

Run: `task test`
Expected: All tests pass with race detector

**Step 4: Final commit if any fixups needed**

```bash
git add -A
git commit -m "chore: fix lint issues from agentskills.io migration"
```

---

### Task 11: Update Documentation

**Files:**
- Modify: `docs/skills.md`
- Modify: `docs/configuration.md`
- Modify: `docs/architecture.md`
- Modify: `README.md`
- Modify: `CLAUDE.md` (if needed)

**Step 1: Update `docs/skills.md`**

Add a section explaining agentskills.io compatibility:
- Link to https://agentskills.io/specification
- Document the frontmatter fields (name, description, license, compatibility, metadata, allowed-tools)
- Explain progressive disclosure (required = full injection, optional = catalog)
- Document workspace-level skills (`.agents/skills/` in workspace)
- Explain that skills from repos override config-level skills by name

**Step 2: Update `docs/configuration.md`**

In the skills section, note:
- `skills.dir` is the config-level skills directory
- Workspace `.agents/skills/` directories are auto-discovered at dispatch time
- Skills are reloaded on config file change

**Step 3: Update `docs/architecture.md`**

Update the skills package description to mention agentskills.io compliance and progressive disclosure.

**Step 4: Update `README.md`**

Add a bullet point in the features section: "agentskills.io compatible skill system"

**Step 5: Commit**

```bash
git add docs/ README.md CLAUDE.md
git commit -m "docs: update documentation for agentskills.io skills migration"
```
