# Planning Chat Event Parsing - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Parse Claude Code's stream-json output to show tool calls, thinking, and usage summaries in the planning chat UI.

**Architecture:** Server-side filtering + minimal transformation (Approach 3). Claude runner adds `--output-format stream-json --verbose`. Handler parses JSON lines, drops noise, tags with `source`, forwards kept events as `agent_event` WebSocket messages. Browser renders each event type with appropriate UI (collapsible tool calls, dimmed thinking, expandable result summary).

**Tech Stack:** Go (server parsing), vanilla JS in templ (browser rendering), existing WebSocket infra.

---

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

---

# Implementation Plan

## Task 1: Add stream-json flags to Claude runner

**Files:**
- Modify: `internal/agent/claude.go:95-105`
- Test: `internal/agent/claude_test.go`

**Step 1: Write the failing test**

Add to `internal/agent/claude_test.go`:

```go
func TestClaudeBuildArgsStreamJSON(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	args := runner.buildArgs("/tmp/ws/.gopilot-prompt.md", AgentOpts{})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--output-format stream-json") {
		t.Error("missing --output-format stream-json flag")
	}
	if !strings.Contains(joined, "--verbose") {
		t.Error("missing --verbose flag")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestClaudeBuildArgsStreamJSON ./internal/agent/...`
Expected: FAIL — flags not present yet.

**Step 3: Write minimal implementation**

In `internal/agent/claude.go`, replace `buildArgs`:

```go
func (r *ClaudeRunner) buildArgs(promptPath string, opts AgentOpts) []string {
	args := []string{
		"--print", promptPath,
		"--output-format", "stream-json",
		"--verbose",
	}
	if opts.ReadOnly {
		args = append(args, "--permission-mode", "plan")
	} else {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/agent/...`
Expected: ALL PASS (including existing TestClaudeBuildArgs and TestClaudeBuildArgsReadOnly).

**Step 5: Commit**

```bash
git add internal/agent/claude.go internal/agent/claude_test.go
git commit -m "feat: add stream-json output format to Claude runner"
```

---

## Task 2: Add Events field to ChatMessage

**Files:**
- Modify: `internal/planning/session.go:19-23`
- Modify: `internal/planning/session.go:38-46` (AddMessage + new AddAgentMessage)

**Step 1: Add Events field and new method**

In `internal/planning/session.go`, update `ChatMessage` struct:

```go
type ChatMessage struct {
	Role      string            `json:"role"`    // "user" or "agent"
	Content   string            `json:"content"` // plain text (user msgs + agent text summary)
	Events    []json.RawMessage `json:"events,omitempty"` // structured agent events for rich replay
	Timestamp time.Time         `json:"timestamp"`
}
```

Add `encoding/json` to the imports.

Add a new method below `AddMessage`:

```go
// AddAgentMessage appends an agent message with both text summary and raw events.
func (s *Session) AddAgentMessage(content string, events []json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ChatMessage{
		Role:      "agent",
		Content:   content,
		Events:    events,
		Timestamp: time.Now(),
	})
}
```

**Step 2: Run existing tests to verify nothing breaks**

Run: `go test -race ./internal/planning/...`
Expected: ALL PASS — existing code still uses `AddMessage` which still works.

**Step 3: Commit**

```bash
git add internal/planning/session.go
git commit -m "feat: add Events field to ChatMessage for rich replay"
```

---

## Task 3: Server-side event parsing in handler

**Files:**
- Modify: `internal/planning/handler.go` (the scanner goroutine in `runAgentTurn`, replay in `HandleWebSocket`)
- Test: `internal/planning/handler_test.go`

**Step 1: Write the failing test for event parsing**

Add to `internal/planning/handler_test.go`:

```go
func TestHandler_StreamJSONParsing(t *testing.T) {
	mgr := planning.NewManager()
	sess, _ := mgr.Create("owner/repo", nil)

	// Simulate Claude stream-json output: system (dropped), assistant (kept), result (kept)
	output := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"abc"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Here is my analysis."}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"main.go"}}]}}`,
		`{"type":"tool_result","tool_use_id":"toolu_1","content":"package main\n"}`,
		`{"type":"rate_limit_event","rate_limit_info":{}}`,
		`{"type":"result","subtype":"success","duration_ms":5000,"num_turns":1,"total_cost_usd":0.05,"usage":{"input_tokens":100,"output_tokens":50},"result":"Here is my analysis."}`,
	}, "\n") + "\n"

	h := planning.NewHandler(mgr, &fakeRunner{output: output}, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.HandleWebSocket(w, r, sess.ID)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	// Read initial status
	_, data, _ := conn.Read(ctx)
	var statusMsg planning.WSMessage
	json.Unmarshal(data, &statusMsg)
	if statusMsg.Type != "status" {
		t.Fatalf("expected status, got %s", statusMsg.Type)
	}

	// Send user message to trigger agent turn
	msgData, _ := json.Marshal(planning.WSMessage{Type: "message", Content: "analyze"})
	conn.Write(ctx, websocket.MessageText, msgData)

	// Collect all messages until status=pending
	var agentEvents []string
	var gotPlainAgent bool
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg planning.WSMessage
		json.Unmarshal(data, &msg)

		if msg.Type == "agent_event" {
			agentEvents = append(agentEvents, msg.Content)
		}
		if msg.Type == "agent" {
			gotPlainAgent = true
		}
		if msg.Type == "status" && msg.Status == "pending" {
			break
		}
	}

	// Should have received agent_event messages for: assistant(text), assistant(tool_use), tool_result, result
	// Should NOT have received: system/init, rate_limit_event
	if len(agentEvents) != 4 {
		t.Errorf("expected 4 agent_events, got %d", len(agentEvents))
		for i, e := range agentEvents {
			t.Logf("  event[%d]: %s", i, e)
		}
	}
	if gotPlainAgent {
		t.Error("should not have received plain 'agent' messages for JSON input")
	}

	// Verify each event has source:"claude" (but we check the first one)
	if len(agentEvents) > 0 {
		var first map[string]any
		json.Unmarshal([]byte(agentEvents[0]), &first)
		if first["source"] != "claude" {
			t.Errorf("expected source=claude, got %v", first["source"])
		}
	}

	// Verify session saved events for replay
	got := mgr.Get(sess.ID)
	var agentMsg *planning.ChatMessage
	for i := range got.Messages {
		if got.Messages[i].Role == "agent" {
			agentMsg = &got.Messages[i]
			break
		}
	}
	if agentMsg == nil {
		t.Fatal("no agent message saved")
	}
	if len(agentMsg.Events) != 4 {
		t.Errorf("expected 4 saved events, got %d", len(agentMsg.Events))
	}
	if !strings.Contains(agentMsg.Content, "Here is my analysis") {
		t.Errorf("text content not extracted, got: %s", agentMsg.Content)
	}
}
```

Note: `ChatMessage` fields are accessed via exported `Messages` slice. You need to export `ChatMessage` pointer access — check that `planning.ChatMessage` is already exported (it is — the struct and its fields are exported).

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestHandler_StreamJSONParsing ./internal/planning/...`
Expected: FAIL — handler still sends `agent` not `agent_event`.

**Step 3: Implement event parsing in handler.go**

Replace the scanner goroutine and post-processing in `runAgentTurn` (lines 123-153). The new implementation:

```go
	var fullResponse strings.Builder
	var events []json.RawMessage
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			// Try to parse as JSON (Claude stream-json)
			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				// Not JSON — plain text fallback (e.g. Copilot)
				fullResponse.WriteString(line)
				fullResponse.WriteString("\n")
				writeJSON(ctx, conn, WSMessage{Type: "agent", Content: line})
				continue
			}

			eventType, _ := raw["type"].(string)

			// Filter noise
			switch eventType {
			case "system", "rate_limit_event":
				continue
			}

			// Transform and forward kept events
			out := map[string]any{"source": "claude", "type": eventType}

			switch eventType {
			case "assistant":
				// Flatten message.content into content_blocks
				if msg, ok := raw["message"].(map[string]any); ok {
					if content, ok := msg["content"].([]any); ok {
						out["content_blocks"] = content
						// Extract text for fullResponse
						for _, block := range content {
							if b, ok := block.(map[string]any); ok {
								if b["type"] == "text" {
									if text, ok := b["text"].(string); ok {
										fullResponse.WriteString(text)
										fullResponse.WriteString("\n")
									}
								}
							}
						}
					}
				}
			case "tool_result":
				out["tool_use_id"] = raw["tool_use_id"]
				out["content"] = raw["content"]
			case "result":
				for _, key := range []string{"subtype", "duration_ms", "num_turns", "total_cost_usd", "usage", "result"} {
					if v, ok := raw[key]; ok {
						out[key] = v
					}
				}
				// Extract result text for fullResponse if not already captured
				if resultText, ok := raw["result"].(string); ok {
					_ = resultText // already captured from assistant events
				}
			default:
				// Unknown type — forward as-is with source tag
				raw["source"] = "claude"
				out = raw
			}

			eventJSON, err := json.Marshal(out)
			if err != nil {
				continue
			}
			events = append(events, json.RawMessage(eventJSON))
			writeJSON(ctx, conn, WSMessage{Type: "agent_event", Content: string(eventJSON)})
		}
	}()

	<-agentSess.Done
	pw.Close()
	<-done

	if agentSess.ExitErr != nil {
		slog.Error("planning agent exited with error", "session", sess.ID, "error", agentSess.ExitErr, "exit_code", agentSess.ExitCode)
		writeJSON(ctx, conn, WSMessage{Type: "error", Content: fmt.Sprintf("agent exited with error (code %d): %v", agentSess.ExitCode, agentSess.ExitErr)})
	} else {
		slog.Info("planning agent completed", "session", sess.ID)
	}

	response := strings.TrimSpace(fullResponse.String())
	if len(events) > 0 {
		sess.AddAgentMessage(response, events)
	} else if response != "" {
		sess.AddMessage("agent", response)
	}
```

**Step 4: Update replay in HandleWebSocket**

Replace the replay loop (lines 65-69):

```go
	// Replay existing messages for reconnect
	sess.mu.Lock()
	for _, msg := range sess.Messages {
		if msg.Role == "agent" && len(msg.Events) > 0 {
			for _, event := range msg.Events {
				writeJSON(ctx, conn, WSMessage{Type: "agent_event", Content: string(event)})
			}
		} else {
			writeJSON(ctx, conn, WSMessage{Type: msg.Role, Content: msg.Content})
		}
	}
	sess.mu.Unlock()
```

**Step 5: Run tests**

Run: `go test -race ./internal/planning/...`
Expected: ALL PASS including new test and existing tests.

Note: The existing `TestHandler_WebSocketChat` uses `fakeRunner` with plain text `"I'll explore the codebase.\n"` — this will fall through to the plain text fallback path and still send `agent` messages, so it keeps passing.

The existing `TestHandler_ReplayMessages` uses `AddMessage` which produces `Events: nil` — the replay fallback path handles this, so it keeps passing.

**Step 6: Commit**

```bash
git add internal/planning/handler.go internal/planning/handler_test.go
git commit -m "feat: parse Claude stream-json events in planning handler"
```

---

## Task 4: Test replay with events

**Files:**
- Test: `internal/planning/handler_test.go`

**Step 1: Write the test**

```go
func TestHandler_ReplayWithEvents(t *testing.T) {
	mgr := planning.NewManager()
	sess, _ := mgr.Create("owner/repo", nil)

	// Simulate a session with prior agent events
	sess.AddMessage("user", "analyze the code")
	event1, _ := json.Marshal(map[string]any{"source": "claude", "type": "assistant", "content_blocks": []any{map[string]any{"type": "text", "text": "analysis"}}})
	event2, _ := json.Marshal(map[string]any{"source": "claude", "type": "result", "duration_ms": 1000})
	sess.AddAgentMessage("analysis", []json.RawMessage{event1, event2})

	h := planning.NewHandler(mgr, &fakeRunner{}, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.HandleWebSocket(w, r, sess.ID)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	// Expect: user msg, 2 agent_events, then status
	var msgs []planning.WSMessage
	for i := 0; i < 4; i++ {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg planning.WSMessage
		json.Unmarshal(data, &msg)
		msgs = append(msgs, msg)
	}

	if msgs[0].Type != "user" {
		t.Errorf("msgs[0]: expected user, got %s", msgs[0].Type)
	}
	if msgs[1].Type != "agent_event" {
		t.Errorf("msgs[1]: expected agent_event, got %s", msgs[1].Type)
	}
	if msgs[2].Type != "agent_event" {
		t.Errorf("msgs[2]: expected agent_event, got %s", msgs[2].Type)
	}
	if msgs[3].Type != "status" {
		t.Errorf("msgs[3]: expected status, got %s", msgs[3].Type)
	}
}
```

**Step 2: Run test**

Run: `go test -race -run TestHandler_ReplayWithEvents ./internal/planning/...`
Expected: PASS (replay logic was implemented in Task 3).

**Step 3: Commit**

```bash
git add internal/planning/handler_test.go
git commit -m "test: verify event replay on WebSocket reconnect"
```

---

## Task 5: Browser rendering — agent_event handler and renderers

**Files:**
- Modify: `internal/web/templates/pages/planning_chat.templ:50-200`

**Step 1: Add toolResultMap and helper functions**

After the existing variable declarations (line 58 area), add:

```javascript
const toolResultMap = new Map(); // tool_use_id -> DOM element
```

**Step 2: Add the agent_event case to ws.onmessage**

In the `switch (msg.type)` block, after the `case 'agent':` block and before `case 'status':`, add:

```javascript
case 'agent_event':
    handleAgentEvent(JSON.parse(msg.content));
    break;
```

**Step 3: Add the handleAgentEvent function**

After the `escapeHTML` function, add:

```javascript
function handleAgentEvent(evt) {
    ensureAgentStreamingDiv();

    switch (evt.type) {
        case 'assistant':
            if (evt.content_blocks) {
                evt.content_blocks.forEach(function(block) {
                    switch (block.type) {
                        case 'text':
                            agentBuf += (block.text || '') + '\n';
                            document.getElementById('agent-streaming-content').textContent = agentBuf;
                            if (!hasPlan && agentBuf.includes('## Plan:')) {
                                hasPlan = true;
                                document.getElementById('export-buttons').classList.remove('hidden');
                            }
                            break;
                        case 'tool_use':
                            appendToolUse(block);
                            break;
                        case 'thinking':
                            appendThinking(block);
                            break;
                    }
                });
            }
            break;
        case 'tool_result':
            fillToolResult(evt);
            break;
        case 'result':
            appendResultSummary(evt);
            break;
    }
    messages.scrollTop = messages.scrollHeight;
}

function ensureAgentStreamingDiv() {
    if (!document.getElementById('agent-streaming')) {
        var div = document.createElement('div');
        div.id = 'agent-streaming';
        div.className = 'bg-gray-100 dark:bg-gray-700 rounded-lg p-3 max-w-[80%]';
        div.innerHTML = '<div class="text-xs text-purple-600 dark:text-purple-400 mb-1 font-medium">Agent</div><pre class="whitespace-pre-wrap text-sm" id="agent-streaming-content"></pre>';
        messages.appendChild(div);
    }
}

function toolSummary(block) {
    var name = block.name || 'Tool';
    var detail = '';
    if (block.input) {
        if (block.input.file_path) detail = block.input.file_path;
        else if (block.input.pattern) detail = block.input.pattern;
        else if (block.input.command) detail = block.input.command.substring(0, 80);
        else if (block.input.query) detail = block.input.query;
        else if (block.input.prompt) detail = block.input.prompt.substring(0, 60);
        else if (block.input.description) detail = block.input.description;
    }
    return name + (detail ? ' \u203a ' + detail : '');
}

function appendToolUse(block) {
    var container = document.getElementById('agent-streaming');
    if (!container) return;
    var details = document.createElement('details');
    details.className = 'my-1 text-sm';
    details.id = 'tool-' + (block.id || '');
    var summary = document.createElement('summary');
    summary.className = 'cursor-pointer text-blue-600 dark:text-blue-400';
    summary.textContent = '\uD83D\uDD27 ' + toolSummary(block);
    var pre = document.createElement('pre');
    pre.className = 'text-xs bg-gray-50 dark:bg-gray-900 p-2 mt-1 max-h-40 overflow-auto rounded';
    pre.textContent = 'Waiting for result...';
    details.appendChild(summary);
    details.appendChild(pre);
    container.appendChild(details);
    if (block.id) {
        toolResultMap.set(block.id, pre);
    }
}

function appendThinking(block) {
    var container = document.getElementById('agent-streaming');
    if (!container) return;
    var details = document.createElement('details');
    details.className = 'my-1 text-sm italic opacity-60';
    var summary = document.createElement('summary');
    summary.className = 'cursor-pointer text-gray-500 dark:text-gray-400';
    summary.textContent = '\uD83E\uDDE0 Thinking...';
    var pre = document.createElement('pre');
    pre.className = 'text-xs p-2 mt-1 max-h-40 overflow-auto not-italic';
    pre.textContent = block.thinking || '';
    details.appendChild(summary);
    details.appendChild(pre);
    container.appendChild(details);
}

function fillToolResult(evt) {
    var pre = toolResultMap.get(evt.tool_use_id);
    if (pre) {
        var content = evt.content;
        if (typeof content !== 'string') {
            content = JSON.stringify(content, null, 2);
        }
        pre.textContent = content;
        toolResultMap.delete(evt.tool_use_id);
    }
}

function formatTokens(n) {
    if (n >= 1000) return (n / 1000).toFixed(1) + 'k';
    return String(n);
}

function appendResultSummary(evt) {
    var container = document.getElementById('agent-streaming');
    if (!container) return;
    var dur = evt.duration_ms ? (evt.duration_ms / 1000).toFixed(1) + 's' : '?';
    var tokens = '?';
    var cost = evt.total_cost_usd ? '$' + evt.total_cost_usd.toFixed(4) : '?';
    if (evt.usage) {
        tokens = formatTokens((evt.usage.input_tokens || 0) + (evt.usage.output_tokens || 0));
    }

    var details = document.createElement('details');
    details.className = 'my-2 text-xs text-gray-400 dark:text-gray-500';
    var summary = document.createElement('summary');
    summary.className = 'cursor-pointer';
    summary.textContent = '\u23F1 ' + dur + ' \u00b7 ' + tokens + ' tokens \u00b7 ' + cost;

    var body = document.createElement('div');
    body.className = 'p-2 mt-1 bg-gray-50 dark:bg-gray-900 rounded text-xs';
    var lines = [];
    if (evt.usage) {
        lines.push('Input: ' + formatTokens(evt.usage.input_tokens || 0));
        lines.push('Output: ' + formatTokens(evt.usage.output_tokens || 0));
        if (evt.usage.cache_read_input_tokens) lines.push('Cache read: ' + formatTokens(evt.usage.cache_read_input_tokens));
        if (evt.usage.cache_creation_input_tokens) lines.push('Cache created: ' + formatTokens(evt.usage.cache_creation_input_tokens));
    }
    if (evt.num_turns) lines.push('Turns: ' + evt.num_turns);
    body.textContent = lines.join(' \u00b7 ');

    details.appendChild(summary);
    details.appendChild(body);
    container.appendChild(details);
}
```

**Step 4: Run templ generate and build**

Run: `task generate && task build`
Expected: Compiles without errors.

**Step 5: Commit**

```bash
git add internal/web/templates/pages/planning_chat.templ internal/web/templates/pages/planning_chat_templ.go
git commit -m "feat: browser rendering for structured agent events"
```

---

## Task 6: Lint, full test suite, manual verification

**Step 1: Run linter**

Run: `task lint`
Expected: No errors. Fix any issues.

**Step 2: Run full test suite**

Run: `task test`
Expected: ALL PASS.

**Step 3: Build and run**

Run: `task build && ./gopilot`

**Step 4: Manual test in browser**

1. Open `http://10.0.1.58:3000/planning`
2. Create a new planning session
3. Send a message like "Explore the codebase and make a plan to add HTTP basic auth"
4. Verify you see:
   - Text streaming in as before
   - Collapsed tool use entries (Read, Grep, Glob) with tool name + key input
   - Clicking a tool use expands to show the result
   - Thinking blocks appear dimmed and collapsed
   - Result summary appears at end with duration/tokens/cost
   - Clicking result summary expands to full breakdown
5. Test reconnect: refresh the page and verify events replay correctly

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: polish planning chat event rendering"
```
