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
	Content string `json:"content,omitempty"`
	Status  string `json:"status,omitempty"`
}

// HandlerConfig configures the planning WebSocket handler.
type HandlerConfig struct {
	WorkspaceRoot string
	SkillText     string
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
	defer conn.CloseNow()

	ctx := r.Context()

	// Replay existing messages for reconnect
	sess.mu.Lock()
	for _, msg := range sess.Messages {
		writeJSON(ctx, conn, WSMessage{Type: msg.Role, Content: msg.Content})
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
	os.MkdirAll(wsDir, 0755)

	pr, pw := io.Pipe()

	opts := agent.AgentOpts{
		Stdout: pw,
	}

	agentSess, err := h.runner.Start(ctx, wsDir, prompt, opts)
	if err != nil {
		writeJSON(ctx, conn, WSMessage{Type: "error", Content: fmt.Sprintf("agent start failed: %v", err)})
		sess.SetStatus(StatusFailed)
		pw.Close()
		return
	}

	var fullResponse strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			fullResponse.WriteString(line)
			fullResponse.WriteString("\n")
			writeJSON(ctx, conn, WSMessage{Type: "agent", Content: line})
		}
	}()

	<-agentSess.Done
	pw.Close()
	<-done // wait for scanner to finish

	response := strings.TrimSpace(fullResponse.String())
	if response != "" {
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

	b.WriteString("## Instructions\n\n")
	b.WriteString("Respond naturally. When you have enough context, propose a structured plan:\n\n")
	b.WriteString("```\n## Plan: <title>\n### Phase 1: <name>\n- [ ] Task (complexity: S/M/L)\n  Dependencies: none\n```\n\n")

	if h.cfg.SkillText != "" {
		b.WriteString("## Planning Skill\n\n")
		b.WriteString(h.cfg.SkillText)
		b.WriteString("\n")
	}

	return b.String()
}

func writeJSON(ctx context.Context, conn *websocket.Conn, msg WSMessage) {
	data, _ := json.Marshal(msg)
	conn.Write(ctx, websocket.MessageText, data)
}

func readJSON(ctx context.Context, conn *websocket.Conn, msg *WSMessage) error {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, msg)
}
