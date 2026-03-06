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
	"nhooyr.io/websocket"
)

type fakeRunner struct {
	output string
}

func (f *fakeRunner) Name() string { return "fake" }

func (f *fakeRunner) Start(ctx context.Context, workspace string, prompt string, opts agent.AgentOpts) (*agent.Session, error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if opts.Stdout != nil {
			opts.Stdout.Write([]byte(f.output))
		}
	}()
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

	// Read initial status message
	var statusMsg planning.WSMessage
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(data, &statusMsg)
	if statusMsg.Type != "status" {
		t.Errorf("expected initial status message, got type=%s", statusMsg.Type)
	}

	// Send a user message
	msgData, _ := json.Marshal(planning.WSMessage{Type: "message", Content: "What does this repo do?"})
	conn.Write(ctx, websocket.MessageText, msgData)

	// Read responses: expect status=active, then agent line(s), then status=pending
	gotAgent := false
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg planning.WSMessage
		json.Unmarshal(data, &msg)
		if msg.Type == "agent" {
			gotAgent = true
		}
		if msg.Type == "status" && msg.Status == "pending" {
			break
		}
	}
	if !gotAgent {
		t.Error("expected at least one agent message")
	}

	// Verify session has messages recorded
	got := mgr.Get(sess.ID)
	if len(got.Messages) < 2 {
		t.Errorf("expected at least 2 messages (user + agent), got %d", len(got.Messages))
	}
}

func TestHandler_SessionNotFound(t *testing.T) {
	mgr := planning.NewManager()
	h := planning.NewHandler(mgr, &fakeRunner{}, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	h.HandleWebSocket(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandler_ReplayMessages(t *testing.T) {
	mgr := planning.NewManager()
	sess, _ := mgr.Create("owner/repo", nil)
	sess.AddMessage("user", "first message")
	sess.AddMessage("agent", "first response")

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

	// Should receive 2 replayed messages + 1 status
	messages := make([]planning.WSMessage, 0, 3)
	for i := 0; i < 3; i++ {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg planning.WSMessage
		json.Unmarshal(data, &msg)
		messages = append(messages, msg)
	}

	if messages[0].Type != "user" || messages[0].Content != "first message" {
		t.Errorf("expected replayed user message, got: %+v", messages[0])
	}
	if messages[1].Type != "agent" || messages[1].Content != "first response" {
		t.Errorf("expected replayed agent message, got: %+v", messages[1])
	}
	if messages[2].Type != "status" {
		t.Errorf("expected status message, got: %+v", messages[2])
	}
}
