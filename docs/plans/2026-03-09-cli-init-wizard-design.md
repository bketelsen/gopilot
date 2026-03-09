# CLI Init Wizard & Cobra Migration Design

**Date:** 2026-03-09
**Status:** Approved

## Summary

Migrate gopilot's CLI from bare `flag` package to cobra+fang for styled output and structured commands. Replace the current `init` command (which just writes a default YAML) with a guided interactive wizard using charmbracelet/huh. Embed all skills in the binary so they can be selected and extracted during init.

## CLI Structure

Flat cobra commands, same as today:

```
gopilot          # run orchestrator (root command's RunE)
gopilot init     # guided setup wizard
gopilot setup    # create GitHub labels
```

fang wraps the root command adding styled help, `--version`, completions, and man page generation.

### Flags (root command)

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to config file (persistent, all subcommands) | `gopilot.yaml` |
| `--dry-run` | List eligible issues without dispatching | `false` |
| `--debug` | Enable debug logging | `false` |
| `--port` | Override dashboard listen port | (from config) |
| `--log` | Write logs to file | (none) |

### Package Layout

| File | Purpose |
|------|---------|
| `cmd/gopilot/main.go` | Creates root command, calls `fang.Execute()` |
| `cmd/gopilot/root.go` | Root command (orchestrator), flag definitions |
| `cmd/gopilot/init.go` | `init` subcommand with huh wizard |
| `cmd/gopilot/setup.go` | `setup` subcommand (EnsureLabels) |

## Embedded Skills

### Location

New top-level package `embedded/skills.go`:

```go
package embedded

import "embed"

//go:embed all:skills
var skills embed.FS
```

The `all:` prefix includes hidden files/dirs (matching loader behavior).

### API

```go
// SkillInfo holds metadata for display in the wizard
type SkillInfo struct {
    Name        string
    Description string
}

// ListSkills returns all embedded skill names and descriptions
func ListSkills() ([]SkillInfo, error)

// ExtractSkill copies the full skill directory tree to destDir/name/
func ExtractSkill(name string, destDir string) error
```

`ListSkills()` reads SKILL.md frontmatter from the embedded FS. `ExtractSkill()` copies the entire subdirectory (scripts, subdirs, all assets) to disk.

## Init Wizard Flow

**Precondition:** If `gopilot.yaml` exists, confirm overwrite.

### Steps

1. **GitHub Token** — `huh.Input` with password echo mode. Pre-fills from `$GITHUB_TOKEN` env. Validates non-empty.

2. **Repos** — `huh.Input` for comma-separated `owner/repo` entries (e.g. `myorg/api, myorg/web`). Validates format.

3. **Agent** — `huh.Select`: `copilot` or `claude`. Default: `claude`.

4. **Skills Selection** — `huh.MultiSelect` populated from `embedded.ListSkills()`. Pre-checked defaults: `verification`, `pr-workflow`, `code-review`. Shows name + description.

5. **Required vs Optional** — `huh.MultiSelect` from selected skills: "Which should be required (always injected)?" Remainder become optional.

### Output

- Writes `gopilot.yaml` with user values + sensible defaults for all other settings
- Creates `skills/` directory, extracts selected skill directory trees
- Prints summary of what was created
- Suggests editing `gopilot.yaml` for advanced settings
- Suggests running `gopilot setup` to create GitHub labels

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI command framework |
| `github.com/charmbracelet/fang` | Styled cobra wrapper (help, version, completions) |
| `github.com/charmbracelet/huh` | Interactive terminal forms |

## Testing

- `embedded/` — unit tests for `ListSkills()` (returns 6 skills with correct metadata) and `ExtractSkill()` (writes full directory tree)
- Config generation logic extracted into testable helper
- Init wizard interactive flow: manual testing
- Cobra command wiring: declarative, light testing

## Non-Goals

- No changes to the orchestrator, agent runners, or web dashboard
- No changes to the skills loader (`Discover()` reads from disk as before)
- No changes to config hot-reload
- Advanced config options (polling interval, concurrency, dashboard settings) stay as "edit the YAML" — not in the wizard
