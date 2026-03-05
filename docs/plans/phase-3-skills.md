# Phase 3: Skills

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Skill loader, prompt injector, and 5 default skills.

**Prerequisite:** Phase 2 complete.

---

### Task 3.1: Skill Loader

**Files:**
- Create: `internal/skills/loader.go`
- Test: `internal/skills/loader_test.go`

**Step 1: Write the failing test**

```go
// internal/skills/loader_test.go
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

	// Base skill
	baseDir := filepath.Join(base, "tdd")
	os.MkdirAll(baseDir, 0755)
	os.WriteFile(filepath.Join(baseDir, "SKILL.md"), []byte(`---
name: tdd
description: base version
type: rigid
---
Base content.
`), 0644)

	// Custom override
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
	// Depth 4 — should NOT be found (max depth 3)
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
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/skills/...`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/skills/loader.go
package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded behavioral contract.
type Skill struct {
	Name        string
	Description string
	Type        string // rigid, flexible, technique
	Content     string // Markdown body after frontmatter
	Dir         string // directory containing the skill
}

// LoadFromDir discovers and loads all SKILL.md files from a directory (max depth 3).
func LoadFromDir(dir string) ([]*Skill, error) {
	return LoadFromDirs([]string{dir})
}

// LoadFromDirs loads skills from multiple directories. Later dirs override earlier ones.
func LoadFromDirs(dirs []string) ([]*Skill, error) {
	byName := make(map[string]*Skill)

	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip errors
			}

			// Check depth (max 3 levels below dir)
			rel, _ := filepath.Rel(dir, path)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if depth > 4 { // dir + 3 levels + filename = 4 segments
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
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			skill.Dir = filepath.Dir(path)
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

func parseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	skill := &Skill{}

	// Parse YAML frontmatter between --- delimiters
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content[3:], "---", 2)
		if len(parts) == 2 {
			parseFrontmatter(parts[0], skill)
			skill.Content = strings.TrimSpace(parts[1])
		}
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("skill at %s has no name in frontmatter", path)
	}

	return skill, nil
}

func parseFrontmatter(fm string, skill *Skill) {
	scanner := bufio.NewScanner(strings.NewReader(fm))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			switch key {
			case "name":
				skill.Name = val
			case "description":
				skill.Description = val
			case "type":
				skill.Type = val
			}
		}
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/skills/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/skills/
git commit -m "feat: skill loader with YAML frontmatter and directory override"
```

---

### Task 3.2: Skill Injector

**Files:**
- Create: `internal/skills/injector.go`
- Test: `internal/skills/injector_test.go`

**Step 1: Write the failing test**

```go
// internal/skills/injector_test.go
package skills

import (
	"strings"
	"testing"
)

func TestInjectRequired(t *testing.T) {
	skills := []*Skill{
		{Name: "tdd", Type: "rigid", Content: "Write tests first."},
		{Name: "verification", Type: "rigid", Content: "Verify before claiming done."},
		{Name: "debugging", Type: "technique", Content: "Debug systematically."},
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
		{Name: "tdd", Type: "rigid", Content: "Write tests first."},
		{Name: "debugging", Type: "technique", Content: "Debug systematically."},
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
		{Name: "tdd", Type: "rigid", Content: "TDD content here."},
	}
	result := InjectSkills(skills, []string{"tdd"}, nil)

	// Should have section markers
	if !strings.Contains(result, "## Skill: tdd") {
		t.Error("missing skill section header")
	}
	if !strings.Contains(result, "(rigid)") {
		t.Error("missing skill type")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/skills/... -run TestInject`
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/skills/injector.go
package skills

import (
	"fmt"
	"strings"
)

// InjectSkills formats skills for prompt injection.
// Required skills are always included. Optional skills are included if in the optional list.
func InjectSkills(allSkills []*Skill, required []string, optional []string) string {
	byName := make(map[string]*Skill)
	for _, s := range allSkills {
		byName[s.Name] = s
	}

	var parts []string

	// Required skills first
	for _, name := range required {
		if skill, ok := byName[name]; ok {
			parts = append(parts, formatSkill(skill))
		}
	}

	// Optional skills
	for _, name := range optional {
		if skill, ok := byName[name]; ok {
			parts = append(parts, formatSkill(skill))
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "# Skills\n\n" + strings.Join(parts, "\n\n---\n\n")
}

func formatSkill(skill *Skill) string {
	return fmt.Sprintf("## Skill: %s (%s)\n\n%s", skill.Name, skill.Type, skill.Content)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race ./internal/skills/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/skills/
git commit -m "feat: skill injector for prompt formatting"
```

---

### Task 3.3: Default Skills

**Files:**
- Create: `skills/tdd/SKILL.md`
- Create: `skills/verification/SKILL.md`
- Create: `skills/pr-workflow/SKILL.md`
- Create: `skills/debugging/SKILL.md`
- Create: `skills/code-review/SKILL.md`

**Step 1: Write the 5 default skills**

`skills/tdd/SKILL.md`:
```markdown
---
name: tdd
description: Use when implementing any code change, feature, or bug fix
type: rigid
---

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

`skills/verification/SKILL.md`:
```markdown
---
name: verification
description: Use when completing any task before claiming it is done
type: rigid
---

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

`skills/pr-workflow/SKILL.md`:
```markdown
---
name: pr-workflow
description: Use when creating branches, commits, and pull requests
type: rigid
---

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

`skills/debugging/SKILL.md`:
```markdown
---
name: debugging
description: Use when encountering bugs, test failures, or unexpected behavior
type: technique
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

`skills/code-review/SKILL.md`:
```markdown
---
name: code-review
description: Use when reviewing code changes made by another agent or developer
type: flexible
---

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

**Step 2: Verify skills load**

```bash
# Quick smoke test — the loader tests already verify parsing
ls -la skills/*/SKILL.md
```

**Step 3: Commit**

```bash
git add skills/
git commit -m "feat: ship 5 default skills — tdd, verification, pr-workflow, debugging, code-review"
```

---

### Task 3.4: Integrate Skills into Orchestrator Prompt

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`

**Step 1: Load skills on startup and inject into prompts**

In `NewOrchestrator` or a new `Init` method:

```go
// Load skills
allSkills, err := skills.LoadFromDir(cfg.Skills.Dir)
if err != nil {
    slog.Warn("failed to load skills", "error", err)
}
o.skills = allSkills
```

In `dispatch()`, when rendering the prompt:

```go
skillText := skills.InjectSkills(o.skills, o.cfg.Skills.Required, o.cfg.Skills.Optional)
rendered, err := prompt.Render(o.cfg.Prompt, issue, attempt, skillText)
```

**Step 2: Run all tests**

Run: `go test -race ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat: integrate skill loading and injection into dispatch"
```

---

## Phase 3 Milestone

Run: `go test -race ./...` — all tests pass.

Skills system:
- Loads SKILL.md files with YAML frontmatter
- Supports required and optional skill selection
- Later directories override earlier ones (custom skills)
- 5 default skills shipped (tdd, verification, pr-workflow, debugging, code-review)
- Skills injected into agent prompts during dispatch
