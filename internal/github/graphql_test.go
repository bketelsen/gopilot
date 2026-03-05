package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/gopilot/internal/config"
)

func TestDiscoverProjectFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"id": "PVT_123",
						"fields": map[string]any{
							"nodes": []any{
								map[string]any{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "PVTSSF_status",
									"name":       "Status",
									"options": []any{
										map[string]any{"id": "opt_todo", "name": "Todo"},
										map[string]any{"id": "opt_ip", "name": "In Progress"},
										map[string]any{"id": "opt_ir", "name": "In Review"},
										map[string]any{"id": "opt_done", "name": "Done"},
									},
								},
								map[string]any{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "PVTSSF_priority",
									"name":       "Priority",
									"options": []any{
										map[string]any{"id": "opt_urgent", "name": "Urgent"},
										map[string]any{"id": "opt_high", "name": "High"},
										map[string]any{"id": "opt_med", "name": "Medium"},
										map[string]any{"id": "opt_low", "name": "Low"},
									},
								},
							},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{
		Token:   "test-token",
		Project: config.ProjectConfig{Owner: "@me", Number: 1},
	}
	gql := NewGraphQLClient(cfg, server.URL+"/graphql")
	meta, err := gql.DiscoverProjectFields(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProjectID != "PVT_123" {
		t.Errorf("ProjectID = %q, want %q", meta.ProjectID, "PVT_123")
	}
	if meta.StatusFieldID != "PVTSSF_status" {
		t.Errorf("StatusFieldID = %q", meta.StatusFieldID)
	}
	if meta.StatusOptions["In Progress"] != "opt_ip" {
		t.Errorf("In Progress option = %q", meta.StatusOptions["In Progress"])
	}
	if meta.PriorityFieldID != "PVTSSF_priority" {
		t.Errorf("PriorityFieldID = %q", meta.PriorityFieldID)
	}
}
