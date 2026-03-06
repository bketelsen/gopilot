package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestFetchIssueComments(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
		comments := []map[string]any{
			{"id": 101, "body": "First comment", "created_at": "2026-01-01T00:00:00Z", "user": map[string]any{"login": "alice"}},
			{"id": 102, "body": "Second comment", "created_at": "2026-01-02T00:00:00Z", "user": map[string]any{"login": "gopilot[bot]"}},
		}
		json.NewEncoder(w).Encode(comments)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	comments, err := client.FetchIssueComments(context.Background(), "owner/repo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 {
		t.Fatalf("got %d comments, want 2", len(comments))
	}
	if comments[0].Author != "alice" {
		t.Errorf("comment[0].Author = %q, want %q", comments[0].Author, "alice")
	}
	if comments[1].ID != 102 {
		t.Errorf("comment[1].ID = %d, want 102", comments[1].ID)
	}
}

func TestCreateIssue(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)

		if payload["title"] != "New issue" {
			t.Errorf("title = %q, want %q", payload["title"], "New issue")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"number": 42, "node_id": "MDU6SXNzdWU0Mg==",
			"title": payload["title"], "body": payload["body"],
			"state": "open", "html_url": "https://github.com/owner/repo/issues/42",
			"labels":     []map[string]any{{"name": "gopilot"}},
			"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}, EligibleLabels: []string{"gopilot"}}
	client := NewRESTClient(cfg, server.URL+"/")

	issue, err := client.CreateIssue(context.Background(), "owner/repo", "New issue", "Body text", []string{"gopilot"})
	if err != nil {
		t.Fatal(err)
	}
	if issue.ID != 42 {
		t.Errorf("issue.ID = %d, want 42", issue.ID)
	}
	if issue.Title != "New issue" {
		t.Errorf("issue.Title = %q, want %q", issue.Title, "New issue")
	}
}

func TestRateLimitParsing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	cfg := config.GitHubConfig{
		Token:          "test",
		Repos:          []string{"o/r"},
		EligibleLabels: []string{"gopilot"},
	}
	client := NewRESTClient(cfg, ts.URL+"/")
	client.FetchCandidateIssues(context.Background())

	rl := client.GetRateLimit()
	if rl.Remaining != 4999 {
		t.Errorf("remaining = %d, want 4999", rl.Remaining)
	}
	if rl.Limit != 5000 {
		t.Errorf("limit = %d, want 5000", rl.Limit)
	}
	wantReset := time.Unix(1700000000, 0)
	if !rl.Reset.Equal(wantReset) {
		t.Errorf("reset = %v, want %v", rl.Reset, wantReset)
	}
}

func TestRemoveLabelURLEncoding(t *testing.T) {
	var receivedURI string
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /repos/owner/repo/issues/1/labels/", func(w http.ResponseWriter, r *http.Request) {
		receivedURI = r.RequestURI
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	// Test with a label that contains characters requiring URL encoding (space, hash)
	err := client.RemoveLabel(context.Background(), "owner/repo", 1, "needs review #2")
	if err != nil {
		t.Fatal(err)
	}
	want := "/repos/owner/repo/issues/1/labels/needs%20review%20%232"
	if receivedURI != want {
		t.Errorf("received URI = %q, want %q", receivedURI, want)
	}
}

func TestGetRepoLabel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/labels/gopilot", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"name":        "gopilot",
			"color":       "0052CC",
			"description": "Eligible for Gopilot agent dispatch",
		})
	})
	mux.HandleFunc("GET /repos/owner/repo/labels/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	// Existing label
	label, err := client.GetRepoLabel(context.Background(), "owner/repo", "gopilot")
	if err != nil {
		t.Fatal(err)
	}
	if label == nil {
		t.Fatal("expected label, got nil")
	}
	if label.Name != "gopilot" {
		t.Errorf("name = %q, want %q", label.Name, "gopilot")
	}
	if label.Color != "0052CC" {
		t.Errorf("color = %q, want %q", label.Color, "0052CC")
	}

	// Missing label
	label, err = client.GetRepoLabel(context.Background(), "owner/repo", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if label != nil {
		t.Errorf("expected nil for missing label, got %+v", label)
	}
}

func TestCreateRepoLabel(t *testing.T) {
	var received map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/labels", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	err := client.CreateRepoLabel(context.Background(), "owner/repo", "gopilot", "0052CC", "Eligible for Gopilot agent dispatch")
	if err != nil {
		t.Fatal(err)
	}
	if received["name"] != "gopilot" {
		t.Errorf("name = %v, want gopilot", received["name"])
	}
	if received["color"] != "0052CC" {
		t.Errorf("color = %v, want 0052CC", received["color"])
	}
	if received["description"] != "Eligible for Gopilot agent dispatch" {
		t.Errorf("description = %v, want correct description", received["description"])
	}
}

func TestUpdateRepoLabel(t *testing.T) {
	var received map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /repos/owner/repo/labels/gopilot", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	err := client.UpdateRepoLabel(context.Background(), "owner/repo", "gopilot", "0052CC", "Updated description")
	if err != nil {
		t.Fatal(err)
	}
	if received["color"] != "0052CC" {
		t.Errorf("color = %v, want 0052CC", received["color"])
	}
	if received["description"] != "Updated description" {
		t.Errorf("description = %v, want Updated description", received["description"])
	}
}

func TestToDomainStatusFromState(t *testing.T) {
	open := ghIssue{Number: 1, State: "open", Title: "Open issue"}
	closed := ghIssue{Number: 2, State: "closed", Title: "Closed issue"}

	openIssue := open.toDomain("o/r")
	if openIssue.Status != "Todo" {
		t.Errorf("open issue Status = %q, want %q", openIssue.Status, "Todo")
	}

	closedIssue := closed.toDomain("o/r")
	if closedIssue.Status != "Done" {
		t.Errorf("closed issue Status = %q, want %q", closedIssue.Status, "Done")
	}
}

func TestFetchLabeledIssues(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		label := r.URL.Query().Get("labels")
		if label != "gopilot" {
			t.Errorf("labels param = %q, want %q", label, "gopilot")
		}

		var issues []map[string]any
		if state == "open" {
			issues = []map[string]any{
				{
					"number": 1, "title": "Open todo", "body": "", "state": "open",
					"html_url": "https://github.com/owner/repo/issues/1", "node_id": "N1",
					"labels":     []map[string]any{{"name": "gopilot"}},
					"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z",
				},
				{
					"number": 10, "title": "A PR not an issue", "body": "", "state": "open",
					"html_url": "https://github.com/owner/repo/pull/10", "node_id": "N10",
					"labels":       []map[string]any{{"name": "gopilot"}},
					"pull_request": map[string]any{},
					"created_at":   "2026-01-01T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z",
				},
			}
		} else if state == "closed" {
			issues = []map[string]any{
				{
					"number": 2, "title": "Closed done", "body": "", "state": "closed",
					"html_url": "https://github.com/owner/repo/issues/2", "node_id": "N2",
					"labels":     []map[string]any{{"name": "gopilot"}},
					"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-03T00:00:00Z",
				},
			}
		}
		json.NewEncoder(w).Encode(issues)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{
		Token: "test-token", Repos: []string{"owner/repo"},
		EligibleLabels: []string{"gopilot"},
	}
	client := NewRESTClient(cfg, server.URL+"/")

	issues, err := client.FetchLabeledIssues(context.Background(), "gopilot")
	if err != nil {
		t.Fatal(err)
	}

	// Should return 2 issues: 1 open + 1 closed; PR #10 should be excluded
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2", len(issues))
	}

	if issues[0].ID != 1 || issues[0].Status != "Todo" {
		t.Errorf("issue[0] = {ID:%d, Status:%q}, want {ID:1, Status:Todo}", issues[0].ID, issues[0].Status)
	}
	if issues[1].ID != 2 || issues[1].Status != "Done" {
		t.Errorf("issue[1] = {ID:%d, Status:%q}, want {ID:2, Status:Done}", issues[1].ID, issues[1].Status)
	}
}

func TestFetchLinkedPullRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues/1/timeline", func(w http.ResponseWriter, r *http.Request) {
		events := []map[string]any{
			{
				"event": "cross-referenced",
				"source": map[string]any{
					"issue": map[string]any{
						"number": 5, "title": "Fix for #1", "state": "open",
						"html_url":     "https://github.com/owner/repo/pull/5",
						"pull_request": map[string]any{"merged_at": nil},
					},
				},
			},
			{
				"event": "cross-referenced",
				"source": map[string]any{
					"issue": map[string]any{
						"number": 6, "title": "Another fix", "state": "closed",
						"html_url":     "https://github.com/owner/repo/pull/6",
						"pull_request": map[string]any{"merged_at": "2026-01-05T00:00:00Z"},
					},
				},
			},
			{
				"event": "labeled",
			},
			{
				"event": "cross-referenced",
				"source": map[string]any{
					"issue": map[string]any{
						"number": 7, "title": "Not a PR, just a reference", "state": "open",
						"html_url": "https://github.com/owner/repo/issues/7",
					},
				},
			},
			// Duplicate cross-reference for PR 5
			{
				"event": "cross-referenced",
				"source": map[string]any{
					"issue": map[string]any{
						"number": 5, "title": "Fix for #1", "state": "open",
						"html_url":     "https://github.com/owner/repo/pull/5",
						"pull_request": map[string]any{"merged_at": nil},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(events)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	prs, err := client.FetchLinkedPullRequests(context.Background(), "owner/repo", 1)
	if err != nil {
		t.Fatal(err)
	}

	// Should return 2 PRs: #5 (open, not merged) and #6 (closed, merged)
	// #7 is not a PR, and duplicate #5 should be deduplicated
	if len(prs) != 2 {
		t.Fatalf("got %d PRs, want 2", len(prs))
	}

	if prs[0].Number != 5 || prs[0].State != "open" || prs[0].Merged {
		t.Errorf("pr[0] = %+v, want Number:5 State:open Merged:false", prs[0])
	}
	if prs[1].Number != 6 || prs[1].State != "closed" || !prs[1].Merged {
		t.Errorf("pr[1] = %+v, want Number:6 State:closed Merged:true", prs[1])
	}
}

func TestErrorWrapping(t *testing.T) {
	// Use an unreachable server to trigger HTTP errors
	cfg := config.GitHubConfig{
		Token:          "test-token",
		Repos:          []string{"owner/repo"},
		EligibleLabels: []string{"gopilot"},
	}
	client := NewRESTClient(cfg, "http://127.0.0.1:1/")

	ctx := context.Background()

	tests := []struct {
		name    string
		fn      func() error
		wantCtx string
	}{
		{
			name:    "FetchCandidateIssues wraps fetch error",
			fn:      func() error { _, err := client.FetchCandidateIssues(ctx); return err },
			wantCtx: "fetching owner/repo",
		},
		{
			name:    "FetchIssueState wraps error",
			fn:      func() error { _, err := client.FetchIssueState(ctx, "owner/repo", 1); return err },
			wantCtx: "fetch issue state owner/repo#1",
		},
		{
			name:    "AddComment wraps error",
			fn:      func() error { return client.AddComment(ctx, "owner/repo", 1, "hi") },
			wantCtx: "add comment to owner/repo#1",
		},
		{
			name:    "AddLabel wraps error",
			fn:      func() error { return client.AddLabel(ctx, "owner/repo", 1, "bug") },
			wantCtx: "add label \"bug\" to owner/repo#1",
		},
		{
			name:    "FetchIssueComments wraps error",
			fn:      func() error { _, err := client.FetchIssueComments(ctx, "owner/repo", 1); return err },
			wantCtx: "fetch comments for owner/repo#1",
		},
		{
			name:    "RemoveLabel wraps error",
			fn:      func() error { return client.RemoveLabel(ctx, "owner/repo", 1, "bug") },
			wantCtx: "remove label \"bug\" from owner/repo#1",
		},
		{
			name:    "CreateIssue wraps error",
			fn:      func() error { _, err := client.CreateIssue(ctx, "owner/repo", "t", "b", nil); return err },
			wantCtx: "create issue in owner/repo",
		},
		{
			name:    "AddSubIssue wraps error",
			fn:      func() error { return client.AddSubIssue(ctx, "owner/repo", 1, 2) },
			wantCtx: "add sub-issue 2 to owner/repo#1",
		},
		{
			name:    "GetRepoLabel wraps error",
			fn:      func() error { _, err := client.GetRepoLabel(ctx, "owner/repo", "x"); return err },
			wantCtx: "get label \"x\" from owner/repo",
		},
		{
			name:    "CreateRepoLabel wraps error",
			fn:      func() error { return client.CreateRepoLabel(ctx, "owner/repo", "x", "fff", "d") },
			wantCtx: "create label \"x\" in owner/repo",
		},
		{
			name:    "UpdateRepoLabel wraps error",
			fn:      func() error { return client.UpdateRepoLabel(ctx, "owner/repo", "x", "fff", "d") },
			wantCtx: "update label \"x\" in owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantCtx) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantCtx)
			}
		})
	}
}

func TestAddSubIssue(t *testing.T) {
	var called bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/issues/1/sub_issues", func(w http.ResponseWriter, r *http.Request) {
		called = true
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["sub_issue_id"] != float64(2) {
			t.Errorf("sub_issue_id = %v, want 2", payload["sub_issue_id"])
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	err := client.AddSubIssue(context.Background(), "owner/repo", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("API was not called")
	}
}
