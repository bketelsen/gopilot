# Writing Skills

## Overview

Skills are behavioral contracts injected into agent prompts. They enforce workflows like TDD, verification, and debugging by providing structured instructions that agents must follow during execution. Each skill is a Markdown file with YAML frontmatter, stored as a `SKILL.md` file in a skills directory.

Gopilot's skill system follows the [agentskills.io specification](https://agentskills.io/specification), an open standard for portable AI agent skills.

Skills allow you to codify engineering standards -- test-driven development, verification before completion, systematic debugging -- and have them automatically applied to every agent run.

## SKILL.md Format

A skill file has two parts: YAML frontmatter (delimited by `---` lines) followed by the Markdown body.

```markdown
---
name: my-skill
description: When to apply this skill
---

## Section Heading

Instructions for the agent go here as standard Markdown.
```

The opening `---` must be the very first line of the file. The closing `---` separates the frontmatter from the body. Everything after the closing delimiter is the skill content that gets injected into the agent prompt.

## Frontmatter Fields

| Field            | Required | Description                                                                 |
|------------------|----------|-----------------------------------------------------------------------------|
| `name`           | Yes      | Unique identifier for the skill. Used in config to reference it.            |
| `description`    | Yes      | When to use this skill. Helps agents decide applicability.                  |
| `license`        | No       | License identifier for the skill (e.g., `MIT`, `Apache-2.0`).              |
| `compatibility`  | No       | List of compatible agents or platforms.                                     |
| `metadata`       | No       | Arbitrary key-value pairs for additional information.                       |
| `allowed-tools`  | No       | Experimental. List of tools the agent is allowed to use with this skill.    |

The `name` field is required. A SKILL.md file without a `name` in its frontmatter will be rejected by the loader.

### Name Validation

Skill names should use lowercase alphanumeric characters and hyphens (e.g., `my-skill`). Names that don't follow this convention will produce a warning in the logs but are still loaded -- the loader is lenient to avoid breaking existing setups.

If two skills share the same `name`, the last one loaded wins (skills are deduplicated by name). See [Precedence](#precedence) for details on which directory wins.

## Directory Structure

Skills can come from two sources:

### Config-level skills directory

Set via `skills.dir` in `gopilot.yaml` (default: `./skills/`). Each skill gets its own subdirectory containing a `SKILL.md` file:

```
skills/
  tdd/
    SKILL.md
  verification/
    SKILL.md
  debugging/
    SKILL.md
  my-custom-skill/
    SKILL.md
```

### Workspace-level skills

Repositories can ship their own skills in a `.agents/skills/` directory at the repo root. These are auto-discovered at dispatch time when the workspace is set up -- no configuration is needed.

```
my-repo/
  .agents/
    skills/
      repo-conventions/
        SKILL.md
      deploy-checklist/
        SKILL.md
  src/
  ...
```

The loader walks each skills directory looking for files named exactly `SKILL.md`, up to 4 levels deep. Any file not named `SKILL.md` is ignored.

### Precedence

When skills from multiple directories share the same `name`, workspace-level skills (from `.agents/skills/`) override config-level skills. This allows repositories to customize or replace global skills for their specific needs.

## Required vs Optional (Progressive Disclosure)

Skills are classified as **required** or **optional** in the configuration. The two categories use different injection strategies:

- **Required** skills are injected as **full content** into every agent prompt. The complete Markdown body of each required skill is included directly in the rendered prompt.
- **Optional** skills are presented as a **catalog** -- a list of skill names, descriptions, and file paths. The agent can read the full skill content on demand by accessing the file path. This keeps prompts compact while still making all skills discoverable.

This progressive disclosure approach ensures agents always have critical instructions (required skills) while avoiding prompt bloat from optional skills that may not be relevant to every task.

Configure skills in `gopilot.yaml` under the `skills` section:

```yaml
skills:
  dir: ./skills
  required:
    - tdd
    - verification
  optional:
    - debugging
    - code-review
```

The values in `required` and `optional` lists must match the `name` field in the corresponding SKILL.md frontmatter.

## Built-in Skills

Gopilot ships with the following skills in the `skills/` directory:

| Name           | Description                                                    |
|----------------|----------------------------------------------------------------|
| `tdd`          | Test-driven development: red-green-refactor workflow           |
| `verification` | Verify work with test/build/lint output before claiming done   |
| `debugging`    | Systematic debugging: reproduce, isolate, root cause, fix      |
| `code-review`  | Code review checklist and comment standards                    |
| `planning`     | Interactive planning conversations in GitHub issues            |
| `pr-workflow`  | Branch, commit, and pull request workflow                      |

## Writing Your Own

1. Create a directory for your skill:

    ```bash
    mkdir -p skills/my-skill
    ```

2. Create `skills/my-skill/SKILL.md` with frontmatter and instructions:

    ```markdown
    ---
    name: my-skill
    description: Use when doing X
    ---

    ## Workflow

    1. First step
    2. Second step
    3. Third step
    ```

3. Add the skill to your `gopilot.yaml` under `skills.required` or `skills.optional`:

    ```yaml
    skills:
      required:
        - my-skill
    ```

4. Verify the skill loads correctly:

    ```bash
    gopilot --dry-run
    ```

## Example

Here is the complete TDD skill as a reference:

```markdown
---
name: tdd
description: Use when implementing any code change, feature, or bug fix
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

## Tips

- **Keep skills focused on one concern.** A skill should address a single workflow or standard. Combine multiple skills via the config rather than cramming everything into one file.
- **Use tables for "red flags" patterns.** These capture common rationalizations that agents might use to skip steps. The two-column format (Thought / Reality) is effective for this.
- **Skills are injected as raw Markdown into the prompt.** Write them as direct instructions to the agent. Use imperative language ("Do X", "Never do Y") rather than descriptions ("This skill is about X").
- **Structure with headings and lists.** Agents parse structured Markdown more reliably than prose paragraphs.
- **Leverage workspace skills for repo-specific conventions.** Add a `.agents/skills/` directory to any repository that needs custom agent behavior without changing the global Gopilot config.
