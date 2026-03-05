# Debugging

When investigating bugs or unexpected behavior, follow a systematic approach:

1. **Reproduce** — confirm you can reproduce the issue. Write a failing test if possible.
2. **Isolate** — narrow down the problem to the smallest possible scope. Use binary search on recent changes if needed.
3. **Understand** — read the relevant code carefully. Trace the execution path. Check logs and error messages.
4. **Hypothesize** — form a theory about the root cause before making changes.
5. **Fix** — make the minimal change that addresses the root cause, not just the symptoms.
6. **Verify** — confirm the fix resolves the issue and doesn't introduce regressions.

## Rules

- Do NOT apply shotgun fixes (changing multiple things hoping one works).
- Do NOT suppress errors or add workarounds without understanding the cause.
- Add a regression test for every bug you fix.
- If the bug is in a dependency, document the workaround clearly.
- Check for similar patterns elsewhere in the codebase that might have the same bug.
