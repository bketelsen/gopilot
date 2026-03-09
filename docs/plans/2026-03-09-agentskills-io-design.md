# Design: Migrate Skills System to agentskills.io Standard

**Date**: 2026-03-09
**Goal**: Replace gopilot's custom SKILL.md format and loading system with the [agentskills.io](https://agentskills.io) standard, enabling cross-client skill interoperability.

## Decision Summary

| Decision | Choice |
|----------|--------|
| Migration strategy | Full replacement of current format |
| Progressive disclosure | Hybrid — required skills injected eagerly, optional skills as catalog for file-read activation |
| Activation mechanism | File-read (agent uses built-in file-read tool with path from catalog) |
| `type` field (rigid/flexible) | Dropped — convey in skill prose instead |
| Discovery paths | Config `skills.dir` + workspace `<workspace>/.agents/skills/` |
| Name validation | Lenient — warn on violations, still load |
| Existing skill migration | Included in this work |

## 1. Skill Struct & Frontmatter Parsing

### New Skill struct

```go
type Skill struct {
    Name          string            // required, validated leniently
    Description   string            // required (skip skill if empty)
    Content       string            // markdown body after frontmatter
    Dir           string            // parent directory of SKILL.md
    Location      string            // absolute path to SKILL.md
    License       string            // optional
    Compatibility string            // optional
    Metadata      map[string]string // optional arbitrary k/v
    AllowedTools  string            // optional, experimental
}
```

**Removed**: `Type` field (rigid/flexible was gopilot-specific, not in the spec).

**Added**: `Location`, `License`, `Compatibility`, `Metadata`, `AllowedTools` per agentskills.io spec.

### Frontmatter parsing

Replace the hand-rolled line parser with `gopkg.in/yaml.v3` unmarshal. This handles:
- Colons in values (common cross-client compatibility issue)
- Multiline strings
- Nested `metadata` map

### Name validation (lenient)

Spec rules: lowercase, hyphens only, no leading/trailing/consecutive hyphens, must match parent directory, max 64 chars.

Behavior: `slog.Warn` on violations, still load the skill. Skip only if:
- `name` is empty
- `description` is empty
- YAML is completely unparseable

## 2. Discovery

### API

```go
// Discover finds all skills from the given directory list.
// Later directories take precedence over earlier ones (by skill name).
func Discover(dirs []string) ([]*Skill, error)
```

Replaces `LoadFromDir` and `LoadFromDirs`.

### Scanning rules

- Max depth: 5 levels
- Skip `.git/`, `node_modules/`, hidden directories (except `.agents`)
- Find files named exactly `SKILL.md`
- Warn on name collisions: `slog.Warn("skill shadowed", "name", name, "by", path)`

### Call sites

**Orchestrator startup** — config dir only:
```go
o.skills, err = skills.Discover([]string{cfg.Skills.Dir})
```

**Dispatch time** — config dir + workspace `.agents/skills/`:
```go
dirs := []string{cfg.Skills.Dir}
wsSkillsDir := filepath.Join(workspace, ".agents", "skills")
if info, err := os.Stat(wsSkillsDir); err == nil && info.IsDir() {
    dirs = append(dirs, wsSkillsDir)
}
dispatchSkills, err := skills.Discover(dirs)
```

Workspace skills override config skills (project overrides user convention).

**Config hot-reload** — re-run `Discover([]string{cfg.Skills.Dir})` to update `o.skills` on config change.

## 3. Injection — Required vs Catalog

`InjectSkills` signature stays the same:

```go
func InjectSkills(allSkills []*Skill, required []string, optional []string) string
```

Output format changes:

### Required skills — full content inline

```markdown
# Skills

## Skill: tdd

Never write implementation before a failing test.
...

---

## Skill: verification

Never claim work is done without evidence.
...
```

Note: `(type)` annotation removed from header.

### Optional skills — catalog with file paths

```markdown
# Available Skills

The following skills provide specialized instructions for specific tasks.
When a task matches a skill's description, use your file-read tool to load
the SKILL.md at the listed location before proceeding.

- **debugging** — Systematic debugging process for any bug or test failure
  Location: /home/debian/gopilot/skills/debugging/SKILL.md
- **code-review** — Code review checklist for completed work
  Location: /home/debian/gopilot/skills/code-review/SKILL.md
```

If no optional skills match, the "Available Skills" section is omitted.

## 4. Orchestrator Integration

### Dispatch (issue + PR fix)

Each dispatch re-discovers skills (config + workspace). Directory walk cost is negligible. Workspace-level skills are picked up without restart.

```go
// In dispatchAgent():
dirs := []string{cfg.Skills.Dir}
wsSkillsDir := filepath.Join(workspace, ".agents", "skills")
if info, err := os.Stat(wsSkillsDir); err == nil && info.IsDir() {
    dirs = append(dirs, wsSkillsDir)
}
dispatchSkills, err := skills.Discover(dirs)
skillText := skills.InjectSkills(dispatchSkills, o.cfg.Skills.Required, o.cfg.Skills.Optional)
```

### Planning chat

No change — continues to use `InjectSkills(skills, []string{"planning"}, nil)`.

### Config hot-reload

Re-run `Discover` on config change to update `o.skills`. Improvement over current behavior (skill files were not reloaded).

## 5. Existing Skill Migration

All 6 skills updated:

| Skill | Changes |
|-------|---------|
| `tdd` | Remove `type: rigid`. Strengthen description. Add rigidity instruction to body prose. |
| `verification` | Remove `type: rigid`. Strengthen description. Add rigidity instruction to body prose. |
| `debugging` | Remove `type: technique`. Strengthen description. |
| `code-review` | Remove `type: flexible`. Strengthen description. Add "adapt to context" note in body prose. |
| `pr-workflow` | Remove `type: rigid`. Strengthen description. Add rigidity instruction to body prose. |
| `planning` | Remove `type: rigid`. Strengthen description. Add rigidity instruction to body prose. |

No directory renames needed — all names comply with the spec.

### Example migration (`tdd/SKILL.md`)

**Before:**
```yaml
---
name: tdd
description: Use when implementing any code change, feature, or bug fix
type: rigid
---
```

**After:**
```yaml
---
name: tdd
description: >-
  Test-driven development workflow for implementing code changes, features,
  or bug fixes. Use when writing any new code or modifying existing behavior.
---
```

Body gets: `> This is a strict workflow. Follow each step exactly as written.`

## 6. Testing

| Test file | Coverage |
|-----------|----------|
| `loader_test.go` | New struct fields, YAML parsing, name validation warnings, missing description skipping |
| `injector_test.go` | Required = full content, optional = catalog with paths, no `(type)` formatting |
| `discovery_test.go` (new) | Multi-directory precedence, workspace override, `.agents/skills/` scanning, skip rules, name collision warnings |
| Orchestrator tests | Update assertions for new injection output format |

## 7. Documentation

| File | Change |
|------|--------|
| `docs/skills.md` | Rewrite: agentskills.io spec reference, new frontmatter fields, progressive disclosure, workspace skills |
| `docs/configuration.md` | Note `skills.dir` is config-level; workspace `.agents/skills/` auto-discovered |
| `docs/architecture.md` | Update skills package description |
| `CLAUDE.md` | Update package layout table if needed |
| `README.md` | Add agentskills.io compatibility mention |

## 8. Dependencies

No new dependencies. `gopkg.in/yaml.v3` is already in `go.mod` (used by config package).
