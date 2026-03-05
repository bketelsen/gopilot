package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	gh "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/metrics"
	"github.com/bketelsen/gopilot/internal/prompt"
	"github.com/bketelsen/gopilot/internal/skills"
	"github.com/bketelsen/gopilot/internal/web"
	"github.com/bketelsen/gopilot/internal/workspace"
)

// Orchestrator runs the poll-dispatch-reconcile loop.
type Orchestrator struct {
	cfg        *config.Config
	github     gh.Client
	agent      agent.Runner
	workspace  workspace.Manager
	state        *State
	retryQueue   *RetryQueue
	sessionsMu   sync.Mutex
	sessions     map[int]*agent.Session
	configPath   string
	skills       []*skills.Skill
	sseHub       *web.SSEHub
	metrics      *metrics.Counters
	tokenTracker *metrics.TokenTracker
}

// NewOrchestrator creates a new orchestrator.
func NewOrchestrator(cfg *config.Config, github gh.Client, agentRunner agent.Runner, configPath ...string) *Orchestrator {
	o := &Orchestrator{
		cfg:        cfg,
		github:     github,
		agent:      agentRunner,
		workspace:    workspace.NewFSManager(cfg.Workspace),
		state:        NewState(),
		retryQueue:   NewRetryQueue(),
		sessions:     make(map[int]*agent.Session),
		metrics:      metrics.NewCounters(),
		tokenTracker: metrics.NewTokenTracker(),
	}
	if len(configPath) > 0 {
		o.configPath = configPath[0]
	}

	allSkills, err := skills.LoadFromDir(cfg.Skills.Dir)
	if err != nil {
		slog.Warn("failed to load skills", "error", err)
	}
	o.skills = allSkills

	return o
}

// Run starts the main loop until context is canceled.
func (o *Orchestrator) Run(ctx context.Context) error {
	slog.Info("orchestrator started",
		"poll_interval", o.cfg.PollInterval(),
		"max_agents", o.cfg.Polling.MaxConcurrentAgents,
	)

	if o.configPath != "" {
		watcher, err := config.Watch(o.configPath, func(newCfg *config.Config, loadErr error) {
			if loadErr != nil {
				slog.Error("config reload failed, keeping current config", "error", loadErr)
				return
			}
			slog.Info("config reloaded successfully")
			o.cfg.Polling.IntervalMS = newCfg.Polling.IntervalMS
			o.cfg.Polling.MaxConcurrentAgents = newCfg.Polling.MaxConcurrentAgents
			o.cfg.Agent.StallTimeoutMS = newCfg.Agent.StallTimeoutMS
			o.cfg.Agent.TurnTimeoutMS = newCfg.Agent.TurnTimeoutMS
			o.cfg.Agent.MaxRetries = newCfg.Agent.MaxRetries
			o.cfg.Agent.MaxRetryBackoffMS = newCfg.Agent.MaxRetryBackoffMS
			o.cfg.Agent.MaxAutopilotContinues = newCfg.Agent.MaxAutopilotContinues
			o.cfg.Skills = newCfg.Skills
			o.cfg.Prompt = newCfg.Prompt
		})
		if err != nil {
			slog.Warn("failed to start config watcher", "error", err)
		}
		if watcher != nil {
			defer watcher.Close()
		}
	}

	if o.cfg.Dashboard.Enabled {
		webSrv := web.NewServer(o.state, o.cfg, o.metrics)
		o.sseHub = webSrv.SSEHub()
		go func() {
			slog.Info("dashboard starting", "addr", o.cfg.Dashboard.Addr)
			if err := http.ListenAndServe(o.cfg.Dashboard.Addr, webSrv); err != nil {
				slog.Error("dashboard server error", "error", err)
			}
		}()
	}

	ticker := time.NewTicker(o.cfg.PollInterval())
	defer ticker.Stop()

	o.Tick(ctx)
	if o.sseHub != nil {
		o.sseHub.Broadcast("agent-update", "refresh")
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("orchestrator shutting down")
			o.shutdown()
			return nil
		case <-ticker.C:
			o.Tick(ctx)
			if o.sseHub != nil {
				o.sseHub.Broadcast("agent-update", "refresh")
			}
		}
	}
}

// DryRun fetches and displays eligible issues without dispatching.
func (o *Orchestrator) DryRun(ctx context.Context) error {
	issues, err := o.github.FetchCandidateIssues(ctx)
	if err != nil {
		return err
	}
	domain.SortByPriority(issues)

	slog.Info("dry run", "eligible_issues", len(issues))
	for _, issue := range issues {
		slog.Info("eligible",
			"issue", issue.Identifier(),
			"title", issue.Title,
			"priority", issue.Priority,
			"status", issue.Status,
		)
	}
	return nil
}

// Tick runs one iteration of the poll-dispatch-reconcile loop.
func (o *Orchestrator) Tick(ctx context.Context) {
	o.reconcile(ctx)
	o.detectStalls(ctx)

	// Process due retries before dispatching new candidates.
	for _, retry := range o.retryQueue.DueEntries() {
		if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
			// Re-enqueue if no slots available.
			maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
			o.retryQueue.Enqueue(retry.IssueID, retry.Identifier, retry.Attempt, retry.Error, maxBackoff)
			continue
		}
		slog.Info("retrying issue", "issue", retry.Identifier, "attempt", retry.Attempt)
		// Fetch fresh issue state for retry.
		issue, err := o.github.FetchIssueState(ctx, "", retry.IssueID)
		if err != nil || issue == nil {
			slog.Warn("retry: could not fetch issue state", "issue_id", retry.IssueID, "error", err)
			continue
		}
		o.state.Release(issue.ID) // Release claim so dispatch can re-claim
		o.dispatch(ctx, *issue, retry.Attempt)
	}

	issues, err := o.github.FetchCandidateIssues(ctx)
	if err != nil {
		slog.Error("failed to fetch candidates", "error", err)
		return
	}

	var candidates []domain.Issue
	for _, issue := range issues {
		if o.state.IsClaimed(issue.ID) || o.state.GetRunning(issue.ID) != nil || o.state.IsInRetryQueue(issue.ID) || o.retryQueue.Has(issue.ID) {
			continue
		}
		candidates = append(candidates, issue)
	}

	domain.SortByPriority(candidates)

	for _, issue := range candidates {
		if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
			break
		}
		o.dispatch(ctx, issue, 1)
	}
}

func (o *Orchestrator) dispatch(ctx context.Context, issue domain.Issue, attempt int) {
	if !o.state.Claim(issue.ID) {
		return
	}

	log := slog.With("issue", issue.Identifier(), "attempt", attempt)

	wsPath, err := o.workspace.Ensure(ctx, issue)
	if err != nil {
		log.Error("workspace ensure failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	if err := o.workspace.RunHook(ctx, "before_run", wsPath, issue); err != nil {
		log.Error("before_run hook failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	skillText := skills.InjectSkills(o.skills, o.cfg.Skills.Required, o.cfg.Skills.Optional)
	rendered, err := prompt.Render(o.cfg.Prompt, issue, attempt, skillText)
	if err != nil {
		log.Error("prompt render failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	if err := o.github.SetProjectStatus(ctx, issue, "In Progress"); err != nil {
		log.Warn("failed to set status to In Progress", "error", err)
	}

	opts := agent.AgentOpts{
		Model:            o.cfg.Agent.Model,
		MaxContinuations: o.cfg.Agent.MaxAutopilotContinues,
	}
	sess, err := o.agent.Start(ctx, wsPath, rendered, opts)
	if err != nil {
		log.Error("agent start failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	o.sessionsMu.Lock()
	o.sessions[issue.ID] = sess
	o.sessionsMu.Unlock()
	o.metrics.Increment("issues_dispatched")

	now := time.Now()
	entry := &domain.RunEntry{
		Issue:       issue,
		SessionID:   sess.ID,
		ProcessPID:  sess.PID,
		StartedAt:   now,
		LastEventAt: now,
		Attempt:     attempt,
	}
	o.state.AddRunning(issue.ID, entry)

	log.Info("agent dispatched",
		"session_id", sess.ID,
		"pid", sess.PID,
		"workspace", wsPath,
	)

	go o.monitorAgent(issue, sess, entry)
}

func (o *Orchestrator) monitorAgent(issue domain.Issue, sess *agent.Session, entry *domain.RunEntry) {
	<-sess.Done

	log := slog.With("issue", issue.Identifier(), "session_id", sess.ID)

	o.state.RemoveRunning(issue.ID)
	o.sessionsMu.Lock()
	delete(o.sessions, issue.ID)
	o.sessionsMu.Unlock()

	if sess.ExitCode == 0 {
		log.Info("agent completed successfully")
		o.metrics.Increment("issues_completed")
		o.state.Release(issue.ID)
	} else {
		log.Warn("agent exited with error", "exit_code", sess.ExitCode, "error", sess.ExitErr)

		errMsg := "exit code " + strconv.Itoa(sess.ExitCode)
		if sess.ExitErr != nil {
			errMsg = sess.ExitErr.Error()
		}
		if entry.Attempt < o.cfg.Agent.MaxRetries {
			maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
			o.retryQueue.Enqueue(issue.ID, issue.Identifier(), entry.Attempt+1, errMsg, maxBackoff)
			log.Info("scheduled retry", "next_attempt", entry.Attempt+1)
		} else {
			o.handleMaxRetriesExceeded(issue, errMsg)
		}
	}
}

func (o *Orchestrator) detectStalls(ctx context.Context) {
	timeout := o.cfg.StallTimeout()
	for _, entry := range o.state.AllRunning() {
		if entry.IsStalled(timeout) {
			log := slog.With("issue", entry.Issue.Identifier(), "session_id", entry.SessionID)
			log.Warn("agent stalled, killing", "last_event", entry.LastEventAt)

			o.sessionsMu.Lock()
			if sess, ok := o.sessions[entry.Issue.ID]; ok {
				o.agent.Stop(sess)
				delete(o.sessions, entry.Issue.ID)
			}
			o.sessionsMu.Unlock()

			o.state.RemoveRunning(entry.Issue.ID)

			duration := time.Since(entry.StartedAt).Round(time.Second)
			comment := fmt.Sprintf("Agent stalled after %s, retrying (attempt %d)", duration, entry.Attempt)
			o.github.AddComment(ctx, entry.Issue.Repo, entry.Issue.ID, comment)

			if entry.Attempt < o.cfg.Agent.MaxRetries {
				maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
				o.retryQueue.Enqueue(entry.Issue.ID, entry.Issue.Identifier(), entry.Attempt+1, "stalled", maxBackoff)
			} else {
				o.handleMaxRetriesExceeded(entry.Issue, "stalled")
			}
		}
	}
}

func (o *Orchestrator) reconcile(ctx context.Context) {
	for _, entry := range o.state.AllRunning() {
		issue, err := o.github.FetchIssueState(ctx, entry.Issue.Repo, entry.Issue.ID)
		if err != nil {
			slog.Warn("reconcile: fetch failed", "issue", entry.Issue.Identifier(), "error", err)
			continue
		}
		if issue == nil {
			continue
		}

		if issue.IsTerminal() {
			slog.Info("reconcile: issue became terminal, stopping agent", "issue", entry.Issue.Identifier(), "status", issue.Status)
			o.stopAndCleanup(ctx, entry, true)
			continue
		}

		if !issue.IsEligible(o.cfg.GitHub.EligibleLabels, o.cfg.GitHub.ExcludedLabels) {
			slog.Info("reconcile: issue no longer eligible, stopping agent", "issue", entry.Issue.Identifier())
			o.stopAndCleanup(ctx, entry, false)
			continue
		}

		entry.Issue = *issue
	}
}

func (o *Orchestrator) stopAndCleanup(ctx context.Context, entry *domain.RunEntry, removeWorkspace bool) {
	o.sessionsMu.Lock()
	if sess, ok := o.sessions[entry.Issue.ID]; ok {
		o.agent.Stop(sess)
		delete(o.sessions, entry.Issue.ID)
	}
	o.sessionsMu.Unlock()

	o.state.RemoveRunning(entry.Issue.ID)
	o.state.Release(entry.Issue.ID)

	o.workspace.RunHook(ctx, "after_run", o.workspace.Path(entry.Issue), entry.Issue)

	if removeWorkspace {
		o.workspace.Cleanup(ctx, entry.Issue)
	}
}

func (o *Orchestrator) handleMaxRetriesExceeded(issue domain.Issue, lastError string) {
	log := slog.With("issue", issue.Identifier())
	log.Error("max retries exceeded", "attempts", o.cfg.Agent.MaxRetries, "last_error", lastError)

	o.metrics.Increment("issues_failed")
	o.state.Release(issue.ID)

	comment := fmt.Sprintf("Gopilot failed after %d attempts. Last error: %s", o.cfg.Agent.MaxRetries, lastError)
	o.github.AddComment(context.Background(), issue.Repo, issue.ID, comment)
	o.github.AddLabel(context.Background(), issue.Repo, issue.ID, "gopilot-failed")
}

func (o *Orchestrator) shutdown() {
	for _, entry := range o.state.AllRunning() {
		slog.Info("stopping agent", "issue", entry.Issue.Identifier())
	}
}
