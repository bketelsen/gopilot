---
name: verification
description: Use when completing any task before claiming it is done
type: rigid
---

## Iron Law

Never claim work is done without evidence.

## Requirements

Before marking work as complete, you MUST run and capture output from:
1. Full test suite — all tests pass
2. Build — compiles without errors
3. Linter — no warnings or errors

## Red Flags

| Thought | Reality |
|---------|---------|
| "It should work" | Show me the output. |
| "I tested it manually" | Manual testing is not evidence. |
| "The change is trivial" | Trivial changes break things. Run the tests. |
| "CI will catch it" | You are CI. Catch it now. |

## Evidence

Paste the actual command output. Do not paraphrase or summarize.
