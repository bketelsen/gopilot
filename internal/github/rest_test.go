package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/gopilot/internal/config"
)

func TestNormalizeLabels(t *testing.T) {
	labels := normalizeLabels([]string{"Gopilot", "BUG", "Feature"})
	want := []string{"gopilot", "bug", "feature"}
	for i, got := range labels {
		if got != want[i] {
			t.Errorf("label[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestFetchCandidateIssues(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		issues := []map[string]any{
			{
				"number":     1,
				"title":      "First issue",
				"body":       "Do something",
				"state":      "open",
				"html_url":   "https://github.com/owner/repo/issues/1",
				"node_id":    "MDU6SXNzdWUx",
				"labels": []map[string]any{
					{"name": "gopilot"},
					{"name": "bug"},
				},
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z",
			},
			{
				"number":     2,
				"title":      "Blocked issue",
				"body":       "",
				"state":      "open",
				"html_url":   "https://github.com/owner/repo/issues/2",
				"node_id":    "MDU6SXNzdWUy",
				"labels": []map[string]any{
					{"name": "gopilot"},
					{"name": "blocked"},
				},
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z",
			},
		}
		json.NewEncoder(w).Encode(issues)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{
		Token:          "test-token",
		Repos:          []string{"owner/repo"},
		EligibleLabels: []string{"gopilot"},
		ExcludedLabels: []string{"blocked"},
	}
	client := NewRESTClient(cfg, server.URL+"/")

	issues, err := client.FetchCandidateIssues(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should return only issue 1 (issue 2 has excluded label "blocked")
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}
	if issues[0].ID != 1 {
		t.Errorf("issue ID = %d, want 1", issues[0].ID)
	}
	if issues[0].Labels[0] != "gopilot" {
		t.Errorf("label = %q, want %q", issues[0].Labels[0], "gopilot")
	}
}
