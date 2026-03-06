# Planning Chat Event Parsing Design

## Goal

Parse Claude Code's `--output-format stream-json` output to show a full activity stream in the planning chat — tool calls, thinking, text, and usage summaries — instead of raw text lines.

## Background

The planning chat currently streams raw stdout lines from the agent subprocess. Claude Code's `--print` mode outputs plain text by default, hiding all tool activity and thinking. By switching to `stream-json` (requires `--verbose`), we get structured JSON events that we can parse and render richly.

## Claude stream-json Event Types

| Event Type | Subtype | Action |
|---|---|---|
| `system` | `hook_started` | Drop (noise) |
| `system` | `hook_response` | Drop (huge payloads) |
| `system` | `init` | Drop (session metadata) |
| `assistant` | — | Keep: contains `content[]` blocks (text, tool_use, thinking) |
| `tool_result` | — | Keep: tool output, correlated by `tool_use_id` |
| `rate_limit_event` | — | Drop |
| `result` | `success`/`error` | Keep: duration, cost, token usage |

## Changes

### 1. Claude Runner (`internal/agent/claude.go`)

Add `--output-format stream-json --verbose` to `buildArgs`. No other changes — stdout is already piped.

### 2. Server-side Event Parsing (`internal/planning/handler.go`)

The scanner goroutine in `runAgentTurn` changes from raw line processing to JSON-aware parsing:

1. `json.Unmarshal` each line into `map[string]any`
2. Filter: drop `system/*`, `rate_limit_event`
3. Keep: `assistant`, `tool_result`, `result`, unknown types
4. Flatten `assistant` events: extract `message.content` array as `content_blocks`
5. Cherry-pick `result` fields: `duration_ms`, `num_turns`, `total_cost_usd`, `usage`, `result`, `subtype`
6. Tag with `"source":"claude"`
7. Send over WebSocket as `WSMessage{Type: "agent_event", Content: <json>}`

Text content blocks are also accumulated into `fullResponse` for session history.

If a line fails to parse as JSON (e.g., Copilot plain text), fall back to sending as `WSMessage{Type: "agent", Content: line}` — backwards compatible.

### 3. Session History (`internal/planning/session.go`)

Add `Events []json.RawMessage` to `ChatMessage`:

```go
type ChatMessage struct {
    Role      string
    Content   string            // plain text summary
    Events    []json.RawMessage // raw agent events for rich replay
    Timestamp time.Time
}
```

On reconnect, agent messages replay their `Events` as `agent_event` WebSocket messages. Falls back to plain `agent` message if `Events` is empty (old sessions).

### 4. Browser Rendering (`internal/web/templates/pages/planning_chat.templ`)

New `agent_event` case in `ws.onmessage`. Parses `msg.content` as JSON, dispatches by type:

**Text blocks** (`content_blocks` where `type === "text"`):
- Appended to streaming `<pre>` like today

**Tool use blocks** (`content_blocks` where `type === "tool_use"`):
- Collapsed `<details>` element
- Summary: tool icon + tool name + key input (file path, pattern, command)
- Body: populated when matching `tool_result` arrives
- Correlated by `tool_use_id` via a `Map<id, DOM element>`

**Thinking blocks** (`content_blocks` where `type === "thinking"`):
- Collapsed `<details>`, italic, dimmed opacity
- Summary: "Thinking..."
- Body: reasoning text

**Tool results** (top-level `type === "tool_result"`):
- Find matching tool_use `<details>` by `tool_use_id`, populate its body
- If no match, append inline

**Result summary** (top-level `type === "result"`):
- Collapsed `<details>` footer after turn completes
- Summary: duration, token count, cost
- Body: full breakdown (input/output/cache tokens, model, turns)

### 5. Copilot (No Changes)

Copilot continues outputting plain text. The handler sends these as `WSMessage{Type: "agent"}`. Browser handles both `agent` (plain text) and `agent_event` (structured) message types. Copilot structured output is future work.

## WebSocket Message Types (Updated)

| Type | Direction | Purpose |
|---|---|---|
| `message` | Browser → Server | User chat input |
| `cancel` | Browser → Server | Disconnect |
| `user` | Server → Browser | User message (replay) |
| `agent` | Server → Browser | Plain text agent line (Copilot, legacy) |
| `agent_event` | Server → Browser | Structured agent event (Claude stream-json) |
| `status` | Server → Browser | Session status (pending/active/done/failed) |
| `error` | Server → Browser | Error notification |

## Non-Goals

- Markdown rendering in the browser (stays as `<pre>` for now)
- Token-by-token streaming (`--include-partial-messages` not used)
- Copilot structured output parsing
- Changes to the main orchestrator's `scanOutput` (dashboard SSE)
