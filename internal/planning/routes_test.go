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
	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestRoutes_CreateSession_MissingRepo(t *testing.T) {
	mgr := planning.NewManager()
	routes := planning.NewRoutes(mgr, nil, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	r := chi.NewRouter()
	routes.Register(r)

	req := httptest.NewRequest("POST", "/api/planning/sessions", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
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
		Sessions []planning.Session `json:"sessions"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(resp.Sessions))
	}
}
