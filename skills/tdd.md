# Test-Driven Development

You MUST follow TDD discipline for all code changes:

1. **Write a failing test first** — before writing any implementation code, write a test that captures the expected behavior. Run it to confirm it fails.
2. **Write the minimum implementation** — write only enough code to make the failing test pass. Do not add extra functionality.
3. **Refactor** — once the test passes, clean up the code while keeping all tests green.
4. **Repeat** — for each new behavior or edge case, start with a new failing test.

## Rules

- Never write implementation code without a corresponding test.
- Run the full test suite after each change to ensure no regressions.
- If you find a bug, write a test that reproduces it before fixing it.
- Test edge cases: empty inputs, nil values, boundary conditions, error paths.
- Prefer table-driven tests for multiple input/output scenarios.
