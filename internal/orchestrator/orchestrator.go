package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/workspace"
)

// Orchestrator runs the poll-dispatch-reconcile loop.
type Orchestrator struct {
	cfg           *config.Config
	cfgPath       string
	cfgModTime    time.Time
	gh            *github.Client
	ws            *workspace.Manager
	runner        agent.AgentRunner
	state         *State
	retryQueue    *RetryQueue
	stallDetector *StallDetector
	dispatcher    *Dispatcher
	meta          *github.ProjectMeta
}

// New creates a new Orchestrator from the given config.
func New(cfg *config.Config, cfgPath string) (*Orchestrator, error) {
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

	var cfgModTime time.Time
	if info, err := os.Stat(cfgPath); err == nil {
		cfgModTime = info.ModTime()
	}

	o := &Orchestrator{
		cfg:           cfg,
		cfgPath:       cfgPath,
		cfgModTime:    cfgModTime,
		gh:            ghClient,
		ws:            ws,
		runner:        runner,
		state:         NewState(),
		retryQueue:    NewRetryQueue(cfg.Agent.MaxRetries, cfg.Agent.MaxRetryBackoffMS),
		stallDetector: NewStallDetector(cfg.Agent.StallTimeoutMS),
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
		o.handleCompletion,
	)

	interval := time.Duration(o.cfg.Polling.IntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("orchestrator started",
		"interval", interval,
		"max_concurrent", o.cfg.Polling.MaxConcurrent,
		"repos", strings.Join(o.cfg.GitHub.Repos, ", "),
		"max_retries", o.cfg.Agent.MaxRetries,
		"stall_timeout_ms", o.cfg.Agent.StallTimeoutMS,
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
	// Check for config changes
	o.checkConfigReload()

	// Reconcile completed runs
	o.reconcile()

	// Detect stalled agents
	o.detectStalls()

	// Check capacity
	running := o.state.RunningCount()
	available := o.cfg.Polling.MaxConcurrent - running
	if available <= 0 {
		slog.Debug("at capacity", "running", running, "max", o.cfg.Polling.MaxConcurrent)
		return
	}

	// First, try retries
	dispatched := o.dispatchRetries(ctx, available)
	available -= dispatched

	if available <= 0 {
		return
	}

	// Fetch fresh candidates
	excludeIDs := o.state.RunningIssueIDs()
	// Also exclude retry queue issues
	for id := range o.retryQueue.ExcludeIDs() {
		excludeIDs[id] = true
	}

	candidates, err := o.gh.FetchCandidates(ctx, github.CandidateOpts{
		Repos:          o.cfg.GitHub.Repos,
		EligibleLabels: o.cfg.GitHub.EligibleLabels,
		ExcludedLabels: o.cfg.GitHub.ExcludedLabels,
		ProjectMeta:    o.meta,
		ExcludeIDs:     excludeIDs,
	})
	if err != nil {
		slog.Error("fetch candidates failed", "error", err)
		return
	}

	if len(candidates) == 0 && dispatched == 0 {
		slog.Debug("no eligible candidates found")
		return
	}

	if len(candidates) > 0 {
		slog.Info("found candidates", "count", len(candidates), "available_slots", available)
	}

	// Dispatch up to available slots
	for _, issue := range candidates {
		if dispatched >= available+dispatched { // already at capacity
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

func (o *Orchestrator) dispatchRetries(ctx context.Context, available int) int {
	ready := o.retryQueue.Ready()
	if len(ready) == 0 {
		return 0
	}

	dispatched := 0
	for _, issue := range ready {
		if dispatched >= available {
			break
		}

		if o.state.IsRunningOrClaimed(issue) {
			continue
		}

		entry := o.retryQueue.Get(issue)
		slog.Info("retrying issue",
			"issue", fmt.Sprintf("%s#%d", issue.Repo, issue.ID),
			"attempt", entry.Attempts,
		)

		if err := o.dispatcher.Dispatch(ctx, issue); err != nil {
			slog.Error("retry dispatch failed",
				"issue", fmt.Sprintf("%s#%d", issue.Repo, issue.ID),
				"error", err,
			)
			continue
		}

		o.retryQueue.Remove(issue)
		dispatched++
	}

	return dispatched
}

func (o *Orchestrator) detectStalls() {
	stalled := o.stallDetector.CheckStalled(o.state.AllRunning())
	for _, entry := range stalled {
		slog.Warn("killing stalled agent",
			"issue", fmt.Sprintf("%s#%d", entry.Issue.Repo, entry.Issue.ID),
			"session", entry.Session.ID,
			"running_for", time.Since(entry.StartedAt),
		)
		o.dispatcher.StopRun(entry)
	}
}

// handleCompletion is called by the dispatcher when an agent run finishes.
func (o *Orchestrator) handleCompletion(issue github.Issue, runErr error) {
	if runErr == nil {
		// Success — remove from retry queue if it was there
		o.retryQueue.Remove(issue)
		return
	}

	// Failure — try to enqueue for retry
	if o.retryQueue.Add(issue) {
		entry := o.retryQueue.Get(issue)
		slog.Info("enqueued for retry",
			"issue", fmt.Sprintf("%s#%d", issue.Repo, issue.ID),
			"attempt", entry.Attempts,
			"next_try", entry.NextTry.Format(time.RFC3339),
		)
		return
	}

	// Max retries exceeded — mark as failed
	slog.Error("max retries exceeded",
		"issue", fmt.Sprintf("%s#%d", issue.Repo, issue.ID),
		"max_retries", o.cfg.Agent.MaxRetries,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	owner, repo := ownerRepo(issue.Repo)

	// Add failure label
	if err := o.gh.AddLabel(ctx, owner, repo, issue.ID, "gopilot-failed"); err != nil {
		slog.Warn("failed to add failure label", "error", err)
	}

	// Comment about the failure
	comment := fmt.Sprintf("gopilot exhausted all %d retry attempts for this issue. Adding `gopilot-failed` label. "+
		"To retry, remove the label and set status back to Todo.", o.cfg.Agent.MaxRetries)
	if err := o.gh.AddComment(ctx, owner, repo, issue.ID, comment); err != nil {
		slog.Warn("failed to comment on failure", "error", err)
	}

	// Set status back to Todo
	if o.meta != nil {
		if err := o.gh.SetProjectStatus(ctx, o.meta, issue.NodeID, "Todo"); err != nil {
			slog.Warn("failed to set failed status", "error", err)
		}
	}
}

func (o *Orchestrator) reconcile() {
	for _, entry := range o.state.AllRunning() {
		select {
		case <-entry.Session.Done:
			o.state.RemoveRunning(entry.Issue)
			slog.Debug("reconciled completed run", "issue", fmt.Sprintf("%s#%d", entry.Issue.Repo, entry.Issue.ID))
		default:
			// Still running
		}
	}
}

func (o *Orchestrator) checkConfigReload() {
	info, err := os.Stat(o.cfgPath)
	if err != nil {
		return
	}

	if !info.ModTime().After(o.cfgModTime) {
		return
	}

	slog.Info("config file changed, reloading", "path", o.cfgPath)

	newCfg, err := config.Load(o.cfgPath)
	if err != nil {
		slog.Error("failed to reload config", "error", err)
		return
	}

	o.cfgModTime = info.ModTime()

	// Update safe-to-reload fields (don't change token or running state)
	o.cfg.Polling = newCfg.Polling
	o.cfg.GitHub.EligibleLabels = newCfg.GitHub.EligibleLabels
	o.cfg.GitHub.ExcludedLabels = newCfg.GitHub.ExcludedLabels
	o.cfg.GitHub.Repos = newCfg.GitHub.Repos
	o.cfg.Agent.MaxAutopilotContinues = newCfg.Agent.MaxAutopilotContinues
	o.cfg.Agent.Model = newCfg.Agent.Model
	o.cfg.Agent.TurnTimeoutMS = newCfg.Agent.TurnTimeoutMS
	o.cfg.Agent.StallTimeoutMS = newCfg.Agent.StallTimeoutMS
	o.cfg.Agent.MaxRetries = newCfg.Agent.MaxRetries
	o.cfg.Agent.MaxRetryBackoffMS = newCfg.Agent.MaxRetryBackoffMS
	o.cfg.Workspace.Hooks = newCfg.Workspace.Hooks
	o.cfg.Prompt = newCfg.Prompt

	// Update dependent components
	o.ws = workspace.NewManager(
		o.cfg.Workspace.Root,
		workspace.Hooks{
			AfterCreate:  o.cfg.Workspace.Hooks.AfterCreate,
			BeforeRun:    o.cfg.Workspace.Hooks.BeforeRun,
			AfterRun:     o.cfg.Workspace.Hooks.AfterRun,
			BeforeRemove: o.cfg.Workspace.Hooks.BeforeRemove,
		},
		o.cfg.Workspace.HookTimeoutMS,
	)
	o.stallDetector = NewStallDetector(o.cfg.Agent.StallTimeoutMS)

	slog.Info("config reloaded",
		"repos", strings.Join(o.cfg.GitHub.Repos, ", "),
		"max_concurrent", o.cfg.Polling.MaxConcurrent,
		"max_retries", o.cfg.Agent.MaxRetries,
	)
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
