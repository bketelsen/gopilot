---
name: debugging
description: >-
  Systematic debugging process for any bug, test failure, or unexpected
  behavior. Use when encountering errors or when code does not behave as expected.
---

## Workflow

1. **Reproduce** — Confirm the bug exists with a minimal reproduction
2. **Isolate** — Narrow down to the specific component or line
3. **Root Cause** — Identify WHY it happens, not just WHERE
4. **Fix** — Apply the minimal fix for the root cause
5. **Verify** — Confirm the fix resolves the issue
6. **Regression** — Run the full test suite to ensure no regressions

## Requirements

- Root cause MUST be identified before any fix is attempted
- Do not guess and check — trace the execution path
- Write a test that reproduces the bug BEFORE fixing it
