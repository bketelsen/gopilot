---
name: tdd
description: >-
  Test-driven development workflow for implementing code changes, features,
  or bug fixes. Use when writing any new code or modifying existing behavior.
---

> This is a strict workflow. Follow each step exactly as written.

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
