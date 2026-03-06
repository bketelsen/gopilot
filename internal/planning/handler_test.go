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

	// Verify all events have source:"claude" and spot-check shapes
	for i, e := range agentEvents {
		var parsed map[string]any
		json.Unmarshal([]byte(e), &parsed)
		if parsed["source"] != "claude" {
			t.Errorf("event[%d]: expected source=claude, got %v", i, parsed["source"])
		}
	}
	// Verify tool_result carries tool_use_id
	if len(agentEvents) >= 3 {
		var toolResult map[string]any
		json.Unmarshal([]byte(agentEvents[2]), &toolResult)
		if toolResult["tool_use_id"] != "toolu_1" {
			t.Errorf("tool_result missing tool_use_id, got: %v", toolResult["tool_use_id"])
		}
	}
	// Verify result carries cherry-picked fields
	if len(agentEvents) >= 4 {
		var result map[string]any
		json.Unmarshal([]byte(agentEvents[3]), &result)
		if result["total_cost_usd"] == nil {
			t.Error("result missing total_cost_usd")
		}
		if result["duration_ms"] == nil {
			t.Error("result missing duration_ms")
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
