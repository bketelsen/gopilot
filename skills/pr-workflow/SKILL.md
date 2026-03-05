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
