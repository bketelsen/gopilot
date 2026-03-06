package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/gopilot/internal/config"
)

func TestAPIErrorSentinelMapping(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    error
	}{
		{"401 maps to ErrUnauthorized", 401, ErrUnauthorized},
		{"403 maps to ErrRateLimited", 403, ErrRateLimited},
		{"404 maps to ErrNotFound", 404, ErrNotFound},
		{"409 maps to ErrConflict", 409, ErrConflict},
		{"500 has no sentinel", 500, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := newAPIError(tt.statusCode, "test body")
			if apiErr.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.statusCode)
			}
			if tt.wantErr != nil {
				if !errors.Is(apiErr, tt.wantErr) {
					t.Errorf("errors.Is(apiErr, %v) = false, want true", tt.wantErr)
				}
			} else {
				if apiErr.Err != nil {
					t.Errorf("Err = %v, want nil for status %d", apiErr.Err, tt.statusCode)
				}
			}
		})
	}
}

func TestAPIErrorUnwrapAndAs(t *testing.T) {
	apiErr := newAPIError(404, "not found")

	// errors.As should work
	var target *APIError
	if !errors.As(apiErr, &target) {
		t.Fatal("errors.As(*APIError) = false, want true")
	}
	if target.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", target.StatusCode)
	}

	// errors.Is should match sentinel through Unwrap
	if !errors.Is(apiErr, ErrNotFound) {
		t.Error("errors.Is(apiErr, ErrNotFound) = false, want true")
	}
}

func TestAPIErrorMessage(t *testing.T) {
	apiErr := newAPIError(403, "rate limit exceeded")
	want := "github API 403: rate limit exceeded"
	if got := apiErr.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestRESTClientReturnsAPIErrorOnHTTPFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		sentinel   error
	}{
		{"401 Unauthorized", 401, ErrUnauthorized},
		{"403 Rate Limited", 403, ErrRateLimited},
		{"404 Not Found", 404, ErrNotFound},
		{"409 Conflict", 409, ErrConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{"message":"error"}`))
			}))
			defer server.Close()

			cfg := config.GitHubConfig{
				Token:          "test-token",
				Repos:          []string{"owner/repo"},
				EligibleLabels: []string{"gopilot"},
			}
			client := NewRESTClient(cfg, server.URL+"/")
			ctx := context.Background()

			// FetchCandidateIssues
			_, err := client.FetchCandidateIssues(ctx)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.sentinel) {
				t.Errorf("FetchCandidateIssues: errors.Is(err, %v) = false; err = %v", tt.sentinel, err)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Errorf("FetchCandidateIssues: errors.As(*APIError) = false; err = %v", err)
			}
		})
	}
}

func TestRESTClientMethodsReturnSentinelErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()

	cfg := config.GitHubConfig{
		Token:          "bad-token",
		Repos:          []string{"owner/repo"},
		EligibleLabels: []string{"gopilot"},
	}
	client := NewRESTClient(cfg, server.URL+"/")
	ctx := context.Background()

	// Test each method returns ErrUnauthorized for 401
	tests := []struct {
		name string
		fn   func() error
	}{
		{"FetchIssueState", func() error { _, err := client.FetchIssueState(ctx, "owner/repo", 1); return err }},
		{"AddComment", func() error { return client.AddComment(ctx, "owner/repo", 1, "hi") }},
		{"AddLabel", func() error { return client.AddLabel(ctx, "owner/repo", 1, "bug") }},
		{"FetchIssueComments", func() error { _, err := client.FetchIssueComments(ctx, "owner/repo", 1); return err }},
		{"RemoveLabel", func() error { return client.RemoveLabel(ctx, "owner/repo", 1, "bug") }},
		{"CreateIssue", func() error { _, err := client.CreateIssue(ctx, "owner/repo", "t", "b", nil); return err }},
		{"AddSubIssue", func() error { return client.AddSubIssue(ctx, "owner/repo", 1, 2) }},
		{"CreateRepoLabel", func() error { return client.CreateRepoLabel(ctx, "owner/repo", "x", "fff", "d") }},
		{"UpdateRepoLabel", func() error { return client.UpdateRepoLabel(ctx, "owner/repo", "x", "fff", "d") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrUnauthorized) {
				t.Errorf("errors.Is(err, ErrUnauthorized) = false; err = %v", err)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Errorf("errors.As(*APIError) = false; err = %v", err)
			}
		})
	}
}

func TestGraphQLClientReturnsAPIErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"rate limit exceeded"}`))
	}))
	defer server.Close()

	cfg := config.GitHubConfig{
		Token:   "test-token",
		Project: config.ProjectConfig{Owner: "testuser", Number: 1},
	}
	gql := NewGraphQLClient(cfg, server.URL+"/graphql")

	_, err := gql.DiscoverProjectFields(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("errors.Is(err, ErrRateLimited) = false; err = %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Errorf("errors.As(*APIError) = false; err = %v", err)
	}
}
