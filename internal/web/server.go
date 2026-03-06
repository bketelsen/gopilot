package web

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/web/templates/pages"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// StateProvider abstracts access to orchestrator state to avoid circular imports.
type StateProvider interface {
	AllRunning() []*domain.RunEntry
	RunningCount() int
	GetRunning(issueID int) *domain.RunEntry
	GetHistory(issueID int) []domain.CompletedRun
}

// MetricsProvider abstracts access to metrics counters.
type MetricsProvider interface {
	All() map[string]int64
}

// RetryProvider abstracts access to the retry queue.
type RetryProvider interface {
	All() []*domain.RetryEntry
	Len() int
}

// PlanningProvider abstracts access to planning state.
type PlanningProvider interface {
	AllPlanning() []*domain.PlanningEntry
	PlanningCount() int
}

type Server struct {
	router         chi.Router
	state          StateProvider
	cfg            *config.Config
	sseHub         *SSEHub
	metrics        MetricsProvider
	retries        RetryProvider
	planning       PlanningProvider
	triggerRefresh func()
}

func NewServer(state StateProvider, cfg *config.Config, metrics MetricsProvider, retries RetryProvider, planning ...PlanningProvider) *Server {
	s := &Server{
		state:   state,
		cfg:     cfg,
		sseHub:  NewSSEHub(),
		metrics: metrics,
		retries: retries,
	}
	if len(planning) > 0 {
		s.planning = planning[0]
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
		r.Get("/dashboard", s.handleDashboardFragment)
		r.Get("/issues/{owner}/{repo}/{id}", s.handleIssueDetailAPI)
		r.Get("/sprint", s.handleSprintAPI)
		r.Post("/refresh", s.handleRefresh)
	})

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))
	r.Get("/", s.handleDashboardPage)
	r.Get("/issues/{owner}/{repo}/{id}", s.handleIssueDetail)
	r.Get("/sprint", s.handleSprintPage)
	r.Get("/settings", s.handleSettingsPage)

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
	running := s.state.AllRunning()
	var retries []*domain.RetryEntry
	if s.retries != nil {
		retries = s.retries.All()
	}
	var planningEntries []*domain.PlanningEntry
	if s.planning != nil {
		planningEntries = s.planning.AllPlanning()
	}
	m := map[string]int64{}
	if s.metrics != nil {
		m = s.metrics.All()
	}

	component := pages.Dashboard(running, retries, planningEntries, m, s.cfg.Polling.MaxConcurrentAgents)
	component.Render(r.Context(), w)
}

func (s *Server) handleDashboardFragment(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	var retries []*domain.RetryEntry
	if s.retries != nil {
		retries = s.retries.All()
	}
	var planningEntries []*domain.PlanningEntry
	if s.planning != nil {
		planningEntries = s.planning.AllPlanning()
	}
	m := map[string]int64{}
	if s.metrics != nil {
		m = s.metrics.All()
	}
	component := pages.DashboardContent(running, retries, planningEntries, m, s.cfg.Polling.MaxConcurrentAgents)
	component.Render(r.Context(), w)
}

func (s *Server) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid issue ID", http.StatusBadRequest)
		return
	}
	fullRepo := owner + "/" + repo
	running := s.state.GetRunning(id)
	history := s.state.GetHistory(id)
	component := pages.IssueDetail(running, history, id, fullRepo)
	component.Render(r.Context(), w)
}

func (s *Server) handleIssueDetailAPI(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid issue ID", http.StatusBadRequest)
		return
	}
	_ = owner + "/" + repo
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running": s.state.GetRunning(id),
		"history": s.state.GetHistory(id),
	})
}

func (s *Server) handleSprintPage(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	byStatus := map[string][]domain.Issue{
		"Todo": {}, "In Progress": {}, "In Review": {}, "Done": {},
	}
	iteration := ""
	for _, entry := range running {
		status := entry.Issue.Status
		if _, ok := byStatus[status]; !ok {
			status = "In Progress"
		}
		byStatus[status] = append(byStatus[status], entry.Issue)
		if entry.Issue.Iteration != "" {
			iteration = entry.Issue.Iteration
		}
	}
	total := 0
	for _, issues := range byStatus {
		total += len(issues)
	}
	data := pages.SprintData{
		Iteration: iteration,
		ByStatus:  byStatus,
		Total:     total,
		Done:      len(byStatus["Done"]),
	}
	component := pages.Sprint(data)
	component.Render(r.Context(), w)
}

func (s *Server) handleSprintAPI(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running_count": len(running),
		"running":       running,
	})
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	agentValid := map[string]bool{}
	agentValid[s.cfg.Agent.Command] = isCommandAvailable(s.cfg.Agent.Command)
	for _, override := range s.cfg.Agent.Overrides {
		agentValid[override.Command] = isCommandAvailable(override.Command)
	}

	m := map[string]int64{}
	if s.metrics != nil {
		m = s.metrics.All()
	}
	data := pages.SettingsData{
		Config:        s.cfg,
		Skills:        nil, // TODO: wire skills provider
		AgentValid:    agentValid,
		RateRemaining: int(m["github_rate_limit_remaining"]),
		RateLimit:     int(m["github_rate_limit_limit"]),
	}
	component := pages.Settings(data)
	component.Render(r.Context(), w)
}

func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// SetRefreshFunc sets the callback invoked by POST /api/v1/refresh.
func (s *Server) SetRefreshFunc(fn func()) {
	s.triggerRefresh = fn
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if s.triggerRefresh != nil {
		s.triggerRefresh()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
}
