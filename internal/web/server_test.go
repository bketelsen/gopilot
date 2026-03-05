package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

// mockState implements StateProvider for testing.
type mockState struct {
	entries []*domain.RunEntry
}

func (m *mockState) AllRunning() []*domain.RunEntry {
	return m.entries
}

func (m *mockState) RunningCount() int {
	return len(m.entries)
}

func (m *mockState) GetRunning(issueID int) *domain.RunEntry { return nil }
func (m *mockState) GetHistory(issueID int) []domain.CompletedRun { return nil }

func TestHealthEndpoint(t *testing.T) {
	state := &mockState{}
	srv := NewServer(state, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestStateEndpoint(t *testing.T) {
	state := &mockState{
		entries: []*domain.RunEntry{
			{
				Issue:     domain.Issue{ID: 42, Repo: "o/r", Title: "Fix bug"},
				SessionID: "sess-1",
				StartedAt: time.Now(),
				Attempt:   1,
			},
		},
	}

	srv := NewServer(state, nil, nil, nil)
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
