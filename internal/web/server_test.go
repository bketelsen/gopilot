package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/skills"
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

func TestSettingsPageShowsSkills(t *testing.T) {
	state := &mockState{}
	cfg := &config.Config{}
	loadedSkills := []*skills.Skill{
		{Name: "git-commit", Type: "required", Description: "Handles git commits"},
		{Name: "testing", Type: "optional", Description: "Runs tests"},
	}

	srv := NewServer(state, cfg, nil, nil)
	srv.SetSkills(loadedSkills)

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "No skills loaded") {
		t.Error("settings page should not show 'No skills loaded' when skills are provided")
	}
	if !strings.Contains(body, "git-commit") {
		t.Error("settings page should contain skill name 'git-commit'")
	}
	if !strings.Contains(body, "testing") {
		t.Error("settings page should contain skill name 'testing'")
	}
}

func TestSettingsPageNoSkills(t *testing.T) {
	state := &mockState{}
	cfg := &config.Config{}

	srv := NewServer(state, cfg, nil, nil)

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "No skills loaded") {
		t.Error("settings page should show 'No skills loaded' when no skills are provided")
	}
}

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

// mockSprint implements SprintProvider for testing.
type mockSprint struct {
	issues map[string][]domain.Issue         // label -> issues
	prs    map[int][]domain.PullRequest      // issueNumber -> PRs
}

func (m *mockSprint) FetchLabeledIssues(_ context.Context, label string) ([]domain.Issue, error) {
	return m.issues[label], nil
}

func (m *mockSprint) FetchLinkedPullRequests(_ context.Context, _ string, issueNumber int) ([]domain.PullRequest, error) {
	return m.prs[issueNumber], nil
}

func TestSprintPageCategorizesIssues(t *testing.T) {
	// Issue 1: open, no PR, no agent running → Todo
	// Issue 2: open, agent running → In Progress
	// Issue 3: open, has open PR → In Review
	// Issue 4: closed, has merged PR → Done
	// Issue 5: closed, no PR → Done (from GitHub state)

	state := &mockState{
		entries: []*domain.RunEntry{
			{
				Issue:     domain.Issue{ID: 2, Repo: "o/r", Title: "Running task", Status: "Todo"},
				SessionID: "s2",
				StartedAt: time.Now(),
			},
		},
	}

	sprint := &mockSprint{
		issues: map[string][]domain.Issue{
			"gopilot": {
				{ID: 1, Repo: "o/r", Title: "Waiting", Status: "Todo"},
				{ID: 2, Repo: "o/r", Title: "Running task", Status: "Todo"},
				{ID: 3, Repo: "o/r", Title: "Has PR", Status: "Todo"},
				{ID: 4, Repo: "o/r", Title: "Merged", Status: "Done"},
				{ID: 5, Repo: "o/r", Title: "Closed no PR", Status: "Done"},
			},
		},
		prs: map[int][]domain.PullRequest{
			1: {},
			2: {},
			3: {{Number: 10, State: "open", Merged: false}},
			4: {{Number: 11, State: "closed", Merged: true}},
			5: {},
		},
	}

	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			EligibleLabels: []string{"gopilot"},
		},
	}

	srv := NewServer(state, cfg, nil, nil)
	srv.SetSprintProvider(sprint)

	req := httptest.NewRequest("GET", "/api/v1/sprint", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	byStatus, ok := resp["by_status"].(map[string]any)
	if !ok {
		t.Fatal("by_status not found in response")
	}

	todo := byStatus["Todo"].([]any)
	inProgress := byStatus["In Progress"].([]any)
	inReview := byStatus["In Review"].([]any)
	done := byStatus["Done"].([]any)

	if len(todo) != 1 {
		t.Errorf("Todo count = %d, want 1", len(todo))
	}
	if len(inProgress) != 1 {
		t.Errorf("In Progress count = %d, want 1", len(inProgress))
	}
	if len(inReview) != 1 {
		t.Errorf("In Review count = %d, want 1", len(inReview))
	}
	if len(done) != 2 {
		t.Errorf("Done count = %d, want 2", len(done))
	}

	total := resp["total"].(float64)
	doneCount := resp["done"].(float64)
	if total != 5 {
		t.Errorf("total = %v, want 5", total)
	}
	if doneCount != 2 {
		t.Errorf("done = %v, want 2", doneCount)
	}
}

func TestSprintPageFallbackWithoutProvider(t *testing.T) {
	state := &mockState{
		entries: []*domain.RunEntry{
			{
				Issue:     domain.Issue{ID: 1, Repo: "o/r", Title: "Active"},
				SessionID: "s1",
				StartedAt: time.Now(),
			},
		},
	}

	// No sprint provider, no config → fallback to legacy behavior
	srv := NewServer(state, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/v1/sprint", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	byStatus := resp["by_status"].(map[string]any)
	inProgress := byStatus["In Progress"].([]any)
	if len(inProgress) != 1 {
		t.Errorf("In Progress count = %d, want 1 (fallback)", len(inProgress))
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
