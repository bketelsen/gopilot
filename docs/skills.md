# Writing Skills

## Overview

Skills are behavioral contracts injected into agent prompts. They enforce workflows like TDD, verification, and debugging by providing structured instructions that agents must follow during execution. Each skill is a Markdown file with YAML frontmatter, stored as a `SKILL.md` file in the skills directory.

Skills allow you to codify engineering standards -- test-driven development, verification before completion, systematic debugging -- and have them automatically applied to every agent run.

## SKILL.md Format

A skill file has two parts: YAML frontmatter (delimited by `---` lines) followed by the Markdown body.

```markdown
---
name: my-skill
description: When to apply this skill
type: rigid
---

## Section Heading

Instructions for the agent go here as standard Markdown.
```

The opening `---` must be the very first line of the file. The closing `---` separates the frontmatter from the body. Everything after the closing delimiter is the skill content that gets injected into the agent prompt.

## Frontmatter Fields

| Field         | Required | Description                                                        |
|---------------|----------|--------------------------------------------------------------------|
| `name`        | Yes      | Unique identifier for the skill. Used in config to reference it.   |
| `description` | No       | When to use this skill. Helps agents decide applicability.         |
| `type`        | No       | `rigid` (follow exactly, no deviation) or `flexible` (adapt principles to context). |

The `name` field is required. A SKILL.md file without a `name` in its frontmatter will be rejected by the loader.

If two skills share the same `name`, the last one loaded wins (skills are deduplicated by name).

## Directory Structure

Skills live in a configurable directory (default: `./skills/`). Each skill gets its own subdirectory containing a `SKILL.md` file:

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

The loader walks the skills directory looking for files named exactly `SKILL.md`, up to 4 levels deep. Any file not named `SKILL.md` is ignored.

## Required vs Optional

Skills are classified as **required** or **optional** in the configuration:

- **Required** skills are always injected into every agent prompt, regardless of context.
- **Optional** skills may be included based on context.

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

| Name           | Type       | Description                                                    |
|----------------|------------|----------------------------------------------------------------|
| `tdd`          | rigid      | Test-driven development: red-green-refactor workflow           |
| `verification` | rigid      | Verify work with test/build/lint output before claiming done   |
| `debugging`    | technique  | Systematic debugging: reproduce, isolate, root cause, fix      |
| `code-review`  | flexible   | Code review checklist and comment standards                    |
| `planning`     | rigid      | Interactive planning conversations in GitHub issues            |
| `pr-workflow`  | rigid      | Branch, commit, and pull request workflow                      |

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
    type: flexible
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

## Tips

- **Keep skills focused on one concern.** A skill should address a single workflow or standard. Combine multiple skills via the config rather than cramming everything into one file.
- **Use tables for "red flags" patterns.** These capture common rationalizations that agents might use to skip steps. The two-column format (Thought / Reality) is effective for this.
- **Use `rigid` for non-negotiable workflows.** TDD, verification, and PR workflows should be rigid -- agents must follow them exactly.
- **Use `flexible` for guidelines that need adaptation.** Debugging and code review benefit from flexibility because the approach varies by situation.
- **Skills are injected as raw Markdown into the prompt.** Write them as direct instructions to the agent. Use imperative language ("Do X", "Never do Y") rather than descriptions ("This skill is about X").
- **Structure with headings and lists.** Agents parse structured Markdown more reliably than prose paragraphs.
