package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strconv"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/bketelsen/gopilot/internal/skills"
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

// SprintProvider abstracts access to sprint/issue data from GitHub.
type SprintProvider interface {
	FetchLabeledIssues(ctx context.Context, label string) ([]domain.Issue, error)
	FetchLinkedPullRequests(ctx context.Context, repo string, issueNumber int) ([]domain.PullRequest, error)
}

// Server is the web dashboard HTTP server.
type Server struct {
	router         chi.Router
	state          StateProvider
	cfg            *config.Config
	sseHub         *SSEHub
	metrics        MetricsProvider
	retries        RetryProvider
	planning       PlanningProvider
	sprint         SprintProvider
	planningMgr    *planning.Manager
	skills         []*skills.Skill
	triggerRefresh func()
}

// NewServer creates a dashboard server wired to the given providers.
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
	r.Get("/planning", s.handlePlanningListPage)
	r.Get("/planning/{id}", s.handlePlanningChatPage)

	return r
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// SSEHub returns the server's SSE event hub.
func (s *Server) SSEHub() *SSEHub {
	return s.sseHub
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck // HTTP response write
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	running := s.state.AllRunning()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // HTTP response write
		"running_count": len(running),
		"running":       running,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.metrics != nil {
		json.NewEncoder(w).Encode(s.metrics.All()) //nolint:errcheck // HTTP response write
	} else {
		json.NewEncoder(w).Encode(map[string]int64{}) //nolint:errcheck // HTTP response write
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
	component.Render(r.Context(), w) //nolint:errcheck // best-effort template render
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
	component.Render(r.Context(), w) //nolint:errcheck // best-effort template render
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
	component.Render(r.Context(), w) //nolint:errcheck // best-effort template render
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
	running := s.state.GetRunning(id)
	resp := map[string]any{
		"running": running,
		"history": s.state.GetHistory(id),
	}
	if running != nil && running.OutputLines != nil {
		resp["output_lines"] = running.OutputLines.Lines()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck // HTTP response write
}

func (s *Server) handleSprintPage(w http.ResponseWriter, r *http.Request) {
	data := s.buildSprintData(r.Context())
	component := pages.Sprint(data)
	component.Render(r.Context(), w) //nolint:errcheck // best-effort template render
}

// buildSprintData assembles sprint board data from all available sources.
func (s *Server) buildSprintData(ctx context.Context) pages.SprintData {
	byStatus := map[string][]domain.Issue{
		"Todo": {}, "In Progress": {}, "In Review": {}, "Done": {},
	}
	iteration := ""

	// Build a set of running issue IDs for quick lookup
	running := s.state.AllRunning()
	runningIDs := make(map[int]bool, len(running))
	for _, entry := range running {
		runningIDs[entry.Issue.ID] = true
		if entry.Issue.Iteration != "" {
			iteration = entry.Issue.Iteration
		}
	}

	if s.sprint != nil && s.cfg != nil && len(s.cfg.GitHub.EligibleLabels) > 0 {
		label := s.cfg.GitHub.EligibleLabels[0]
		allIssues, err := s.sprint.FetchLabeledIssues(ctx, label)
		if err != nil {
			log.Printf("sprint: failed to fetch labeled issues: %v", err)
		} else {
			// Enrich issues with linked PR data and categorize
			for _, issue := range allIssues {
				prs, err := s.sprint.FetchLinkedPullRequests(ctx, issue.Repo, issue.ID)
				if err != nil {
					log.Printf("sprint: failed to fetch PRs for %s#%d: %v", issue.Repo, issue.ID, err)
				}
				issue.LinkedPRs = prs

				switch {
				case issue.HasMergedPR():
					byStatus["Done"] = append(byStatus["Done"], issue)
				case issue.HasOpenPR():
					byStatus["In Review"] = append(byStatus["In Review"], issue)
				case runningIDs[issue.ID]:
					byStatus["In Progress"] = append(byStatus["In Progress"], issue)
				case issue.Status == "Done":
					byStatus["Done"] = append(byStatus["Done"], issue)
				default:
					byStatus["Todo"] = append(byStatus["Todo"], issue)
				}

				if issue.Iteration != "" {
					iteration = issue.Iteration
				}
			}
		}
	} else {
		// Fallback: use only running agents (legacy behavior)
		for _, entry := range running {
			byStatus["In Progress"] = append(byStatus["In Progress"], entry.Issue)
		}
	}

	total := 0
	for _, issues := range byStatus {
		total += len(issues)
	}

	return pages.SprintData{
		Iteration: iteration,
		ByStatus:  byStatus,
		Total:     total,
		Done:      len(byStatus["Done"]),
	}
}

func (s *Server) handleSprintAPI(w http.ResponseWriter, r *http.Request) {
	data := s.buildSprintData(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // HTTP response write
		"iteration": data.Iteration,
		"by_status": data.ByStatus,
		"total":     data.Total,
		"done":      data.Done,
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
	var skillDisplays []pages.SkillDisplay
	for _, sk := range s.skills {
		skillDisplays = append(skillDisplays, pages.SkillDisplay{
			Name:        sk.Name,
			Type:        sk.Type,
			Description: sk.Description,
		})
	}

	data := pages.SettingsData{
		Config:        s.cfg,
		Skills:        skillDisplays,
		AgentValid:    agentValid,
		RateRemaining: int(m["github_rate_limit_remaining"]),
		RateLimit:     int(m["github_rate_limit_limit"]),
	}
	component := pages.Settings(data)
	component.Render(r.Context(), w) //nolint:errcheck // best-effort template render
}

func (s *Server) handlePlanningListPage(w http.ResponseWriter, r *http.Request) {
	var sessions []*planning.Session
	if s.planningMgr != nil {
		sessions = s.planningMgr.List()
	}
	repos := s.cfg.GitHub.Repos
	component := pages.PlanningList(sessions, repos)
	component.Render(r.Context(), w) //nolint:errcheck // best-effort template render
}

func (s *Server) handlePlanningChatPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.planningMgr == nil {
		http.Error(w, "planning not configured", http.StatusNotFound)
		return
	}
	sess := s.planningMgr.Get(id)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	component := pages.PlanningChat(sess)
	component.Render(r.Context(), w) //nolint:errcheck // best-effort template render
}

func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// SetSprintProvider configures the data source for the sprint view.
func (s *Server) SetSprintProvider(sp SprintProvider) {
	s.sprint = sp
}
// SetSkills sets the loaded skills for display on the settings page.
func (s *Server) SetSkills(sk []*skills.Skill) {
	s.skills = sk
}

// SetRefreshFunc sets the callback invoked by POST /api/v1/refresh.
func (s *Server) SetRefreshFunc(fn func()) {
	s.triggerRefresh = fn
}

// SetPlanningManager configures the planning manager and registers its routes.
func (s *Server) SetPlanningManager(mgr *planning.Manager, runner agent.Runner, cfg planning.HandlerConfig) {
	s.planningMgr = mgr
	routes := planning.NewRoutes(mgr, runner, cfg)
	routes.Register(s.router)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if s.triggerRefresh != nil {
		s.triggerRefresh()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "triggered"}) //nolint:errcheck // HTTP response write
}
