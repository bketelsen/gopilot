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
