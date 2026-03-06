# Copilot Structured Event Normalization

**Date:** 2026-03-06

## Goal

Add `--output-format json` to the Copilot CLI runner and normalize its JSONL events into the same shape the frontend already handles for Claude, so both agents get the same rich UI (collapsible tool calls, result summaries).

## Event Mapping

| Copilot Event | Action | Normalized Output |
|---|---|---|
| `user.message` | Filter | — |
| `assistant.turn_start` | Filter | — |
| `assistant.message_delta` | Filter (ephemeral) | — |
| `assistant.message` | Transform | `{type: "assistant", source: "copilot", content_blocks: [...]}` |
| `tool.execution_start` | Filter | — |
| `tool.execution_complete` | Transform | `{type: "tool_result", source: "copilot", tool_use_id, content}` |
| `assistant.turn_end` | Filter | — |
| `result` | Transform | `{type: "result", source: "copilot", subtype, duration_ms, usage}` |

## Field Mapping

### `assistant.message` → normalized `assistant`

- `data.content` → text content block: `{type: "text", text: "..."}`
- `data.toolRequests[]` → tool_use content blocks: `{type: "tool_use", id: toolCallId, name: name, input: arguments}`
- Text block only added if `data.content` is non-empty

### `tool.execution_complete` → normalized `tool_result`

- `data.toolCallId` → `tool_use_id`
- `data.result.content` → `content`

### `result` → normalized `result`

- `exitCode` 0 → `subtype: "success"`, else `"error"`
- `usage.sessionDurationMs` → `duration_ms`
- `usage` forwarded as-is

## Changes

1. **`internal/agent/copilot.go`** — Add `--output-format json` to `buildArgs()`
2. **`internal/planning/handler.go`** — Detect runner name, branch parsing logic for copilot events
3. **No frontend changes** — same `handleAgentEvent()` JS handles both agents
