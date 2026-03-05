package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	gh "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/prompt"
	"github.com/bketelsen/gopilot/internal/workspace"
)

// Orchestrator runs the poll-dispatch-reconcile loop.
type Orchestrator struct {
	cfg       *config.Config
	github    gh.Client
	agent     agent.Runner
	workspace workspace.Manager
	state     *State
}

// NewOrchestrator creates a new orchestrator.
func NewOrchestrator(cfg *config.Config, github gh.Client, agentRunner agent.Runner) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		github:    github,
		agent:     agentRunner,
		workspace: workspace.NewFSManager(cfg.Workspace),
		state:     NewState(),
	}
}

// Run starts the main loop until context is canceled.
func (o *Orchestrator) Run(ctx context.Context) error {
	slog.Info("orchestrator started",
		"poll_interval", o.cfg.PollInterval(),
		"max_agents", o.cfg.Polling.MaxConcurrentAgents,
	)

	ticker := time.NewTicker(o.cfg.PollInterval())
	defer ticker.Stop()

	o.Tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("orchestrator shutting down")
			o.shutdown()
			return nil
		case <-ticker.C:
			o.Tick(ctx)
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
	issues, err := o.github.FetchCandidateIssues(ctx)
	if err != nil {
		slog.Error("failed to fetch candidates", "error", err)
		return
	}

	var candidates []domain.Issue
	for _, issue := range issues {
		if o.state.IsClaimed(issue.ID) || o.state.GetRunning(issue.ID) != nil || o.state.IsInRetryQueue(issue.ID) {
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

	rendered, err := prompt.Render(o.cfg.Prompt, issue, attempt, "")
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

	if sess.ExitCode == 0 {
		log.Info("agent completed successfully")
		o.state.Release(issue.ID)
	} else {
		log.Warn("agent exited with error", "exit_code", sess.ExitCode, "error", sess.ExitErr)
		o.state.Release(issue.ID)
	}
}

func (o *Orchestrator) shutdown() {
	for _, entry := range o.state.AllRunning() {
		slog.Info("stopping agent", "issue", entry.Issue.Identifier())
	}
}
