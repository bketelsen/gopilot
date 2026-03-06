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
		Stdout:   pw,
		ReadOnly: true,
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
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			fullResponse.WriteString(line)
			fullResponse.WriteString("\n")
			if strings.TrimSpace(line) == "" {
				continue
			}
			writeJSON(ctx, conn, WSMessage{Type: "agent", Content: line})
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
	b.WriteString("You are a PLANNING assistant ONLY. You must NEVER execute, implement, or modify any code.\n")
	b.WriteString("Your job is to:\n")
	b.WriteString("1. Explore the codebase to understand the current state\n")
	b.WriteString("2. Ask clarifying questions if needed\n")
	b.WriteString("3. Propose a structured plan for the user to review\n\n")
	b.WriteString("DO NOT create, edit, or delete any files. DO NOT run any commands that modify state.\n")
	b.WriteString("You may read files and search code to inform your plan.\n\n")
	b.WriteString("When you have enough context, propose a structured plan using this exact format:\n\n")
	b.WriteString("```\n## Plan: <title>\n### Phase 1: <name>\n- [ ] Task description (complexity: S/M/L)\n  Dependencies: none\n```\n\n")
	b.WriteString("After presenting the plan, STOP and wait for user feedback. Do not proceed to implement it.\n\n")

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
