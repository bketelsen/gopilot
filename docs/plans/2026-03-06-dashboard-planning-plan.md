# Dashboard-Based Interactive Planning — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the issue-comment planning flow with a WebSocket-driven chat UI in the dashboard where agents have full repo access.

**Architecture:** New `internal/planning/` package manages chat sessions. Each session clones a workspace, launches agent subprocesses per turn (reusing existing `agent.Runner`), and bridges stdout to the browser via WebSocket. The agent runner gets an `io.Writer` option for stdout capture. The existing `orchestrator/planning.go` code is replaced with a simple redirect comment.

**Tech Stack:** Go, nhooyr.io/websocket, chi/v5, templ, HTMX, Tailwind CSS v4

---

### Task 1: Add stdout capture to agent runner

**Files:**
- Modify: `internal/agent/runner.go:15-20`
- Modify: `internal/agent/claude.go:22-66`
- Modify: `internal/agent/copilot.go:24-66`
- Test: `internal/agent/runner_test.go` (new)

**Step 1: Write the failing test**

Create `internal/agent/runner_test.go`:

```go
package agent_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bketelsen/gopilot/internal/agent"
)

func TestClaudeRunner_StdoutCapture(t *testing.T) {
	// Create a fake "claude" script that writes to stdout
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "claude")
	os.WriteFile(script, []byte("#!/bin/bash\necho 'hello from agent'"), 0755)

	var buf bytes.Buffer
	runner := &agent.ClaudeRunner{Command: script, Token: "test"}
	sess, err := runner.Start(context.Background(), tmpDir, "test prompt", agent.AgentOpts{
		Stdout: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	<-sess.Done
	if !bytes.Contains(buf.Bytes(), []byte("hello from agent")) {
		t.Errorf("expected stdout capture, got: %s", buf.String())
	}
}

func TestClaudeRunner_StdoutDefaultsToStderr(t *testing.T) {
	// When Stdout is nil, should not panic (defaults to os.Stderr)
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "claude")
	os.WriteFile(script, []byte("#!/bin/bash\nexit 0"), 0755)

	runner := &agent.ClaudeRunner{Command: script, Token: "test"}
	sess, err := runner.Start(context.Background(), tmpDir, "test prompt", agent.AgentOpts{})
	if err != nil {
		t.Fatal(err)
	}
	<-sess.Done
	if sess.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", sess.ExitCode)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestClaudeRunner ./internal/agent/...`
Expected: FAIL — `AgentOpts` has no `Stdout` field

**Step 3: Add Stdout field to AgentOpts**

In `internal/agent/runner.go`, add `Stdout` to `AgentOpts`:

```go
import "io"

type AgentOpts struct {
	Model            string
	MaxContinuations int
	Env              []string
	Stdout           io.Writer // if nil, defaults to os.Stderr
}
```

**Step 4: Update ClaudeRunner.Start to use opts.Stdout**

In `internal/agent/claude.go`, replace line 32 (`cmd.Stdout = os.Stderr`) with:

```go
if opts.Stdout != nil {
	cmd.Stdout = opts.Stdout
} else {
	cmd.Stdout = os.Stderr
}
```

**Step 5: Update CopilotRunner.Start the same way**

In `internal/agent/copilot.go`, same change at the equivalent line.

**Step 6: Run tests to verify they pass**

Run: `go test -race -run TestClaudeRunner ./internal/agent/...`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/agent/
git commit -m "feat: add stdout capture option to agent runner"
```

---

### Task 2: Add nhooyr.io/websocket dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

Run: `go get nhooyr.io/websocket@latest`

**Step 2: Verify it builds**

Run: `go build ./...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add nhooyr.io/websocket"
```

---

### Task 3: Planning session types and manager

**Files:**
- Create: `internal/planning/session.go`
- Create: `internal/planning/manager.go`
- Test: `internal/planning/manager_test.go`

**Step 1: Write the failing test**

Create `internal/planning/manager_test.go`:

```go
package planning_test

import (
	"testing"

	"github.com/bketelsen/gopilot/internal/planning"
)

func TestManager_CreateAndGet(t *testing.T) {
	m := planning.NewManager()
	sess, err := m.Create("owner/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.Repo != "owner/repo" {
		t.Errorf("expected repo owner/repo, got %s", sess.Repo)
	}
	if sess.Status != planning.StatusPending {
		t.Errorf("expected status pending, got %s", sess.Status)
	}

	got := m.Get(sess.ID)
	if got == nil {
		t.Fatal("expected to find session")
	}
	if got.ID != sess.ID {
		t.Errorf("expected ID %s, got %s", sess.ID, got.ID)
	}
}

func TestManager_CreateWithLinkedIssue(t *testing.T) {
	m := planning.NewManager()
	issueID := 42
	sess, err := m.Create("owner/repo", &issueID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.LinkedIssue == nil || *sess.LinkedIssue != 42 {
		t.Errorf("expected linked issue 42, got %v", sess.LinkedIssue)
	}
}

func TestManager_List(t *testing.T) {
	m := planning.NewManager()
	m.Create("owner/repo1", nil)
	m.Create("owner/repo2", nil)

	sessions := m.List()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestManager_Close(t *testing.T) {
	m := planning.NewManager()
	sess, _ := m.Create("owner/repo", nil)
	m.Close(sess.ID)

	got := m.Get(sess.ID)
	if got.Status != planning.StatusDone {
		t.Errorf("expected status done, got %s", got.Status)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/planning/...`
Expected: FAIL — package doesn't exist

**Step 3: Create session types**

Create `internal/planning/session.go`:

```go
package planning

import (
	"sync"
	"time"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type ChatMessage struct {
	Role      string    `json:"role"` // "user" or "agent"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Session struct {
	ID          string        `json:"id"`
	Repo        string        `json:"repo"`
	LinkedIssue *int          `json:"linked_issue,omitempty"`
	Status      Status        `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	Messages    []ChatMessage `json:"messages"`

	mu sync.Mutex // protects Messages and Status
}

// AddMessage appends a message to the session history.
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ChatMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

// SetStatus updates the session status.
func (s *Session) SetStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
}
```

**Step 4: Create manager**

Create `internal/planning/manager.go`:

```go
package planning

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) Create(repo string, linkedIssue *int) (*Session, error) {
	id := generateID()
	sess := &Session{
		ID:          id,
		Repo:        repo,
		LinkedIssue: linkedIssue,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
	}
	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()
	return sess, nil
}

func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

func (m *Manager) Close(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.Status = StatusDone
	}
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("plan-%x", b)
}
```

**Step 5: Run tests to verify they pass**

Run: `go test -race ./internal/planning/...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/planning/
git commit -m "feat: add planning session types and manager"
```

---

### Task 4: Planning chat handler (WebSocket + agent dispatch)

**Files:**
- Create: `internal/planning/handler.go`
- Create: `internal/planning/handler_test.go`

This is the core piece: WebSocket handler that receives user messages, launches agent subprocesses, and streams stdout back.

**Step 1: Write the failing test**

Create `internal/planning/handler_test.go`:

```go
package planning_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/planning"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// fakeRunner implements agent.Runner for tests.
type fakeRunner struct {
	output string
}

func (f *fakeRunner) Name() string { return "fake" }

func (f *fakeRunner) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	// Write output to opts.Stdout if set
	if opts.Stdout != nil {
		opts.Stdout.Write([]byte(f.output))
	}
	done := make(chan struct{})
	close(done)
	return &agent.Session{
		ID:   "fake-1",
		PID:  1,
		Done: done,
	}, nil
}

func (f *fakeRunner) Stop(sess *agent.Session) error { return nil }

func TestHandler_WebSocketChat(t *testing.T) {
	mgr := planning.NewManager()
	sess, _ := mgr.Create("owner/repo", nil)

	h := planning.NewHandler(mgr, &fakeRunner{output: "I'll explore the codebase.\n"}, planning.HandlerConfig{
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

	// Send a user message
	wsjson.Write(ctx, conn, planning.WSMessage{Type: "message", Content: "What does this repo do?"})

	// Read agent response
	var msg planning.WSMessage
	err = wsjson.Read(ctx, conn, &msg)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != "agent" {
		t.Errorf("expected type 'agent', got %q", msg.Type)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestHandler ./internal/planning/...`
Expected: FAIL — `Handler` type doesn't exist

**Step 3: Create the handler**

Create `internal/planning/handler.go`:

```go
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

type WSMessage struct {
	Type    string `json:"type"`              // message, agent, status, error, done, cancel
	Content string `json:"content,omitempty"`
	Status  string `json:"status,omitempty"`
}

type HandlerConfig struct {
	WorkspaceRoot string
	SkillText     string // pre-rendered planning skill
}

type Handler struct {
	mgr    *Manager
	runner agent.Runner
	cfg    HandlerConfig
}

func NewHandler(mgr *Manager, runner agent.Runner, cfg HandlerConfig) *Handler {
	return &Handler{mgr: mgr, runner: runner, cfg: cfg}
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess := h.mgr.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow any origin for local dev
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

	// Send current status
	writeJSON(ctx, conn, WSMessage{Type: "status", Status: string(sess.Status)})

	// Read messages from client
	for {
		var msg WSMessage
		err := readJSON(ctx, conn, &msg)
		if err != nil {
			return // client disconnected
		}

		switch msg.Type {
		case "message":
			sess.AddMessage("user", msg.Content)
			sess.SetStatus(StatusActive)
			writeJSON(ctx, conn, WSMessage{Type: "status", Status: "active"})
			h.runAgentTurn(ctx, conn, sess)
		case "cancel":
			// TODO: cancel running agent
			return
		}
	}
}

func (h *Handler) runAgentTurn(ctx context.Context, conn *websocket.Conn, sess *Session) {
	prompt := h.buildPrompt(sess)

	// Set up workspace directory for the agent
	wsDir := filepath.Join(h.cfg.WorkspaceRoot, sess.ID)
	os.MkdirAll(wsDir, 0755)

	// Pipe agent stdout to WebSocket
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

	// Stream stdout lines to WebSocket
	var fullResponse strings.Builder
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			fullResponse.WriteString(line)
			fullResponse.WriteString("\n")
			writeJSON(ctx, conn, WSMessage{Type: "agent", Content: line})
		}
	}()

	// Wait for agent to finish
	<-agentSess.Done
	pw.Close()

	// Record agent response
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
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -run TestHandler ./internal/planning/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/planning/
git commit -m "feat: add planning WebSocket handler with agent I/O bridge"
```

---

### Task 5: HTTP API endpoints for planning sessions

**Files:**
- Create: `internal/planning/routes.go`
- Modify: `internal/web/server.go:41-90`
- Test: `internal/planning/routes_test.go`

**Step 1: Write the failing test**

Create `internal/planning/routes_test.go`:

```go
package planning_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/go-chi/chi/v5"
)

func TestRoutes_CreateSession(t *testing.T) {
	mgr := planning.NewManager()
	routes := planning.NewRoutes(mgr, nil, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	r := chi.NewRouter()
	routes.Register(r)

	body := `{"repo":"owner/repo"}`
	req := httptest.NewRequest("POST", "/api/planning/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID   string `json:"id"`
		Repo string `json:"repo"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Repo != "owner/repo" {
		t.Errorf("expected repo owner/repo, got %s", resp.Repo)
	}
}

func TestRoutes_ListSessions(t *testing.T) {
	mgr := planning.NewManager()
	mgr.Create("owner/repo1", nil)
	mgr.Create("owner/repo2", nil)

	routes := planning.NewRoutes(mgr, nil, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	r := chi.NewRouter()
	routes.Register(r)

	req := httptest.NewRequest("GET", "/api/planning/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Sessions []*planning.Session `json:"sessions"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(resp.Sessions))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestRoutes ./internal/planning/...`
Expected: FAIL — `Routes` type doesn't exist

**Step 3: Create routes**

Create `internal/planning/routes.go`:

```go
package planning

import (
	"encoding/json"
	"net/http"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/go-chi/chi/v5"
)

type Routes struct {
	mgr     *Manager
	handler *Handler
}

func NewRoutes(mgr *Manager, runner agent.Runner, cfg HandlerConfig) *Routes {
	return &Routes{
		mgr:     mgr,
		handler: NewHandler(mgr, runner, cfg),
	}
}

func (rt *Routes) Register(r chi.Router) {
	r.Route("/api/planning", func(r chi.Router) {
		r.Post("/sessions", rt.createSession)
		r.Get("/sessions", rt.listSessions)
		r.Get("/sessions/{id}/ws", rt.websocket)
	})
}

type createRequest struct {
	Repo        string `json:"repo"`
	LinkedIssue *int   `json:"linked_issue,omitempty"`
}

func (rt *Routes) createSession(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Repo == "" {
		http.Error(w, "repo is required", http.StatusBadRequest)
		return
	}

	sess, err := rt.mgr.Create(req.Repo, req.LinkedIssue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sess)
}

func (rt *Routes) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions := rt.mgr.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sessions": sessions,
	})
}

func (rt *Routes) websocket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rt.handler.HandleWebSocket(w, r, id)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -run TestRoutes ./internal/planning/...`
Expected: PASS

**Step 5: Wire routes into web server**

In `internal/web/server.go`, add planning routes to `Server` and `buildRouter()`:

Add to Server struct:
```go
planningRoutes *planning.Routes
```

In `NewServer`, accept an optional `planning.Routes`:
```go
func NewServer(state StateProvider, cfg *config.Config, metrics MetricsProvider, retries RetryProvider, opts ...ServerOption) *Server {
```

Or simpler: add a `SetPlanningRoutes` method:
```go
func (s *Server) SetPlanningRoutes(routes *planning.Routes) {
	routes.Register(s.router)
}
```

In `buildRouter()`, add the planning page routes:
```go
r.Get("/planning", s.handlePlanningListPage)
r.Get("/planning/{id}", s.handlePlanningChatPage)
```

Add sidebar link in `internal/web/templates/layouts/base.templ`:
```
<li>
	<a href="/planning" class="block px-3 py-2 rounded-md hover:bg-gray-100 dark:hover:bg-gray-700">
		Planning
	</a>
</li>
```

**Step 6: Run full test suite**

Run: `go test -race ./...`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/planning/ internal/web/
git commit -m "feat: add planning API routes and wire into web server"
```

---

### Task 6: Planning list page (templ)

**Files:**
- Create: `internal/web/templates/pages/planning_list.templ`
- Modify: `internal/web/server.go` (add handler)

**Step 1: Create the planning list page template**

Create `internal/web/templates/pages/planning_list.templ`:

```go
package pages

import (
	"fmt"
	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

templ PlanningList(sessions []*planning.Session, repos []string) {
	@layouts.Base("Planning") {
		<div class="max-w-4xl mx-auto">
			<div class="flex justify-between items-center mb-6">
				<h1 class="text-2xl font-bold">Planning Sessions</h1>
				<button
					onclick="document.getElementById('new-session-dialog').showModal()"
					class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
				>
					New Planning Session
				</button>
			</div>

			<!-- Sessions list -->
			<div class="space-y-3">
				if len(sessions) == 0 {
					<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-8 text-center text-gray-500">
						No planning sessions yet. Start one to begin.
					</div>
				}
				for _, sess := range sessions {
					<a href={ templ.SafeURL(fmt.Sprintf("/planning/%s", sess.ID)) }
						class="block bg-white dark:bg-gray-800 rounded-lg shadow p-4 hover:ring-2 hover:ring-blue-500 transition">
						<div class="flex justify-between items-center">
							<div>
								<div class="font-medium">{ sess.Repo }</div>
								if sess.LinkedIssue != nil {
									<div class="text-sm text-gray-500">Issue #{ fmt.Sprintf("%d", *sess.LinkedIssue) }</div>
								}
							</div>
							<div class="flex items-center gap-3">
								<span class={ "px-2 py-1 text-xs font-medium rounded-full",
									templ.KV("bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200", string(sess.Status) == "active"),
									templ.KV("bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200", string(sess.Status) == "pending"),
									templ.KV("bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200", string(sess.Status) == "done"),
								}>
									{ string(sess.Status) }
								</span>
								<span class="text-sm text-gray-500">
									{ fmt.Sprintf("%d messages", len(sess.Messages)) }
								</span>
							</div>
						</div>
					</a>
				}
			</div>

			<!-- New Session Dialog -->
			<dialog id="new-session-dialog" class="rounded-lg shadow-xl p-0 backdrop:bg-black/50 bg-white dark:bg-gray-800">
				<form method="dialog" class="p-6 min-w-96">
					<h2 class="text-lg font-bold mb-4">New Planning Session</h2>
					<div class="mb-4">
						<label class="block text-sm font-medium mb-1">Repository</label>
						<select id="new-session-repo" class="w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2">
							for _, repo := range repos {
								<option value={ repo }>{ repo }</option>
							}
						</select>
					</div>
					<div class="mb-4">
						<label class="block text-sm font-medium mb-1">GitHub Issue (optional)</label>
						<input id="new-session-issue" type="number" placeholder="e.g. 42"
							class="w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2"/>
					</div>
					<div class="flex justify-end gap-2">
						<button type="submit" class="px-4 py-2 rounded-md border border-gray-300 dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-700">Cancel</button>
						<button type="submit" id="create-session-btn"
							class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
							onclick="createSession(event)">
							Start
						</button>
					</div>
				</form>
			</dialog>

			<script>
				async function createSession(e) {
					e.preventDefault();
					const repo = document.getElementById('new-session-repo').value;
					const issueInput = document.getElementById('new-session-issue').value;
					const body = { repo };
					if (issueInput) body.linked_issue = parseInt(issueInput);

					const resp = await fetch('/api/planning/sessions', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify(body),
					});
					if (resp.ok) {
						const data = await resp.json();
						window.location.href = '/planning/' + data.id;
					}
				}
			</script>
		</div>
	}
}
```

**Step 2: Add handler in server.go**

Add `handlePlanningListPage` to `internal/web/server.go`:

```go
func (s *Server) handlePlanningListPage(w http.ResponseWriter, r *http.Request) {
	var sessions []*planning.Session
	if s.planningMgr != nil {
		sessions = s.planningMgr.List()
	}
	repos := s.cfg.GitHub.Repos
	component := pages.PlanningList(sessions, repos)
	component.Render(r.Context(), w)
}
```

**Step 3: Generate templ and verify build**

Run: `task generate && go build ./...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add internal/web/
git commit -m "feat: add planning sessions list page"
```

---

### Task 7: Planning chat page (templ + WebSocket JS)

**Files:**
- Create: `internal/web/templates/pages/planning_chat.templ`
- Modify: `internal/web/server.go` (add handler)

**Step 1: Create the chat page template**

Create `internal/web/templates/pages/planning_chat.templ`:

```go
package pages

import (
	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
)

templ PlanningChat(session *planning.Session) {
	@layouts.Base("Planning - " + session.Repo) {
		<div class="max-w-4xl mx-auto flex flex-col" style="height: calc(100vh - 3rem);">
			<!-- Header -->
			<div class="flex items-center justify-between mb-4">
				<div>
					<a href="/planning" class="text-sm text-gray-500 hover:text-gray-700">&larr; All sessions</a>
					<h1 class="text-xl font-bold">{ session.Repo }</h1>
				</div>
				<span id="status-badge" class="px-2 py-1 text-xs font-medium rounded-full bg-yellow-100 text-yellow-800">
					{ string(session.Status) }
				</span>
			</div>

			<!-- Chat messages -->
			<div id="chat-messages" class="flex-1 overflow-y-auto space-y-4 mb-4 p-4 bg-white dark:bg-gray-800 rounded-lg shadow">
				<!-- Messages populated by JS -->
			</div>

			<!-- Input -->
			<div class="flex gap-2">
				<input id="chat-input" type="text" placeholder="Type a message..."
					class="flex-1 rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-4 py-2"
					onkeydown="if(event.key==='Enter')sendMessage()"/>
				<button onclick="sendMessage()"
					class="px-6 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50"
					id="send-btn">
					Send
				</button>
			</div>
		</div>

		<script>
			const sessionID = window.location.pathname.split('/').pop();
			const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
			const ws = new WebSocket(`${wsProto}//${location.host}/api/planning/sessions/${sessionID}/ws`);
			const messages = document.getElementById('chat-messages');
			const input = document.getElementById('chat-input');
			const sendBtn = document.getElementById('send-btn');
			const statusBadge = document.getElementById('status-badge');
			let agentBuf = '';

			ws.onmessage = (e) => {
				const msg = JSON.parse(e.data);
				switch (msg.type) {
					case 'user':
						appendMessage('user', msg.content);
						break;
					case 'agent':
						// Streaming: accumulate lines into current agent bubble
						if (!document.getElementById('agent-streaming')) {
							const div = document.createElement('div');
							div.id = 'agent-streaming';
							div.className = 'bg-gray-100 dark:bg-gray-700 rounded-lg p-3 max-w-[80%]';
							div.innerHTML = '<div class="text-xs text-purple-600 dark:text-purple-400 mb-1 font-medium">Agent</div><pre class="whitespace-pre-wrap text-sm" id="agent-streaming-content"></pre>';
							messages.appendChild(div);
						}
						agentBuf += msg.content + '\n';
						document.getElementById('agent-streaming-content').textContent = agentBuf;
						messages.scrollTop = messages.scrollHeight;
						break;
					case 'status':
						statusBadge.textContent = msg.status;
						if (msg.status === 'active') {
							sendBtn.disabled = true;
							input.disabled = true;
							statusBadge.className = 'px-2 py-1 text-xs font-medium rounded-full bg-green-100 text-green-800';
						} else {
							sendBtn.disabled = false;
							input.disabled = false;
							// Finalize streaming bubble
							const streaming = document.getElementById('agent-streaming');
							if (streaming) {
								streaming.removeAttribute('id');
								document.getElementById('agent-streaming-content')?.removeAttribute('id');
							}
							agentBuf = '';
							statusBadge.className = 'px-2 py-1 text-xs font-medium rounded-full bg-yellow-100 text-yellow-800';
							input.focus();
						}
						break;
					case 'error':
						appendMessage('error', msg.content);
						break;
				}
			};

			ws.onclose = () => {
				statusBadge.textContent = 'disconnected';
				statusBadge.className = 'px-2 py-1 text-xs font-medium rounded-full bg-red-100 text-red-800';
			};

			function appendMessage(role, content) {
				const div = document.createElement('div');
				const isUser = role === 'user';
				div.className = `rounded-lg p-3 max-w-[80%] ${isUser ? 'ml-auto bg-blue-100 dark:bg-blue-900' : role === 'error' ? 'bg-red-100 dark:bg-red-900' : 'bg-gray-100 dark:bg-gray-700'}`;
				const label = isUser ? 'You' : role === 'error' ? 'Error' : 'Agent';
				const labelColor = isUser ? 'text-blue-600 dark:text-blue-400' : role === 'error' ? 'text-red-600' : 'text-purple-600 dark:text-purple-400';
				div.innerHTML = `<div class="text-xs ${labelColor} mb-1 font-medium">${label}</div><pre class="whitespace-pre-wrap text-sm">${escapeHTML(content)}</pre>`;
				messages.appendChild(div);
				messages.scrollTop = messages.scrollHeight;
			}

			function sendMessage() {
				const text = input.value.trim();
				if (!text || sendBtn.disabled) return;
				appendMessage('user', text);
				ws.send(JSON.stringify({ type: 'message', content: text }));
				input.value = '';
			}

			function escapeHTML(s) {
				const div = document.createElement('div');
				div.textContent = s;
				return div.innerHTML;
			}

			input.focus();
		</script>
	}
}
```

**Step 2: Add handler in server.go**

```go
func (s *Server) handlePlanningChatPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.planningMgr == nil {
		http.Error(w, "planning not configured", http.StatusNotFound)
		return
	}
	sess := s.planningMgr.Get(id)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	component := pages.PlanningChat(sess)
	component.Render(r.Context(), w)
}
```

**Step 3: Generate templ and verify build**

Run: `task generate && go build ./...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add internal/web/
git commit -m "feat: add planning chat page with WebSocket client"
```

---

### Task 8: Plan output generation

**Files:**
- Create: `internal/planning/output.go`
- Create: `internal/planning/output_test.go`

**Step 1: Write the failing test**

Create `internal/planning/output_test.go`:

```go
package planning_test

import (
	"testing"

	"github.com/bketelsen/gopilot/internal/planning"
)

func TestParsePlan(t *testing.T) {
	markdown := `## Plan: Auth System Redesign
### Phase 1: Foundation
- [ ] Create auth middleware (complexity: M)
  Dependencies: none
- [ ] Add JWT token validation (complexity: S)
  Dependencies: none
### Phase 2: Integration
- [x] Wire up protected routes (complexity: L)
  Dependencies: Create auth middleware
`
	plan, err := planning.ParsePlan(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Title != "Auth System Redesign" {
		t.Errorf("expected title 'Auth System Redesign', got %q", plan.Title)
	}
	if len(plan.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(plan.Phases))
	}
	if len(plan.Phases[0].Tasks) != 2 {
		t.Errorf("expected 2 tasks in phase 1, got %d", len(plan.Phases[0].Tasks))
	}
	// Checked items should be marked
	if !plan.Phases[1].Tasks[0].Checked {
		t.Error("expected checked task to be true")
	}
}

func TestPlanToMarkdownDoc(t *testing.T) {
	plan := &planning.Plan{
		Title: "Test Plan",
		Phases: []planning.Phase{
			{
				Name: "Phase 1",
				Tasks: []planning.Task{
					{Description: "Do thing", Complexity: "S", Checked: true},
				},
			},
		},
	}
	doc := planning.PlanToMarkdown(plan)
	if doc == "" {
		t.Fatal("expected non-empty markdown")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestParse ./internal/planning/...`
Expected: FAIL

**Step 3: Implement plan parser and output**

Create `internal/planning/output.go`:

```go
package planning

import (
	"fmt"
	"regexp"
	"strings"
)

type Plan struct {
	Title  string  `json:"title"`
	Phases []Phase `json:"phases"`
}

type Phase struct {
	Name  string `json:"name"`
	Tasks []Task `json:"tasks"`
}

type Task struct {
	Description  string `json:"description"`
	Complexity   string `json:"complexity"`
	Dependencies string `json:"dependencies"`
	Checked      bool   `json:"checked"`
}

var (
	planTitleRe = regexp.MustCompile(`^##\s+Plan:\s+(.+)`)
	phaseRe     = regexp.MustCompile(`^###\s+(?:Phase\s+\d+:\s+)?(.+)`)
	taskRe      = regexp.MustCompile(`^-\s+\[([ xX])\]\s+(.+?)(?:\s+\(complexity:\s+(\w+)\))?$`)
	depRe       = regexp.MustCompile(`^\s+Dependencies:\s+(.+)`)
)

func ParsePlan(markdown string) (*Plan, error) {
	lines := strings.Split(markdown, "\n")
	plan := &Plan{}
	var currentPhase *Phase

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if m := planTitleRe.FindStringSubmatch(line); m != nil {
			plan.Title = strings.TrimSpace(m[1])
			continue
		}

		if m := phaseRe.FindStringSubmatch(line); m != nil {
			plan.Phases = append(plan.Phases, Phase{Name: strings.TrimSpace(m[1])})
			currentPhase = &plan.Phases[len(plan.Phases)-1]
			continue
		}

		if m := taskRe.FindStringSubmatch(line); m != nil && currentPhase != nil {
			task := Task{
				Description: strings.TrimSpace(m[2]),
				Complexity:  m[3],
				Checked:     m[1] != " ",
			}
			// Check next line for dependencies
			if i+1 < len(lines) {
				if dm := depRe.FindStringSubmatch(lines[i+1]); dm != nil {
					task.Dependencies = strings.TrimSpace(dm[1])
					i++
				}
			}
			currentPhase.Tasks = append(currentPhase.Tasks, task)
		}
	}

	if plan.Title == "" {
		return nil, fmt.Errorf("no plan title found")
	}
	return plan, nil
}

func PlanToMarkdown(plan *Plan) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", plan.Title))
	for i, phase := range plan.Phases {
		b.WriteString(fmt.Sprintf("## Phase %d: %s\n\n", i+1, phase.Name))
		for _, task := range phase.Tasks {
			check := " "
			if task.Checked {
				check = "x"
			}
			b.WriteString(fmt.Sprintf("- [%s] %s", check, task.Description))
			if task.Complexity != "" {
				b.WriteString(fmt.Sprintf(" (complexity: %s)", task.Complexity))
			}
			b.WriteString("\n")
			if task.Dependencies != "" {
				b.WriteString(fmt.Sprintf("  Dependencies: %s\n", task.Dependencies))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -run "TestParse|TestPlan" ./internal/planning/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/planning/
git commit -m "feat: add plan parser and markdown output generation"
```

---

### Task 9: Output action endpoints (create issues / save doc)

**Files:**
- Modify: `internal/planning/routes.go`
- Test: `internal/planning/routes_test.go`

**Step 1: Write the failing test**

Add to `internal/planning/routes_test.go`:

```go
func TestRoutes_CreateIssuesFromPlan(t *testing.T) {
	mgr := planning.NewManager()
	sess, _ := mgr.Create("owner/repo", nil)
	sess.AddMessage("agent", `## Plan: Test Feature
### Phase 1: Setup
- [x] Create config file (complexity: S)
  Dependencies: none
- [x] Add validation (complexity: M)
  Dependencies: Create config file
`)

	gh := &fakeGitHub{}
	routes := planning.NewRoutes(mgr, nil, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
		GitHubClient:  gh,
	})

	r := chi.NewRouter()
	routes.Register(r)

	body := fmt.Sprintf(`{"session_id":"%s","action":"issues"}`, sess.ID)
	req := httptest.NewRequest("POST", "/api/planning/output", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gh.issuesCreated != 2 {
		t.Errorf("expected 2 issues created, got %d", gh.issuesCreated)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestRoutes_Create ./internal/planning/...`
Expected: FAIL

**Step 3: Add output endpoint**

Add to `internal/planning/routes.go`:

```go
// In Register():
r.Post("/output", rt.createOutput)

// Handler:
type outputRequest struct {
	SessionID string `json:"session_id"`
	Action    string `json:"action"` // "issues", "doc", "both"
}

type IssueCreator interface {
	CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*domain.Issue, error)
	AddSubIssue(ctx context.Context, repo string, parentID, childID int) error
}

func (rt *Routes) createOutput(w http.ResponseWriter, r *http.Request) {
	var req outputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	sess := rt.mgr.Get(req.SessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Find the last agent message containing a plan
	var planText string
	for i := len(sess.Messages) - 1; i >= 0; i-- {
		if sess.Messages[i].Role == "agent" && strings.Contains(sess.Messages[i].Content, "## Plan:") {
			planText = sess.Messages[i].Content
			break
		}
	}
	if planText == "" {
		http.Error(w, "no plan found in conversation", http.StatusBadRequest)
		return
	}

	plan, err := ParsePlan(planText)
	if err != nil {
		http.Error(w, "failed to parse plan: "+err.Error(), http.StatusBadRequest)
		return
	}

	result := map[string]any{}

	if req.Action == "issues" || req.Action == "both" {
		issues, err := rt.createIssuesFromPlan(r.Context(), sess, plan)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result["issues"] = issues
	}

	if req.Action == "doc" || req.Action == "both" {
		doc := PlanToMarkdown(plan)
		result["document"] = doc
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -run TestRoutes ./internal/planning/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/planning/
git commit -m "feat: add plan output endpoints for issue creation and doc export"
```

---

### Task 10: Replace old planning flow with redirect

**Files:**
- Modify: `internal/orchestrator/planning.go`
- Modify: `internal/orchestrator/planning_test.go`

**Step 1: Write the test for redirect behavior**

Update `internal/orchestrator/planning_test.go` — replace existing dispatch tests with redirect test:

```go
func TestProcessPlanningIssues_PostsRedirect(t *testing.T) {
	gh := &mockGitHub{}
	o := newTestOrchestrator(gh)

	issues := []domain.Issue{
		{ID: 1, Repo: "owner/repo", Title: "Plan something"},
	}

	o.processPlanningIssues(context.Background(), issues)

	if len(gh.comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(gh.comments))
	}
	if !strings.Contains(gh.comments[0].Body, "/planning/new") {
		t.Errorf("expected redirect comment, got: %s", gh.comments[0].Body)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestProcessPlanningIssues_PostsRedirect ./internal/orchestrator/...`
Expected: FAIL — current code dispatches an agent instead

**Step 3: Replace processPlanningIssues with redirect**

Replace the body of `processPlanningIssues` in `internal/orchestrator/planning.go`:

```go
func (o *Orchestrator) processPlanningIssues(ctx context.Context, issues []domain.Issue) {
	for _, issue := range issues {
		// Skip if already redirected (tracked by planning state)
		if o.state.IsPlanning(issue.ID) {
			continue
		}

		// Post redirect comment
		addr := o.cfg.Dashboard.Addr
		if addr == "" {
			addr = ":3000"
		}
		body := fmt.Sprintf(
			"Planning sessions are now interactive in the dashboard.\n\n"+
				"Start one at: http://localhost%s/planning/new?repo=%s&issue=%d\n\n"+
				PlanningCommentMarker,
			addr, issue.Repo, issue.ID,
		)
		if err := o.github.AddComment(ctx, issue.Repo, issue.ID, body); err != nil {
			slog.Error("failed to post planning redirect", "issue", issue.Identifier(), "error", err)
			continue
		}

		// Track that we've redirected this issue
		o.state.AddPlanning(issue.ID, &PlanningEntry{
			IssueID: issue.ID,
			Repo:    issue.Repo,
			Phase:   PlanningPhaseComplete,
		})
	}
}
```

**Step 4: Remove dispatchPlanningAgent and related functions**

Remove `dispatchPlanningAgent()` from `planning.go`. Keep `partitionPlanningIssues()`, `PlanningCommentMarker`, and `isBotComment()` (still used by the redirect tracking). Remove `hasNewHumanComment()`.

**Step 5: Run tests to verify they pass**

Run: `go test -race ./internal/orchestrator/...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/orchestrator/
git commit -m "refactor: replace issue-comment planning with dashboard redirect"
```

---

### Task 11: Wire everything together in main.go

**Files:**
- Modify: `cmd/gopilot/main.go`
- Modify: `internal/web/server.go`

**Step 1: Add planning manager and routes to Server**

In `internal/web/server.go`, add `planningMgr *planning.Manager` to the Server struct and add a setter:

```go
func (s *Server) SetPlanningManager(mgr *planning.Manager, runner agent.Runner, cfg planning.HandlerConfig) {
	s.planningMgr = mgr
	routes := planning.NewRoutes(mgr, runner, cfg)
	routes.Register(s.router)
}
```

**Step 2: Wire in main.go**

In `cmd/gopilot/main.go`, after creating the web server:

```go
planningMgr := planning.NewManager()
webSrv.SetPlanningManager(planningMgr, defaultRunner, planning.HandlerConfig{
	WorkspaceRoot: cfg.Workspace.Root,
	SkillText:     skills.InjectSkills(loadedSkills, []string{"planning"}, nil),
})
```

**Step 3: Verify full build**

Run: `task build`
Expected: SUCCESS

**Step 4: Run full test suite**

Run: `task test`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/gopilot/ internal/web/
git commit -m "feat: wire planning manager into main application"
```

---

### Task 12: Update dashboard planning table to link to new UI

**Files:**
- Modify: `internal/web/templates/pages/dashboard.templ:72-112`

**Step 1: Update the planning sessions section**

Replace the Planning Sessions table with a simpler card that links to `/planning`:

```go
<!-- Planning Sessions -->
<div class="bg-white dark:bg-gray-800 rounded-lg shadow mb-6">
	<div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
		<h2 class="text-lg font-semibold">Planning Sessions</h2>
		<a href="/planning" class="text-sm text-blue-500 hover:underline">View all &rarr;</a>
	</div>
	<div class="p-4">
		<a href="/planning"
			class="inline-flex items-center px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm">
			New Planning Session
		</a>
	</div>
</div>
```

**Step 2: Generate templ and verify build**

Run: `task generate && go build ./...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add internal/web/templates/pages/dashboard.templ
git commit -m "feat: update dashboard planning section to link to new chat UI"
```

---

### Task 13: Add sidebar navigation link

**Files:**
- Modify: `internal/web/templates/layouts/base.templ:17-33`

**Step 1: Add Planning link to sidebar**

After the Sprint `<li>`, add:

```html
<li>
	<a href="/planning" class="block px-3 py-2 rounded-md hover:bg-gray-100 dark:hover:bg-gray-700">
		Planning
	</a>
</li>
```

**Step 2: Generate templ and verify build**

Run: `task generate && go build ./...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add internal/web/templates/layouts/base.templ
git commit -m "feat: add Planning link to sidebar navigation"
```

---

### Task 14: End-to-end smoke test

**Files:**
- Create: `internal/planning/integration_test.go`

**Step 1: Write integration test**

```go
package planning_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/go-chi/chi/v5"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestIntegration_FullPlanningFlow(t *testing.T) {
	mgr := planning.NewManager()
	runner := &fakeRunner{output: "## Plan: Test\n### Phase 1: Init\n- [x] Do thing (complexity: S)\n  Dependencies: none\n"}

	routes := planning.NewRoutes(mgr, runner, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	r := chi.NewRouter()
	routes.Register(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 1. Create session
	resp, err := http.Post(srv.URL+"/api/planning/sessions",
		"application/json",
		strings.NewReader(`{"repo":"owner/repo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// 2. Connect WebSocket and send message
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/planning/sessions/" + created.ID + "/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	// Read initial status
	var status planning.WSMessage
	wsjson.Read(ctx, conn, &status)

	// Send message
	wsjson.Write(ctx, conn, planning.WSMessage{Type: "message", Content: "Plan a test feature"})

	// Read responses until we get status=pending (turn complete)
	for {
		var msg planning.WSMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type == "status" && msg.Status == "pending" {
			break
		}
	}

	// 3. Verify session has messages
	sess := mgr.Get(created.ID)
	if len(sess.Messages) < 2 {
		t.Errorf("expected at least 2 messages (user + agent), got %d", len(sess.Messages))
	}
}
```

**Step 2: Run integration test**

Run: `go test -race -run TestIntegration ./internal/planning/...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/planning/
git commit -m "test: add end-to-end integration test for planning flow"
```

---

Plan complete and saved to `docs/plans/2026-03-06-dashboard-planning-plan.md`. Two execution options:

**1. Subagent-Driven (this session)** — I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** — Open new session with executing-plans, batch execution with checkpoints

Which approach?