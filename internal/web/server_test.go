package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/orchestrator"
)

func TestHealthEndpoint(t *testing.T) {
	state := orchestrator.NewState()
	srv := NewServer(state, nil)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestStateEndpoint(t *testing.T) {
	state := orchestrator.NewState()
	state.AddRunning(42, &domain.RunEntry{
		Issue:     domain.Issue{ID: 42, Repo: "o/r", Title: "Fix bug"},
		SessionID: "sess-1",
		StartedAt: time.Now(),
		Attempt:   1,
	})

	srv := NewServer(state, nil)
	req := httptest.NewRequest("GET", "/api/v1/state", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["running_count"].(float64) != 1 {
		t.Errorf("running_count = %v, want 1", resp["running_count"])
	}
}
