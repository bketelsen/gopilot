# Pull Request Workflow

When your work is complete, create a pull request following this workflow:

1. **Create a feature branch** — use the naming convention `gopilot/issue-{number}`.
2. **Make atomic commits** — each commit should represent a logical unit of work with a clear message.
3. **Write a PR description** that includes:
   - A summary of changes
   - Reference to the issue (e.g., "Fixes #42")
   - A test plan describing how to verify the changes
4. **Self-review** — read through your own diff before submitting. Look for:
   - Leftover debug code or TODO comments
   - Accidental file changes
   - Missing error handling
5. **Create the PR** targeting the default branch.

## Rules

- Always reference the issue number in the PR title or body.
- Keep PRs focused — one issue per PR.
- Do not include unrelated changes or refactors.
- Ensure CI passes before requesting review.
