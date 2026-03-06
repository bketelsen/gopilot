package planning

import (
	"encoding/json"
	"net/http"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/go-chi/chi/v5"
)

// Routes registers HTTP endpoints for planning sessions.
type Routes struct {
	mgr     *Manager
	handler *Handler
}

// NewRoutes creates planning routes with the given manager and agent runner.
func NewRoutes(mgr *Manager, runner agent.Runner, cfg HandlerConfig) *Routes {
	return &Routes{
		mgr:     mgr,
		handler: NewHandler(mgr, runner, cfg),
	}
}

// Register adds planning routes to the given chi router.
func (rt *Routes) Register(r chi.Router) {
	r.Route("/api/planning", func(r chi.Router) {
		r.Post("/sessions", rt.createSession)
		r.Get("/sessions", rt.listSessions)
		r.Get("/sessions/{id}/ws", rt.websocket)
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
	json.NewEncoder(w).Encode(sess)
}

func (rt *Routes) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions := rt.mgr.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sessions": sessions,
	})
}

func (rt *Routes) websocket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rt.handler.HandleWebSocket(w, r, id)
}
