package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/workspace"
)

// Orchestrator runs the poll-dispatch-reconcile loop.
type Orchestrator struct {
	cfg        *config.Config
	gh         *github.Client
	ws         *workspace.Manager
	runner     agent.AgentRunner
	state      *State
	dispatcher *Dispatcher
	meta       *github.ProjectMeta
}

// New creates a new Orchestrator from the given config.
func New(cfg *config.Config) (*Orchestrator, error) {
	ghClient := github.NewClient(cfg.GitHub.Token)

	ws := workspace.NewManager(
		cfg.Workspace.Root,
		workspace.Hooks{
			AfterCreate:  cfg.Workspace.Hooks.AfterCreate,
			BeforeRun:    cfg.Workspace.Hooks.BeforeRun,
			AfterRun:     cfg.Workspace.Hooks.AfterRun,
			BeforeRemove: cfg.Workspace.Hooks.BeforeRemove,
		},
		cfg.Workspace.HookTimeoutMS,
	)

	runner := agent.NewCopilotRunner(cfg.Agent.Command)

	o := &Orchestrator{
		cfg:    cfg,
		gh:     ghClient,
		ws:     ws,
		runner: runner,
		state:  NewState(),
	}

	return o, nil
}

// Run starts the orchestrator loop. It blocks until ctx is cancelled.
func (o *Orchestrator) Run(ctx context.Context) error {
	// Discover project metadata
	meta, err := o.gh.DiscoverProject(ctx, o.cfg.GitHub.Project.Owner, o.cfg.GitHub.Project.Number)
	if err != nil {
		return fmt.Errorf("discover project: %w", err)
	}
	o.meta = meta

	o.dispatcher = NewDispatcher(
		o.gh, o.ws, o.runner, o.state, meta, o.cfg.Prompt,
		agentDispatchConfig{
			MaxContinues: o.cfg.Agent.MaxAutopilotContinues,
			Model:        o.cfg.Agent.Model,
			TurnTimeout:  time.Duration(o.cfg.Agent.TurnTimeoutMS) * time.Millisecond,
		},
	)

	interval := time.Duration(o.cfg.Polling.IntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("orchestrator started",
		"interval", interval,
		"max_concurrent", o.cfg.Polling.MaxConcurrent,
		"repos", strings.Join(o.cfg.GitHub.Repos, ", "),
	)

	// Run first tick immediately
	o.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("orchestrator shutting down")
			o.shutdown()
			return nil
		case <-ticker.C:
			o.tick(ctx)
		}
	}
}

func (o *Orchestrator) tick(ctx context.Context) {
	// Reconcile completed runs
	o.reconcile()

	// Check capacity
	running := o.state.RunningCount()
	available := o.cfg.Polling.MaxConcurrent - running
	if available <= 0 {
		slog.Debug("at capacity", "running", running, "max", o.cfg.Polling.MaxConcurrent)
		return
	}

	// Fetch candidates
	candidates, err := o.gh.FetchCandidates(ctx, github.CandidateOpts{
		Repos:          o.cfg.GitHub.Repos,
		EligibleLabels: o.cfg.GitHub.EligibleLabels,
		ExcludedLabels: o.cfg.GitHub.ExcludedLabels,
		ProjectMeta:    o.meta,
		ExcludeIDs:     o.state.RunningIssueIDs(),
	})
	if err != nil {
		slog.Error("fetch candidates failed", "error", err)
		return
	}

	if len(candidates) == 0 {
		slog.Debug("no eligible candidates found")
		return
	}

	slog.Info("found candidates", "count", len(candidates), "available_slots", available)

	// Dispatch up to available slots
	dispatched := 0
	for _, issue := range candidates {
		if dispatched >= available {
			break
		}

		if o.state.IsRunningOrClaimed(issue) {
			continue
		}

		if err := o.dispatcher.Dispatch(ctx, issue); err != nil {
			slog.Error("dispatch failed",
				"issue", fmt.Sprintf("%s#%d", issue.Repo, issue.ID),
				"error", err,
			)
			continue
		}
		dispatched++
	}
}

func (o *Orchestrator) reconcile() {
	for _, entry := range o.state.AllRunning() {
		select {
		case <-entry.Session.Done:
			// Agent has exited — the waitForCompletion goroutine handles cleanup
			// but we double-check here
			o.state.RemoveRunning(entry.Issue)
			slog.Debug("reconciled completed run", "issue", fmt.Sprintf("%s#%d", entry.Issue.Repo, entry.Issue.ID))
		default:
			// Still running
		}
	}
}

// DryRun fetches candidates and prints them without dispatching.
func (o *Orchestrator) DryRun(ctx context.Context) error {
	meta, err := o.gh.DiscoverProject(ctx, o.cfg.GitHub.Project.Owner, o.cfg.GitHub.Project.Number)
	if err != nil {
		return fmt.Errorf("discover project: %w", err)
	}

	candidates, err := o.gh.FetchCandidates(ctx, github.CandidateOpts{
		Repos:          o.cfg.GitHub.Repos,
		EligibleLabels: o.cfg.GitHub.EligibleLabels,
		ExcludedLabels: o.cfg.GitHub.ExcludedLabels,
		ProjectMeta:    meta,
	})
	if err != nil {
		return fmt.Errorf("fetch candidates: %w", err)
	}

	if len(candidates) == 0 {
		fmt.Println("No eligible candidates found.")
		return nil
	}

	fmt.Printf("Found %d eligible candidate(s):\n\n", len(candidates))
	for i, issue := range candidates {
		fmt.Printf("  %d. %s#%d — %s\n", i+1, issue.Repo, issue.ID, issue.Title)
		fmt.Printf("     Status: %s  Priority: %d  Labels: [%s]\n",
			issue.Status, issue.Priority, strings.Join(issue.Labels, ", "))
		fmt.Printf("     URL: %s\n\n", issue.URL)
	}

	return nil
}

func (o *Orchestrator) shutdown() {
	entries := o.state.AllRunning()
	if len(entries) == 0 {
		return
	}

	slog.Info("stopping running agents", "count", len(entries))
	for _, entry := range entries {
		slog.Info("stopping agent", "issue", fmt.Sprintf("%s#%d", entry.Issue.Repo, entry.Issue.ID), "session", entry.Session.ID)
		entry.Session.Stop()
	}

	// Wait for agents to exit (with timeout)
	timeout := time.After(15 * time.Second)
	for _, entry := range entries {
		select {
		case <-entry.Session.Done:
		case <-timeout:
			slog.Warn("timed out waiting for agent to exit", "session", entry.Session.ID)
			return
		}
	}
}
