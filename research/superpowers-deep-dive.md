# Deep Dive: obra/superpowers

Researched: 2026-03-05
Source: https://github.com/obra/superpowers (v4.3.1, MIT License, by Jesse Vincent)

## What It Is

Superpowers is a **complete software development workflow system for AI coding agents**. It is NOT a runtime or framework — it's a collection of structured Markdown documents ("skills") that, when injected into an AI agent's context, enforce a disciplined development pipeline:

**brainstorm → design → plan → implement (via subagents with TDD) → review → finish**

There is no server, no daemon, no compiled binary. The entire system is "prompt engineering as infrastructure" — Markdown documents with YAML frontmatter that define behavioral contracts AI agents must follow.

---

## Core Concepts: Skills

A **skill** is a `SKILL.md` file with YAML frontmatter:

```yaml
---
name: skill-name
description: Use when [triggering conditions]
---
```

- `name`: hyphenated identifier
- `description`: third-person trigger condition starting with "Use when..." — critically describes WHEN to trigger, NOT what it does (prevents AI from shortcutting)

### Skill Types
- **Rigid/Discipline-Enforcing** (TDD, debugging): must be followed exactly
- **Flexible/Pattern** (code review, worktrees): principles to adapt
- **Technique** (root-cause-tracing): concrete steps
- **Reference** (API docs): information to look up

### The 14 Skills

| Skill | Purpose |
|-------|---------|
| `using-superpowers` | Bootstrap: forces skill checking before every action |
| `brainstorming` | Design-before-code via Socratic questioning |
| `writing-plans` | Creates bite-sized implementation plans (2-5 min tasks) |
| `subagent-driven-development` | Executes plans via fresh subagent per task + 2-stage review |
| `executing-plans` | Alternative: batch execution with human checkpoints |
| `dispatching-parallel-agents` | Parallel subagent dispatch for independent problems |
| `test-driven-development` | RED-GREEN-REFACTOR enforcement |
| `systematic-debugging` | 4-phase root cause investigation before any fix |
| `verification-before-completion` | Evidence before claims — no "should work" allowed |
| `requesting-code-review` | Dispatch code reviewer subagent |
| `receiving-code-review` | Handle review feedback (no performative agreement) |
| `using-git-worktrees` | Isolated workspace setup |
| `finishing-a-development-branch` | Merge/PR/keep/discard decision workflow |
| `writing-skills` | Meta-skill for creating new skills using TDD |

---

## Execution Model

### Layer 1: Bootstrap Injection
On session start, a hook injects the `using-superpowers` skill wrapped in `<EXTREMELY_IMPORTANT>` tags into the system prompt. This forces the AI to check for relevant skills before every response.

### Layer 2: Skill Matching
The AI reads user messages and matches against skill descriptions. Even a "1% chance" of relevance means the skill must be invoked. Loading happens via the platform's `Skill` tool.

### Layer 3: Development Pipeline
- **Brainstorming**: Socratic questioning → 2-3 approach proposals → section-by-section approval → saves to `docs/plans/YYYY-MM-DD-<topic>-design.md`
- **Writing Plans**: Break design into 2-5 minute tasks with exact file paths, complete code, and verification commands
- **Execution** (two modes):
  - **Subagent-Driven**: Controller dispatches fresh subagent per task → Self-review → Spec compliance review (separate subagent) → Code quality review (separate subagent). Loops until approved.
  - **Batch Execution**: 3 tasks at a time with human checkpoints

### Layer 4: Subagent Architecture
- Each subagent gets a prompt template (implementer, spec-reviewer, code-quality-reviewer)
- Controller provides FULL TASK TEXT (subagents never read the plan file)
- Subagents are stateless — fresh context per task prevents "context pollution"
- Spec reviewer is told: "The implementer finished suspiciously quickly. Their report may be incomplete. You MUST verify everything independently."

### Cost Data
From integration tests: a typical 2-task plan with 7 subagent dispatches ≈ $4.67. Each subagent ≈ $0.07-$0.09.

---

## Platform Integrations

| Platform | Integration Method | Key File |
|----------|-------------------|----------|
| **Claude Code** | Plugin + hooks | `.claude-plugin/plugin.json`, `hooks/hooks.json` |
| **Cursor** | Plugin + hooks | `.cursor-plugin/plugin.json` |
| **OpenCode** | System prompt transform | `.opencode/plugins/superpowers.js` |
| **Codex** | Native skill discovery | `.codex/INSTALL.md` |

### Tool Mappings

| Claude Code | OpenCode | Description |
|-------------|----------|-------------|
| `TodoWrite` | `update_plan` | Task tracking |
| `Task` (subagents) | `@mention` syntax | Subagent dispatch |
| `Skill` tool | Native `skill` | Skill loading |
| `Read/Write/Edit/Bash` | Native tools | File operations |

---

## Extension Model

### Adding Skills
```
skills/
  skill-name/
    SKILL.md              # Main reference (required)
    supporting-file.*     # Optional supporting files
```

### Personal Skill Shadowing
Personal skills override superpowers skills (per-user customization without forking):
- Claude Code: `~/.claude/skills/`
- Codex: `~/.agents/skills/`
- OpenCode: `~/.config/opencode/skills/`

Resolution order: personal → superpowers (unless `superpowers:` prefix forces superpowers version).

### Skill Discovery
`lib/skills-core.js` (208 lines, zero npm dependencies):
- `findSkillsInDir()` — recursively scans for `SKILL.md` (max depth 3)
- `extractFrontmatter()` — parses YAML frontmatter
- `resolveSkillPath()` — handles shadowing

### Commands & Agents
- `commands/` — slash command shortcuts to skills (e.g., `/brainstorm`, `/execute-plan`)
- `agents/` — specialized agent role definitions (e.g., `code-reviewer.md`)

---

## Repository Structure

```
superpowers/
├── skills/                            # THE CORE
│   ├── using-superpowers/SKILL.md     # Bootstrap
│   ├── brainstorming/SKILL.md
│   ├── writing-plans/SKILL.md
│   ├── subagent-driven-development/
│   │   ├── SKILL.md
│   │   ├── implementer-prompt.md
│   │   ├── spec-reviewer-prompt.md
│   │   └── code-quality-reviewer-prompt.md
│   ├── executing-plans/SKILL.md
│   ├── test-driven-development/SKILL.md
│   ├── systematic-debugging/
│   │   ├── SKILL.md
│   │   ├── root-cause-tracing.md
│   │   ├── defense-in-depth.md
│   │   └── find-polluter.sh
│   ├── dispatching-parallel-agents/SKILL.md
│   ├── verification-before-completion/SKILL.md
│   ├── requesting-code-review/
│   ├── receiving-code-review/SKILL.md
│   ├── using-git-worktrees/SKILL.md
│   ├── finishing-a-development-branch/SKILL.md
│   └── writing-skills/               # Meta-skill
├── lib/skills-core.js                 # Core JS (208 lines, 0 deps)
├── commands/                          # Slash commands
├── agents/                            # Agent role definitions
├── hooks/                             # Session-start injection
├── .claude-plugin/                    # Claude Code plugin
├── .cursor-plugin/                    # Cursor plugin
├── .opencode/plugins/                 # OpenCode plugin
├── .codex/                            # Codex installation
├── docs/                              # Testing docs, plans
└── tests/                             # Integration tests
```

---

## Dependencies

- **Language**: Markdown (skills) + small JS (plugin code) + Bash (hooks, tests)
- **Runtime deps**: None. Requires a host AI platform (Claude Code, Cursor, OpenCode, Codex)
- **`lib/skills-core.js`**: Node.js builtins only (`fs`, `path`, `child_process`), zero npm deps
- **Test deps**: `python3`, `jq`, `bash`, the AI agent itself

---

## Patterns Adaptable to a Copilot CLI Workflow

### 1. Skill-as-Behavioral-Contract
Control AI agent behavior through structured Markdown with clear triggers, checklists, and "iron laws." For gopilot:
- `triaging-github-issues` — triggered when new issue assigned
- `sprint-planning` — triggered at sprint start
- `pr-review-workflow` — triggered on PR creation
- `release-management` — triggered for releases

### 2. Subagent Orchestration
Coordinator dispatches fresh, stateless subagents per task with multi-stage review. For gopilot:
- **Issue-worker** subagent per GitHub issue
- **Review** subagent verifies work matches issue requirements
- **QA** subagent runs verification
- Coordinator manages queue and tracks progress in GitHub Projects

### 3. Automatic Skill Activation
Skills checked before every action — even 1% chance means invoke. For gopilot:
- Match GitHub webhook events to workflow skills
- Match issue labels to skill triggers
- Never start work without loading appropriate workflow

### 4. Plan-as-Artifact
Plans saved as versioned documents with goal, architecture, and bite-sized tasks (exact file paths, verification commands). For gopilot:
- Sprint plans as structured docs with clear tasks
- Each task includes acceptance criteria and test commands
- Plans versioned in git, linked to GitHub Project iterations

### 5. Anti-Rationalization Defense
Every rigid skill includes:
- **Iron Law** (non-negotiable rule)
- **Red Flags** (thoughts that mean STOP)
- **Common Rationalizations** table (excuse → reality)
- Explicit closure of every loophole

For gopilot: prevent agents from skipping tests, merging without review, or working on main.

### 6. TDD-for-Process
Apply TDD to workflow definitions: write failing test (agent without skill fails), write minimal skill (passes), refactor (close loopholes). Use this to iteratively develop and harden gopilot workflows.

### 7. Context Injection at Session Start
Critical instructions injected via system prompt transformation at every session start. For gopilot: inject project conventions, active sprint context, and assigned issues into every agent invocation.

### 8. Shadowing/Override
Personal skills override defaults without forking:
- Default workflows for all projects
- Per-project overrides for team conventions
- Per-user overrides for individual preferences

---

## Key Takeaways for Gopilot

1. **The system works with zero runtime code** — skills are just structured prompts. This means gopilot could define workflows as Markdown files that any AI agent platform can consume.

2. **Subagent orchestration is the killer feature** — fresh context per task, multi-stage review, coordinator pattern. This maps directly to "assign issue → dispatch worker → review → merge."

3. **The anti-rationalization patterns are battle-tested** — AI agents WILL try to skip steps. Superpowers has iterated extensively on preventing this. Gopilot should adopt these patterns.

4. **GitHub integration is minimal in Superpowers** — this is a gap gopilot can fill. Superpowers focuses on local dev workflow; gopilot could extend this to issue management, sprint planning, and project tracking via GitHub Projects v2 + sub-issues.

5. **Cost is manageable** — at ~$0.07-$0.09 per subagent dispatch, automated issue resolution is economically viable for many tasks.
