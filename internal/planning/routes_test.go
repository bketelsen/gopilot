package planning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/go-chi/chi/v5"
)

type fakeGitHub struct {
	issuesCreated int
}

func (f *fakeGitHub) CreateIssue(_ context.Context, repo, title, body string, labels []string) (*domain.Issue, error) {
	f.issuesCreated++
	return &domain.Issue{ID: f.issuesCreated, Repo: repo, Title: title}, nil
}

func (f *fakeGitHub) AddSubIssue(_ context.Context, repo string, parentID, childID int) error {
	return nil
}

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

func TestRoutes_CreateOutput_Issues(t *testing.T) {
	mgr := planning.NewManager()
	sess, _ := mgr.Create("owner/repo", nil)
	sess.AddMessage("agent", "## Plan: Test Feature\n### Phase 1: Setup\n- [x] Create config file (complexity: S)\n  Dependencies: none\n- [x] Add validation (complexity: M)\n  Dependencies: Create config file\n- [ ] Optional thing (complexity: L)\n  Dependencies: none\n")

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
	// Only checked tasks (2 of 3) should create issues
	if gh.issuesCreated != 2 {
		t.Errorf("expected 2 issues created, got %d", gh.issuesCreated)
	}
}

func TestRoutes_CreateOutput_Doc(t *testing.T) {
	mgr := planning.NewManager()
	sess, _ := mgr.Create("owner/repo", nil)
	sess.AddMessage("agent", "## Plan: Doc Test\n### Phase 1: Init\n- [x] Do thing (complexity: S)\n  Dependencies: none\n")

	routes := planning.NewRoutes(mgr, nil, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	r := chi.NewRouter()
	routes.Register(r)

	body := fmt.Sprintf(`{"session_id":"%s","action":"doc"}`, sess.ID)
	req := httptest.NewRequest("POST", "/api/planning/output", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	doc, ok := resp["document"].(string)
	if !ok || doc == "" {
		t.Error("expected non-empty document in response")
	}
}

func TestRoutes_CreateOutput_NoPlan(t *testing.T) {
	mgr := planning.NewManager()
	sess, _ := mgr.Create("owner/repo", nil)
	sess.AddMessage("user", "hello")

	routes := planning.NewRoutes(mgr, nil, planning.HandlerConfig{
		WorkspaceRoot: t.TempDir(),
	})

	r := chi.NewRouter()
	routes.Register(r)

	body := fmt.Sprintf(`{"session_id":"%s","action":"doc"}`, sess.ID)
	req := httptest.NewRequest("POST", "/api/planning/output", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
