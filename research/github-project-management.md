# GitHub Native Project Management Features

Researched: 2026-03-05

## GitHub Projects (v2)

Flexible, spreadsheet-like planning tool that lives **outside** any single repository. Owned by a **user** or **organization**, pulling in issues/PRs from multiple repos.

### Views

| View Type | Description |
|-----------|-------------|
| **Table** | Spreadsheet-style. Rows are items, columns are fields. Sortable, filterable, groupable. |
| **Board** | Kanban-style. Cards grouped into columns by any single-select field (commonly Status). |
| **Roadmap** | Timeline/Gantt-style. Items positioned on date axis using date or iteration fields. |

Multiple saved views per project, each with its own layout, filters, grouping, and sorting.

### Custom Fields

| Field Type | Description |
|-----------|-------------|
| **Text** | Free-form text |
| **Number** | Numeric (story points, effort) |
| **Date** | Calendar date (deadlines, start/end) |
| **Single Select** | Dropdown (Status, Priority, Size, Team) |
| **Iteration** | First-class sprint/iteration planning |

### Iterations (Sprints)

- Define iteration durations (e.g., 2-week sprints)
- Auto-generates future iterations based on cadence
- Supports breaks between iterations
- Items assigned to iterations; filter/group views by iteration
- Roadmap view can use iterations for timeline positioning
- Must be created via web UI or GraphQL API (not `gh` CLI `field-create`)

### Automation / Built-in Workflows

- **Auto-add**: Automatically add issues/PRs matching a filter from linked repos
- **Item added**: Set field value when item added (e.g., Status → Todo)
- **Item reopened**: Set field value on reopen
- **Item closed**: Set field value on close (e.g., Status → Done)
- **PR merged**: Set field value on merge
- **Auto-archive**: Archive items matching criteria (e.g., closed > 14 days)
- **Auto-close issue**: Close issue when reaching a certain status
- **GitHub Actions**: `actions/add-to-project` action + GraphQL API for custom automation

### Visibility & Templates

- Public or Private (`gh project edit --visibility PUBLIC|PRIVATE`)
- Org projects can be marked as templates (`gh project mark-template`)

---

## Sub-Issues (2024-2025, newer feature)

Native parent-child issue relationships — true hierarchy for epic/story/task breakdown.

- Any issue can have **parent** and **child** relationships
- Creates hierarchy: Epic → Story → Sub-task (arbitrary depth)
- Parent issue shows **progress bar** of children
- **Cross-repo** — parent and child can be in different repositories
- In Projects v2, group/filter by parent issue for epic-level tracking
- Sub-issues inherit or can be independently added to projects

### API Support
- REST API and GraphQL API endpoints available
- `gh` CLI does not yet have dedicated sub-issue subcommands; use `gh api`

---

## Milestones

- **Repository-scoped only** — cannot span multiple repos
- Title, optional description, optional due date
- Issues/PRs assigned to at most one milestone
- Progress bar (open vs closed ratio)
- Can be open or closed
- CLI: `gh issue create --milestone "v1.0"`, `gh issue list --milestone "v1.0"`

---

## Labels

- **Repository-scoped** — each repo has its own set
- Orgs can define default labels for new repos
- Name, color, optional description
- Issues/PRs can have multiple labels
- No hierarchy, no progress tracking, no parent-child relationships
- CLI: `gh label create`, `gh label list`, `gh issue create --label "bug"`

---

## Scoping Summary

| Feature | Repo | User | Org | Cross-Repo |
|---------|------|------|-----|------------|
| **Projects v2** | — | Yes | Yes | **Yes** |
| **Sub-Issues** | Yes | — | — | **Yes** |
| **Milestones** | **Yes only** | — | — | No |
| **Labels** | **Yes only** | — | — | No |

---

## CLI Support (`gh` CLI)

### Projects v2 Commands

| Command | Purpose |
|---------|---------|
| `gh project create` | Create a new project |
| `gh project list` | List projects for an owner |
| `gh project view` | View project details |
| `gh project edit` | Edit title, description, visibility |
| `gh project close` / `delete` | Close or delete |
| `gh project copy` | Copy project structure |
| `gh project link` / `unlink` | Link/unlink to repo or team |
| `gh project mark-template` | Mark as template |
| `gh project field-create` | Create custom fields (TEXT, SINGLE_SELECT, DATE, NUMBER) |
| `gh project field-delete` / `field-list` | Delete or list fields |
| `gh project item-add` | Add issue/PR to project |
| `gh project item-create` | Create draft issue in project |
| `gh project item-edit` | Edit field values on item |
| `gh project item-delete` / `item-archive` | Delete or archive item |
| `gh project item-list` | List items with filter queries |

### Query Syntax Examples

```bash
gh project item-list 1 --owner "@me" --query "assignee:monalisa"
gh project item-list 1 --owner "@me" --query "label:bug -status:Done"
gh project item-list 1 --owner "@me" --query "assignee:@me is:issue is:open"
```

### GraphQL API

Full programmatic access via `ProjectV2` types:
- Types: `ProjectV2`, `ProjectV2Item`, `ProjectV2Field`, `ProjectV2IterationField`, `ProjectV2View`
- Mutations: `createProjectV2`, `updateProjectV2`, `addProjectV2ItemById`, `updateProjectV2ItemFieldValue`, etc.
- Execute via `gh api graphql -f query='...'`

---

## Free vs Paid

| Feature | Free | Team | Enterprise |
|---------|------|------|-----------|
| Projects v2 (boards, tables, roadmap, fields, iterations) | Yes | Yes | Yes |
| Sub-Issues (basic) | Yes | Yes | Yes |
| Milestones, Labels | Yes | Yes | Yes |
| Built-in workflows | Yes | Yes | Yes |
| Issue Types (Bug, Feature, Task) | — | Yes | Yes |
| Project insights/charts | Limited | Yes | Yes |
| Advanced roadmap features | — | — | Yes |

Core functionality is available on **all plans including Free**. Paid plans add higher item limits and org-level features.

---

## Recommended Architecture for Copilot Task Orchestration

1. **Org-level or User-level Project v2** with custom fields: Status (Todo/In Progress/In Review/Done), Priority, Iteration
2. **Sub-issues** for epic → story → task hierarchy
3. **Iterations** for sprint planning
4. **Board view** for daily work, **Table view** for backlog, **Roadmap view** for stakeholder visibility
5. **Auto-add workflows** to pull issues from linked repos automatically
6. **`gh` CLI + GraphQL API** for all automation and integration
