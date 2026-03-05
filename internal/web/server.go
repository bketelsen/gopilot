package web

import (
	"encoding/json"
	"net/http"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/orchestrator"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	router chi.Router
	state  *orchestrator.State
	cfg    *config.Config
	sseHub *SSEHub
}

func NewServer(state *orchestrator.State, cfg *config.Config) *Server {
	s := &Server{
		state:  state,
		cfg:    cfg,
		sseHub: NewSSEHub(),
	}
	s.router = s.buildRouter()
	return s
}

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/state", s.handleState)
		r.Get("/events", s.sseHub.HandleSSE)
	})

	r.Get("/", s.handleDashboardPage)

	return r
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) SSEHub() *SSEHub {
	return s.sseHub
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running_count": len(running),
		"running":       running,
	})
}

func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html><body><h1>Gopilot Dashboard</h1><p>Coming soon.</p></body></html>"))
}
