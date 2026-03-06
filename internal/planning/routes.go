package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/go-chi/chi/v5"
)

// IssueCreator is the subset of github.Client needed for plan output.
type IssueCreator interface {
	CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*domain.Issue, error)
	AddSubIssue(ctx context.Context, repo string, parentID, childID int) error
}

// Routes registers HTTP endpoints for planning sessions.
type Routes struct {
	mgr     *Manager
	handler *Handler
	github  IssueCreator
}

// NewRoutes creates planning routes with the given manager and agent runner.
func NewRoutes(mgr *Manager, runner agent.Runner, cfg HandlerConfig) *Routes {
	return &Routes{
		mgr:     mgr,
		handler: NewHandler(mgr, runner, cfg),
		github:  cfg.GitHubClient,
	}
}

// Register adds planning routes to the given chi router.
func (rt *Routes) Register(r chi.Router) {
	r.Route("/api/planning", func(r chi.Router) {
		r.Post("/sessions", rt.createSession)
		r.Get("/sessions", rt.listSessions)
		r.Get("/sessions/{id}/ws", rt.websocket)
		r.Post("/output", rt.createOutput)
	})
}

type createRequest struct {
	Repo        string `json:"repo"`
	LinkedIssue *int   `json:"linked_issue,omitempty"`
}

func (rt *Routes) createSession(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Repo == "" {
		http.Error(w, "repo is required", http.StatusBadRequest)
		return
	}

	sess, err := rt.mgr.Create(req.Repo, req.LinkedIssue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sess) //nolint:errcheck // HTTP response write
}

func (rt *Routes) listSessions(w http.ResponseWriter, _ *http.Request) {
	sessions := rt.mgr.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // HTTP response write
		"sessions": sessions,
	})
}

func (rt *Routes) websocket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rt.handler.HandleWebSocket(w, r, id)
}

type outputRequest struct {
	SessionID string `json:"session_id"`
	Action    string `json:"action"` // "issues", "doc", "both"
}

func (rt *Routes) createOutput(w http.ResponseWriter, r *http.Request) {
	var req outputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	sess := rt.mgr.Get(req.SessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Find the last agent message containing a plan
	var planText string
	sess.mu.Lock()
	for i := len(sess.Messages) - 1; i >= 0; i-- {
		if sess.Messages[i].Role == "agent" && strings.Contains(sess.Messages[i].Content, "## Plan:") {
			planText = sess.Messages[i].Content
			break
		}
	}
	sess.mu.Unlock()

	if planText == "" {
		http.Error(w, "no plan found in conversation", http.StatusBadRequest)
		return
	}

	plan, err := ParsePlan(planText)
	if err != nil {
		http.Error(w, "failed to parse plan: "+err.Error(), http.StatusBadRequest)
		return
	}

	result := map[string]any{}

	if req.Action == "issues" || req.Action == "both" {
		if rt.github == nil {
			http.Error(w, "GitHub client not configured", http.StatusInternalServerError)
			return
		}
		created, err := rt.createIssuesFromPlan(r.Context(), sess, plan)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result["issues_created"] = created
	}

	if req.Action == "doc" || req.Action == "both" {
		doc := PlanToMarkdown(plan)
		result["document"] = doc
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result) //nolint:errcheck // HTTP response write
}

func (rt *Routes) createIssuesFromPlan(ctx context.Context, sess *Session, plan *Plan) (int, error) {
	count := 0
	for _, phase := range plan.Phases {
		for _, task := range phase.Tasks {
			if !task.Checked {
				continue
			}
			body := fmt.Sprintf("Phase: %s\nComplexity: %s\n", phase.Name, task.Complexity)
			if task.Dependencies != "" && task.Dependencies != "none" {
				body += fmt.Sprintf("Dependencies: %s\n", task.Dependencies)
			}
			_, err := rt.github.CreateIssue(ctx, sess.Repo, task.Description, body, []string{"gopilot"})
			if err != nil {
				return count, fmt.Errorf("creating issue %q: %w", task.Description, err)
			}
			count++
		}
	}
	return count, nil
}
