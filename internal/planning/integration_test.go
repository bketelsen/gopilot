package planning_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/go-chi/chi/v5"
	"nhooyr.io/websocket"
)

func TestIntegration_FullPlanningFlow(t *testing.T) {
	mgr := planning.NewManager()
	runner := &fakeRunner{output: "## Plan: Test Feature\n### Phase 1: Init\n- [x] Do thing (complexity: S)\n  Dependencies: none\n"}

	routes := planning.NewRoutes(mgr, runner, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	r := chi.NewRouter()
	routes.Register(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 1. Create session via API
	resp, err := http.Post(srv.URL+"/api/planning/sessions",
		"application/json",
		strings.NewReader(`{"repo":"owner/repo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string `json:"id"` }
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	if created.ID == "" {
		t.Fatal("expected non-empty session ID")
	}

	// 2. Connect WebSocket
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/planning/sessions/" + created.ID + "/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	// Read initial status
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var initStatus planning.WSMessage
	json.Unmarshal(data, &initStatus)
	if initStatus.Type != "status" {
		t.Errorf("expected initial status, got type=%s", initStatus.Type)
	}

	// 3. Send user message
	msgData, _ := json.Marshal(planning.WSMessage{Type: "message", Content: "Plan a test feature"})
	err = conn.Write(ctx, websocket.MessageText, msgData)
	if err != nil {
		t.Fatal(err)
	}

	// 4. Read responses until turn complete (status=pending)
	gotAgent := false
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("reading response: %v", err)
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

	// 5. Verify session state
	sess := mgr.Get(created.ID)
	if sess == nil {
		t.Fatal("session not found")
	}
	if len(sess.Messages) < 2 {
		t.Errorf("expected at least 2 messages (user + agent), got %d", len(sess.Messages))
	}

	// 6. Verify list endpoint shows the session
	listResp, err := http.Get(srv.URL + "/api/planning/sessions")
	if err != nil {
		t.Fatal(err)
	}
	var listResult struct {
		Sessions []planning.Session `json:"sessions"`
	}
	json.NewDecoder(listResp.Body).Decode(&listResult)
	listResp.Body.Close()
	if len(listResult.Sessions) != 1 {
		t.Errorf("expected 1 session in list, got %d", len(listResult.Sessions))
	}
}
