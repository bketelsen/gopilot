package planning

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bketelsen/gopilot/internal/agent"
	"nhooyr.io/websocket"
)

// WSMessage is the JSON message format for WebSocket communication.
type WSMessage struct {
	Type    string `json:"type"`              // message, agent, status, error, cancel
	Content string `json:"content"`
	Status  string `json:"status,omitempty"`
}

// HandlerConfig configures the planning WebSocket handler.
type HandlerConfig struct {
	WorkspaceRoot string
	SkillText     string
	GitHubClient  IssueCreator
}

// Handler manages WebSocket connections for planning sessions.
type Handler struct {
	mgr    *Manager
	runner agent.Runner
	cfg    HandlerConfig
}

// NewHandler creates a new planning WebSocket handler.
func NewHandler(mgr *Manager, runner agent.Runner, cfg HandlerConfig) *Handler {
	return &Handler{mgr: mgr, runner: runner, cfg: cfg}
}

// HandleWebSocket upgrades the HTTP connection and runs the chat loop.
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess := h.mgr.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}
	defer conn.CloseNow() //nolint:errcheck // best-effort close on exit

	ctx := r.Context()

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

	writeJSON(ctx, conn, WSMessage{Type: "status", Status: string(sess.Status)})

	for {
		var msg WSMessage
		err := readJSON(ctx, conn, &msg)
		if err != nil {
			return
		}

		switch msg.Type {
		case "message":
			sess.AddMessage("user", msg.Content)
			sess.SetStatus(StatusActive)
			writeJSON(ctx, conn, WSMessage{Type: "status", Status: "active"})
			h.runAgentTurn(ctx, conn, sess)
		case "cancel":
			return
		}
	}
}

func (h *Handler) runAgentTurn(ctx context.Context, conn *websocket.Conn, sess *Session) {
	prompt := h.buildPrompt(sess)

	wsDir := filepath.Join(h.cfg.WorkspaceRoot, sess.ID)
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		slog.Error("failed to create workspace dir", "path", wsDir, "error", err)
		writeJSON(ctx, conn, WSMessage{Type: "error", Content: fmt.Sprintf("workspace setup failed: %v", err)})
		sess.SetStatus(StatusFailed)
		return
	}

	pr, pw := io.Pipe()

	opts := agent.AgentOpts{
		Stdout:           pw,
		ReadOnly:         true,
		MaxContinuations: 5,
	}

	slog.Info("starting planning agent", "session", sess.ID, "workspace", wsDir)

	agentSess, err := h.runner.Start(ctx, wsDir, prompt, opts)
	if err != nil {
		slog.Error("planning agent start failed", "session", sess.ID, "error", err)
		writeJSON(ctx, conn, WSMessage{Type: "error", Content: fmt.Sprintf("agent start failed: %v", err)})
		sess.SetStatus(StatusFailed)
		pw.Close()
		return
	}

	slog.Info("planning agent started", "session", sess.ID, "pid", agentSess.PID)

	var fullResponse strings.Builder
	var events []json.RawMessage
	planDetected := false
	done := make(chan struct{})
	agentName := h.runner.Name()
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				// Not JSON — plain text fallback
				fullResponse.WriteString(line)
				fullResponse.WriteString("\n")
				writeJSON(ctx, conn, WSMessage{Type: "agent", Content: line})
				continue
			}

			eventType, _ := raw["type"].(string)

			// Stop agent after it delivers a plan (on next turn boundary)
			if planDetected && (eventType == "assistant.turn_end" || eventType == "result") {
				slog.Info("plan detected, stopping agent after turn end", "session", sess.ID)
				h.runner.Stop(agentSess) //nolint:errcheck // best-effort stop after plan delivery
			}

			var out map[string]any
			var text string
			if agentName == "copilot" {
				out, text = transformCopilotEvent(raw)
			} else {
				out, text = transformClaudeEvent(raw)
			}
			if out == nil {
				continue
			}

			if text != "" {
				fullResponse.WriteString(text)
				fullResponse.WriteString("\n")
				if !planDetected && containsPlanTitle(fullResponse.String()) {
					planDetected = true
					slog.Info("plan title detected in agent output", "session", sess.ID)
				}
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
	<-done // wait for scanner to finish

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

	sess.SetStatus(StatusPending)
	writeJSON(ctx, conn, WSMessage{Type: "status", Status: "pending"})
}

func (h *Handler) buildPrompt(sess *Session) string {
	var b strings.Builder

	b.WriteString("# Planning Session\n\n")
	b.WriteString(fmt.Sprintf("You are a planning assistant for the **%s** repository.\n", sess.Repo))
	b.WriteString("You have full access to the codebase in your working directory.\n")
	b.WriteString("Explore the code proactively. Cite specific files and functions.\n\n")

	if sess.LinkedIssue != nil {
		b.WriteString(fmt.Sprintf("This session is linked to GitHub issue #%d.\n\n", *sess.LinkedIssue))
	}

	if len(sess.Messages) > 0 {
		b.WriteString("## Conversation So Far\n\n")
		for _, msg := range sess.Messages {
			role := "User"
			if msg.Role == "agent" {
				role = "You (previously)"
			}
			b.WriteString(fmt.Sprintf("**%s:** %s\n\n", role, msg.Content))
		}
	}

	b.WriteString("## CRITICAL RULES — READ BEFORE DOING ANYTHING\n\n")
	b.WriteString("**YOU ARE A PLANNING-ONLY ASSISTANT.** Your ONLY purpose is to produce a written plan.\n\n")
	b.WriteString("### What you MUST do:\n")
	b.WriteString("1. Read files and search code to understand the codebase\n")
	b.WriteString("2. Ask the user clarifying questions if needed\n")
	b.WriteString("3. Output a structured plan in the EXACT format shown below\n")
	b.WriteString("4. STOP after presenting the plan and wait for user feedback\n\n")
	b.WriteString("### What you MUST NEVER do:\n")
	b.WriteString("- NEVER create, edit, write, or delete any files\n")
	b.WriteString("- NEVER run shell commands, scripts, or builds\n")
	b.WriteString("- NEVER implement any part of the plan\n")
	b.WriteString("- NEVER write code (not even examples or snippets to files)\n")
	b.WriteString("- NEVER attempt to use tools you don't have access to\n\n")
	b.WriteString("If you catch yourself starting to implement, STOP IMMEDIATELY and return to planning.\n\n")
	b.WriteString("### Required plan format\n\n")
	b.WriteString("Your plan MUST use this EXACT markdown format (not inside a code fence):\n\n")
	b.WriteString("## Plan: <title>\n\n")
	b.WriteString("### Phase 1: <name>\n")
	b.WriteString("- [ ] Task description (complexity: S/M/L)\n")
	b.WriteString("  Dependencies: none\n\n")
	b.WriteString("### Phase 2: <name>\n")
	b.WriteString("- [ ] Task description (complexity: S/M/L)\n")
	b.WriteString("  Dependencies: Phase 1\n\n")
	b.WriteString("**Output the plan as regular markdown text, NOT inside a code block.**\n\n")

	if h.cfg.SkillText != "" {
		b.WriteString("## Planning Skill\n\n")
		b.WriteString(h.cfg.SkillText)
		b.WriteString("\n")
	}

	return b.String()
}

// transformClaudeEvent normalizes a Claude stream-json event.
// Returns nil to indicate the event should be filtered.
func transformClaudeEvent(raw map[string]any) (out map[string]any, text string) {
	eventType, _ := raw["type"].(string)

	switch eventType {
	case "system", "rate_limit_event":
		return nil, ""
	}

	out = map[string]any{"source": "claude", "type": eventType}

	switch eventType {
	case "assistant":
		if msg, ok := raw["message"].(map[string]any); ok {
			if content, ok := msg["content"].([]any); ok {
				out["content_blocks"] = content
				for _, block := range content {
					if b, ok := block.(map[string]any); ok {
						if b["type"] == "text" {
							if t, ok := b["text"].(string); ok {
								text += t + "\n"
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
	default:
		raw["source"] = "claude"
		out = raw
	}

	return out, strings.TrimRight(text, "\n")
}

// transformCopilotEvent normalizes a Copilot JSONL event into the same shape as Claude events.
// Returns nil to indicate the event should be filtered.
func transformCopilotEvent(raw map[string]any) (out map[string]any, text string) {
	eventType, _ := raw["type"].(string)

	switch eventType {
	case "user.message", "assistant.turn_start", "assistant.turn_end",
		"assistant.message_delta", "assistant.reasoning", "assistant.reasoning_delta",
		"tool.execution_start", "session.info":
		return nil, ""
	}

	data, _ := raw["data"].(map[string]any)

	switch eventType {
	case "assistant.message":
		out = map[string]any{"source": "copilot", "type": "assistant"}
		var blocks []any

		if content, _ := data["content"].(string); content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": content})
			text = content
		}

		if toolReqs, ok := data["toolRequests"].([]any); ok {
			for _, tr := range toolReqs {
				if req, ok := tr.(map[string]any); ok {
					blocks = append(blocks, map[string]any{
						"type":  "tool_use",
						"id":    req["toolCallId"],
						"name":  req["name"],
						"input": req["arguments"],
					})
				}
			}
		}

		out["content_blocks"] = blocks

	case "tool.execution_complete":
		out = map[string]any{
			"source":      "copilot",
			"type":        "tool_result",
			"tool_use_id": data["toolCallId"],
		}
		if result, ok := data["result"].(map[string]any); ok {
			out["content"] = result["content"]
		}

	case "result":
		subtype := "success"
		if code, ok := raw["exitCode"].(float64); ok && code != 0 {
			subtype = "error"
		}
		out = map[string]any{
			"source":  "copilot",
			"type":    "result",
			"subtype": subtype,
		}
		if usage, ok := raw["usage"].(map[string]any); ok {
			out["usage"] = usage
			if dur, ok := usage["sessionDurationMs"].(float64); ok {
				out["duration_ms"] = dur
			}
		}

	default:
		return nil, ""
	}

	return out, text
}

func writeJSON(ctx context.Context, conn *websocket.Conn, msg WSMessage) {
	data, _ := json.Marshal(msg)
	conn.Write(ctx, websocket.MessageText, data) //nolint:errcheck // best-effort websocket write
}

func readJSON(ctx context.Context, conn *websocket.Conn, msg *WSMessage) error {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("read websocket message: %w", err)
	}
	if err := json.Unmarshal(data, msg); err != nil {
		return fmt.Errorf("unmarshal websocket message: %w", err)
	}
	return nil
}
