# Verification

Before declaring your work complete, you MUST verify it:

1. **Compile check** — ensure the code compiles without errors (`go build ./...` or equivalent).
2. **Run tests** — execute the full test suite and confirm all tests pass.
3. **Lint check** — run `go vet ./...` (or equivalent linter) and fix any warnings.
4. **Manual review** — re-read the issue description and confirm your changes address all requirements.
5. **No regressions** — verify that existing functionality still works correctly.

## Rules

- Do NOT skip verification steps even if you are confident in your changes.
- If a test fails, fix the issue before proceeding.
- If you added new public APIs, ensure they have tests.
- If you modified existing behavior, verify affected tests are updated.
