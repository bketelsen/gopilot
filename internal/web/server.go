package web

import (
	"encoding/json"
	"net/http"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/web/templates/layouts"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// StateProvider abstracts access to orchestrator state to avoid circular imports.
type StateProvider interface {
	AllRunning() []*domain.RunEntry
}

// MetricsProvider abstracts access to metrics counters.
type MetricsProvider interface {
	All() map[string]int64
}

type Server struct {
	router  chi.Router
	state   StateProvider
	cfg     *config.Config
	sseHub  *SSEHub
	metrics MetricsProvider
}

func NewServer(state StateProvider, cfg *config.Config, metrics MetricsProvider) *Server {
	s := &Server{
		state:   state,
		cfg:     cfg,
		sseHub:  NewSSEHub(),
		metrics: metrics,
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
		r.Get("/metrics", s.handleMetrics)
		r.Get("/events", s.sseHub.HandleSSE)
	})

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))
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

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.metrics != nil {
		json.NewEncoder(w).Encode(s.metrics.All())
	} else {
		json.NewEncoder(w).Encode(map[string]int64{})
	}
}

func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	component := layouts.Base("Dashboard")
	component.Render(r.Context(), w)
}
