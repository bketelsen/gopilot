package orchestrator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
	gh "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/metrics"
	"github.com/bketelsen/gopilot/internal/planning"
	"github.com/bketelsen/gopilot/internal/prompt"
	"github.com/bketelsen/gopilot/internal/skills"
	"github.com/bketelsen/gopilot/internal/web"
	"github.com/bketelsen/gopilot/internal/workspace"
)

// Orchestrator runs the poll-dispatch-reconcile loop.
type Orchestrator struct {
	cfg        *config.Config
	github     gh.Client
	agents     map[string]agent.Runner
	workspace  workspace.Manager
	state        *State
	retryQueue   *RetryQueue
	sessionsMu   sync.Mutex
	sessions     map[int]*agent.Session
	prSessions   map[int]*agent.Session // PR number -> active fix session
	configPath   string
	skills       []*skills.Skill
	sseHub       *web.SSEHub
	metrics      *metrics.Counters
	tokenTracker *metrics.TokenTracker
	rateLimitFn  func() (remaining, limit int)
	lastPRPoll   time.Time
}

// NewOrchestrator creates a new orchestrator.
func NewOrchestrator(cfg *config.Config, github gh.Client, agents map[string]agent.Runner, configPath ...string) *Orchestrator {
	o := &Orchestrator{
		cfg:        cfg,
		github:     github,
		agents:     agents,
		workspace:    workspace.NewFSManager(cfg.Workspace),
		state:        NewState(),
		retryQueue:   NewRetryQueue(),
		sessions:     make(map[int]*agent.Session),
		prSessions:   make(map[int]*agent.Session),
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
		watcher, err := config.Watch(ctx, o.configPath, func(newCfg *config.Config, loadErr error) {
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
			o.cfg.Planning = newCfg.Planning
			o.cfg.PRMonitoring = newCfg.PRMonitoring
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
		webSrv := web.NewServer(o.state, o.cfg, o.metrics, o.retryQueue, &StatePlanningAdapter{State: o.state})
		webSrv.SetSprintProvider(o.github)
		webSrv.SetSkills(o.skills)
		o.sseHub = webSrv.SSEHub()
		webSrv.SetRefreshFunc(func() {
			go o.Tick(ctx)
		})

		planningMgr := planning.NewManager()
		webSrv.SetPlanningManager(planningMgr, o.agents[o.cfg.Agent.Command], planning.HandlerConfig{
			WorkspaceRoot: o.cfg.Workspace.Root,
			GitHubClient:  o.github,
		})
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
		return fmt.Errorf("dry run: fetch candidates: %w", err)
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
	o.monitorPRs(ctx)

	// Process due retries before dispatching new candidates.
	for _, retry := range o.retryQueue.DueEntries() {
		if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
			// Re-enqueue if no slots available.
			maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
			o.retryQueue.Enqueue(retry.IssueID, retry.Repo, retry.Identifier, retry.Attempt, retry.Error, maxBackoff)
			continue
		}
		slog.Info("retrying issue", "issue", retry.Identifier, "attempt", retry.Attempt)
		// Fetch fresh issue state for retry.
		issue, err := o.github.FetchIssueState(ctx, retry.Repo, retry.IssueID)
		if err != nil {
			if errors.Is(err, gh.ErrNotFound) {
				slog.Info("retry: issue not found, dropping retry", "issue_id", retry.IssueID)
			} else if errors.Is(err, gh.ErrRateLimited) {
				slog.Warn("retry: rate limited, re-enqueuing", "issue_id", retry.IssueID)
				maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
				o.retryQueue.Enqueue(retry.IssueID, retry.Repo, retry.Identifier, retry.Attempt, retry.Error, maxBackoff)
			} else {
				slog.Warn("retry: could not fetch issue state", "issue_id", retry.IssueID, "error", err)
			}
			continue
		}
		if issue == nil {
			slog.Warn("retry: issue state returned nil", "issue_id", retry.IssueID)
			continue
		}
		if !issue.IsEligible(o.cfg.GitHub.EligibleLabels, o.cfg.GitHub.ExcludedLabels) {
			slog.Info("retry: issue no longer eligible, releasing", "issue", retry.Identifier)
			o.state.Release(issue.ID)
			continue
		}
		o.state.Release(issue.ID) // Release claim so dispatch can re-claim
		// Planning issues use the dashboard now; skip retry dispatch.
		if o.state.IsPlanning(issue.ID) {
			continue
		}
		o.dispatch(ctx, *issue, retry.Attempt)
	}

	issues, err := o.github.FetchCandidateIssues(ctx)
	if err != nil {
		if errors.Is(err, gh.ErrRateLimited) {
			slog.Warn("rate limited while fetching candidates, will retry next tick", "error", err)
		} else if errors.Is(err, gh.ErrUnauthorized) {
			slog.Error("unauthorized — check GitHub token", "error", err)
		} else {
			slog.Error("failed to fetch candidates", "error", err)
		}
		return
	}

	// Parse BlockedBy from body text
	for i := range issues {
		if len(issues[i].BlockedBy) == 0 {
			issues[i].BlockedBy = domain.ParseBlockedBy(issues[i].Body)
		}
	}

	// Build resolved map for blocking check
	resolved := make(map[int]bool)
	for _, issue := range issues {
		if issue.IsTerminal() {
			resolved[issue.ID] = true
		}
	}

	var candidates []domain.Issue
	for _, issue := range issues {
		if o.state.IsCompleted(issue.ID) || o.state.IsClaimed(issue.ID) || o.state.GetRunning(issue.ID) != nil || o.state.IsInRetryQueue(issue.ID) || o.retryQueue.Has(issue.ID) {
			continue
		}
		if issue.IsBlocked(resolved) {
			slog.Debug("skipping blocked issue", "issue", issue.Identifier(), "blocked_by", issue.BlockedBy)
			continue
		}
		candidates = append(candidates, issue)
	}

	// Partition planning vs coding issues
	planningIssues, codingCandidates := partitionPlanningIssues(candidates, o.cfg.Planning.Label)

	// Handle planning issues
	o.processPlanningIssues(ctx, planningIssues)

	// Dispatch coding issues
	domain.SortByPriority(codingCandidates)
	for _, issue := range codingCandidates {
		if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
			break
		}
		o.dispatch(ctx, issue, 1)
	}

	if o.rateLimitFn != nil {
		remaining, limit := o.rateLimitFn()
		o.metrics.Set("github_rate_limit_remaining", int64(remaining))
		o.metrics.Set("github_rate_limit_limit", int64(limit))
	}
}

func (o *Orchestrator) agentForIssue(issue domain.Issue) agent.Runner {
	cmd := o.cfg.AgentCommandForIssue(issue.Repo, issue.Labels)
	if runner, ok := o.agents[cmd]; ok {
		return runner
	}
	if runner, ok := o.agents[o.cfg.Agent.Command]; ok {
		return runner
	}
	for _, runner := range o.agents {
		return runner
	}
	return nil
}

func (o *Orchestrator) stopSession(sess *agent.Session) {
	for _, runner := range o.agents {
		runner.Stop(sess) //nolint:errcheck // best-effort stop
		return
	}
}

func (o *Orchestrator) dispatch(ctx context.Context, issue domain.Issue, attempt int) {
	// Never dispatch planning-labeled issues as coding agents.
	for _, label := range issue.Labels {
		if strings.EqualFold(label, o.cfg.Planning.Label) {
			return
		}
	}
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
		if attempt < o.cfg.Agent.MaxRetries {
			maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
			o.retryQueue.Enqueue(issue.ID, issue.Repo, issue.Identifier(), attempt+1, "before_run hook: "+err.Error(), maxBackoff)
			log.Info("scheduled retry", "next_attempt", attempt+1)
		} else {
			o.handleMaxRetriesExceeded(issue, "before_run hook: "+err.Error())
		}
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

	runner := o.agentForIssue(issue)
	if runner == nil {
		log.Error("no agent runner available")
		o.state.Release(issue.ID)
		return
	}

	pr, pw := io.Pipe()
	opts := agent.AgentOpts{
		Model:            o.cfg.Agent.Model,
		MaxContinuations: o.cfg.Agent.MaxAutopilotContinues,
		Stdout:           pw,
	}
	sess, err := runner.Start(ctx, wsPath, rendered, opts)
	if err != nil {
		log.Error("agent start failed", "error", err)
		pw.Close()
		o.state.Release(issue.ID)
		return
	}

	o.sessionsMu.Lock()
	o.sessions[issue.ID] = sess
	o.sessionsMu.Unlock()
	o.metrics.Increment("issues_dispatched")

	now := time.Now()
	entry := &domain.RunEntry{
		Issue:        issue,
		SessionID:    sess.ID,
		ProcessPID:   sess.PID,
		StartedAt:    now,
		LastEventAt:  now,
		Attempt:      attempt,
		OutputBuffer: domain.NewRingBuffer(50),
	}
	o.state.AddRunning(issue.ID, entry)

	go o.scanOutput(pr, entry)

	log.Info("agent dispatched",
		"session_id", sess.ID,
		"pid", sess.PID,
		"workspace", wsPath,
	)

	go o.monitorAgent(issue, sess, entry, pw)
}

// scanOutput reads agent stdout line by line, updating RunEntry fields and broadcasting SSE events.
func (o *Orchestrator) scanOutput(pr *io.PipeReader, entry *domain.RunEntry) {
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		line := scanner.Text()
		entry.RecordOutput(line, time.Now())

		if o.sseHub != nil {
			eventName := fmt.Sprintf("agent-output-%d", entry.Issue.ID)
			o.sseHub.Broadcast(eventName, "<div>"+line+"</div>")
		}
	}
	pr.Close()
}

func (o *Orchestrator) monitorAgent(issue domain.Issue, sess *agent.Session, entry *domain.RunEntry, pw *io.PipeWriter) {
	<-sess.Done
	pw.Close() // Close pipe writer to unblock scanOutput

	log := slog.With("issue", issue.Identifier(), "session_id", sess.ID)

	o.state.RemoveRunning(issue.ID)
	o.sessionsMu.Lock()
	delete(o.sessions, issue.ID)
	o.sessionsMu.Unlock()

	finishedAt := time.Now()
	completedErrMsg := ""
	if sess.ExitErr != nil {
		completedErrMsg = sess.ExitErr.Error()
	}
	duration := finishedAt.Sub(entry.StartedAt)
	o.state.AddHistory(issue.ID, domain.CompletedRun{
		SessionID:  sess.ID,
		Attempt:    entry.Attempt,
		StartedAt:  entry.StartedAt,
		FinishedAt: finishedAt,
		Duration:   duration,
		ExitCode:   sess.ExitCode,
		Error:      completedErrMsg,
		Tokens:     entry.Tokens,
	})
	o.metrics.RecordDuration("session_duration", duration)

	if sess.ExitCode == 0 {
		log.Info("agent completed successfully")
		o.metrics.Increment("issues_completed")

		// Don't mark planning issues as fully completed — they need multi-turn dispatch
		if o.state.IsPlanning(issue.ID) {
			// Snapshot the latest comment ID so we only detect truly new human
			// comments on subsequent ticks — not comments the agent just posted.
			if comments, err := o.github.FetchIssueComments(context.Background(), issue.Repo, issue.ID); err == nil {
				var maxID int
				for _, c := range comments {
					if c.ID > maxID {
						maxID = c.ID
					}
				}
				o.state.UpdatePlanning(issue.ID, func(e *PlanningEntry) {
					if maxID > e.LastCommentID {
						e.LastCommentID = maxID
					}
				})
			}
		} else {
			o.state.MarkCompleted(issue.ID)
		}
		o.state.Release(issue.ID)
	} else {
		log.Warn("agent exited with error", "exit_code", sess.ExitCode, "error", sess.ExitErr)

		// Reset planning phase so retry dispatches correctly.
		if o.state.IsPlanning(issue.ID) {
			o.state.UpdatePlanning(issue.ID, func(e *PlanningEntry) {
				e.Phase = PlanningPhaseDetected
			})
		}

		errMsg := "exit code " + strconv.Itoa(sess.ExitCode)
		if sess.ExitErr != nil {
			errMsg = sess.ExitErr.Error()
		}
		if entry.Attempt < o.cfg.Agent.MaxRetries {
			maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
			o.retryQueue.Enqueue(issue.ID, issue.Repo, issue.Identifier(), entry.Attempt+1, errMsg, maxBackoff)
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
				o.stopSession(sess)
				delete(o.sessions, entry.Issue.ID)
			}
			o.sessionsMu.Unlock()

			o.state.RemoveRunning(entry.Issue.ID)

			duration := time.Since(entry.StartedAt).Round(time.Second)
			comment := fmt.Sprintf("Agent stalled after %s, retrying (attempt %d)", duration, entry.Attempt)
			if o.state.IsPlanning(entry.Issue.ID) {
				comment += "\n\n" + PlanningCommentMarker
			}
			o.github.AddComment(ctx, entry.Issue.Repo, entry.Issue.ID, comment) //nolint:errcheck // best-effort comment

			if entry.Attempt < o.cfg.Agent.MaxRetries {
				maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
				o.retryQueue.Enqueue(entry.Issue.ID, entry.Issue.Repo, entry.Issue.Identifier(), entry.Attempt+1, "stalled", maxBackoff)
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
			if errors.Is(err, gh.ErrNotFound) {
				slog.Info("reconcile: issue not found, stopping agent", "issue", entry.Issue.Identifier())
				o.stopAndCleanup(ctx, entry, true)
			} else {
				slog.Warn("reconcile: fetch failed", "issue", entry.Issue.Identifier(), "error", err)
			}
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
		o.stopSession(sess)
		delete(o.sessions, entry.Issue.ID)
	}
	o.sessionsMu.Unlock()

	o.state.RemoveRunning(entry.Issue.ID)
	o.state.Release(entry.Issue.ID)

	o.workspace.RunHook(ctx, "after_run", o.workspace.Path(entry.Issue), entry.Issue) //nolint:errcheck // best-effort hook

	if removeWorkspace {
		o.workspace.Cleanup(ctx, entry.Issue) //nolint:errcheck // best-effort cleanup
	}
}

func (o *Orchestrator) handleMaxRetriesExceeded(issue domain.Issue, lastError string) {
	log := slog.With("issue", issue.Identifier())
	log.Error("max retries exceeded", "attempts", o.cfg.Agent.MaxRetries, "last_error", lastError)

	o.metrics.Increment("issues_failed")
	o.state.Release(issue.ID)

	if err := o.github.SetProjectStatus(context.Background(), issue, "Todo"); err != nil {
		log.Warn("failed to reset status to Todo", "error", err)
	}

	comment := fmt.Sprintf("Gopilot failed after %d attempts. Last error: %s", o.cfg.Agent.MaxRetries, lastError)
	if o.state.IsPlanning(issue.ID) {
		comment += "\n\n" + PlanningCommentMarker
	}
	o.github.AddComment(context.Background(), issue.Repo, issue.ID, comment) //nolint:errcheck // best-effort comment
	o.github.AddLabel(context.Background(), issue.Repo, issue.ID, "gopilot-failed") //nolint:errcheck // best-effort label
}

// SetRateLimitFunc sets a function to query GitHub API rate limit.
func (o *Orchestrator) SetRateLimitFunc(fn func() (remaining, limit int)) {
	o.rateLimitFn = fn
}

func (o *Orchestrator) shutdown() {
	for _, entry := range o.state.AllRunning() {
		slog.Info("stopping agent", "issue", entry.Issue.Identifier())
	}
}

// monitorPRs checks labeled PRs for failed CI checks and queues fix dispatches.
func (o *Orchestrator) monitorPRs(ctx context.Context) {
	if !o.cfg.PRMonitoring.Enabled {
		return
	}

	// Respect the separate PR poll interval.
	if time.Since(o.lastPRPoll) < o.cfg.PRMonitoring.PollInterval() {
		return
	}
	o.lastPRPoll = time.Now()

	label := o.cfg.PRMonitoring.Label
	prs, err := o.github.FetchMonitoredPRs(ctx, label)
	if err != nil {
		if errors.Is(err, gh.ErrRateLimited) {
			slog.Warn("rate limited while fetching monitored PRs", "error", err)
		} else {
			slog.Error("failed to fetch monitored PRs", "error", err)
		}
		return
	}

	for i, pr := range prs {
		if o.state.IsPRBeingFixed(pr.Number) {
			continue
		}

		checkRuns, err := o.github.FetchCheckRuns(ctx, pr.Repo, pr.HeadSHA)
		if err != nil {
			slog.Warn("failed to fetch check runs for PR", "pr", pr.Identifier(), "error", err)
			continue
		}
		prs[i].CheckRuns = checkRuns

		// Only act on PRs where all checks have completed.
		if !prs[i].ChecksComplete() {
			continue
		}

		if !prs[i].HasFailedChecks() {
			slog.Debug("PR checks all passing", "pr", pr.Identifier())
			continue
		}

		failed := prs[i].FailedCheckRuns()
		slog.Info("PR has failed checks, queuing fix",
			"pr", pr.Identifier(),
			"failed_checks", len(failed),
		)

		// Fetch failure logs for each failed check.
		for j := range failed {
			logOutput, err := o.github.FetchCheckRunLog(ctx, pr.Repo, failed[j].ID)
			if err != nil {
				slog.Warn("failed to fetch check run log", "check", failed[j].Name, "error", err)
				continue
			}
			if len(logOutput) > o.cfg.PRMonitoring.LogTruncateLen {
				logOutput = logOutput[:o.cfg.PRMonitoring.LogTruncateLen] + "\n... (truncated)"
			}
			failed[j].Output = logOutput
		}

		prCopy := prs[i]
		prCopy.CheckRuns = failed
		o.state.AddPRFix(pr.Number, &domain.PRFixEntry{
			PR:          prCopy,
			Attempt:     1,
			MaxAttempts: o.cfg.PRMonitoring.MaxFixAttempts,
			NextRetryAt: time.Now(),
		})
	}

	// Dispatch due PR fixes.
	for _, fix := range o.state.AllPRFixes() {
		if time.Now().Before(fix.NextRetryAt) {
			continue
		}
		if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
			break
		}
		o.state.RemovePRFix(fix.PR.Number)
		o.dispatchPRFix(ctx, fix)
	}
}

// dispatchPRFix dispatches an agent to fix a PR with failing CI checks.
func (o *Orchestrator) dispatchPRFix(ctx context.Context, fix *domain.PRFixEntry) {
	log := slog.With("pr", fix.PR.Identifier(), "attempt", fix.Attempt)

	// Create a synthetic issue to reuse workspace infrastructure.
	syntheticIssue := domain.Issue{
		ID:    fix.PR.Number + 1000000, // Offset to avoid collision with real issues
		Repo:  fix.PR.Repo,
		Title: fmt.Sprintf("PR Fix: %s", fix.PR.Title),
		URL:   fix.PR.URL,
	}

	wsPath, err := o.workspace.Ensure(ctx, syntheticIssue)
	if err != nil {
		log.Error("workspace ensure failed for PR fix", "error", err)
		return
	}

	if err := o.workspace.RunHook(ctx, "before_run", wsPath, syntheticIssue); err != nil {
		log.Error("before_run hook failed for PR fix", "error", err)
		return
	}

	skillText := skills.InjectSkills(o.skills, o.cfg.Skills.Required, o.cfg.Skills.Optional)
	rendered, err := prompt.RenderPRFix(fix.PR, fix.PR.FailedCheckRuns(), fix.Attempt, skillText)
	if err != nil {
		log.Error("PR fix prompt render failed", "error", err)
		return
	}

	// Use the default agent runner.
	runner := o.agents[o.cfg.Agent.Command]
	if runner == nil {
		for _, r := range o.agents {
			runner = r
			break
		}
	}
	if runner == nil {
		log.Error("no agent runner available for PR fix")
		return
	}

	opts := agent.AgentOpts{
		Model:            o.cfg.Agent.Model,
		MaxContinuations: o.cfg.Agent.MaxAutopilotContinues,
	}
	sess, err := runner.Start(ctx, wsPath, rendered, opts)
	if err != nil {
		log.Error("agent start failed for PR fix", "error", err)
		return
	}

	o.sessionsMu.Lock()
	o.prSessions[fix.PR.Number] = sess
	o.sessionsMu.Unlock()
	o.metrics.Increment("pr_fixes_dispatched")

	now := time.Now()
	entry := &domain.RunEntry{
		Issue:       syntheticIssue,
		SessionID:   sess.ID,
		ProcessPID:  sess.PID,
		StartedAt:   now,
		LastEventAt: now,
		Attempt:     fix.Attempt,
	}
	o.state.AddPRRunning(fix.PR.Number, entry)

	log.Info("PR fix agent dispatched",
		"session_id", sess.ID,
		"pid", sess.PID,
		"workspace", wsPath,
	)

	go o.monitorPRFixAgent(fix, sess, entry)
}

// monitorPRFixAgent monitors a PR fix agent session until completion.
func (o *Orchestrator) monitorPRFixAgent(fix *domain.PRFixEntry, sess *agent.Session, entry *domain.RunEntry) {
	<-sess.Done

	log := slog.With("pr", fix.PR.Identifier(), "session_id", sess.ID)

	o.state.RemovePRRunning(fix.PR.Number)
	o.sessionsMu.Lock()
	delete(o.prSessions, fix.PR.Number)
	o.sessionsMu.Unlock()

	finishedAt := time.Now()
	completedErrMsg := ""
	if sess.ExitErr != nil {
		completedErrMsg = sess.ExitErr.Error()
	}
	duration := finishedAt.Sub(entry.StartedAt)
	o.state.AddPRHistory(fix.PR.Number, domain.CompletedRun{
		SessionID:  sess.ID,
		Attempt:    fix.Attempt,
		StartedAt:  entry.StartedAt,
		FinishedAt: finishedAt,
		Duration:   duration,
		ExitCode:   sess.ExitCode,
		Error:      completedErrMsg,
		Tokens:     entry.Tokens,
	})

	if sess.ExitCode == 0 {
		log.Info("PR fix agent completed successfully")
		o.metrics.Increment("pr_fixes_completed")
		// Comment on the PR about the fix.
		comment := fmt.Sprintf("Gopilot pushed a fix for failing CI checks (attempt %d).", fix.Attempt)
		if err := o.github.AddComment(context.Background(), fix.PR.Repo, fix.PR.Number, comment); err != nil {
			log.Warn("failed to comment on PR after fix", "error", err)
		}
	} else {
		log.Warn("PR fix agent exited with error", "exit_code", sess.ExitCode, "error", sess.ExitErr)

		errMsg := "exit code " + strconv.Itoa(sess.ExitCode)
		if sess.ExitErr != nil {
			errMsg = sess.ExitErr.Error()
		}

		if fix.Attempt < fix.MaxAttempts {
			// Re-queue with backoff.
			maxBackoff := time.Duration(o.cfg.Agent.MaxRetryBackoffMS) * time.Millisecond
			backoff := time.Duration(fix.Attempt) * maxBackoff / time.Duration(fix.MaxAttempts)
			nextFix := &domain.PRFixEntry{
				PR:          fix.PR,
				Attempt:     fix.Attempt + 1,
				MaxAttempts: fix.MaxAttempts,
				NextRetryAt: time.Now().Add(backoff),
				LastError:   errMsg,
			}
			o.state.AddPRFix(fix.PR.Number, nextFix)
			log.Info("PR fix scheduled for retry", "next_attempt", fix.Attempt+1)
		} else {
			log.Error("PR fix max attempts exceeded", "attempts", fix.MaxAttempts)
			o.metrics.Increment("pr_fixes_failed")
			comment := fmt.Sprintf("Gopilot failed to fix CI checks after %d attempts. Last error: %s\n\nThis PR needs manual attention.", fix.MaxAttempts, errMsg)
			if err := o.github.AddComment(context.Background(), fix.PR.Repo, fix.PR.Number, comment); err != nil {
				log.Warn("failed to comment on PR after max retries", "error", err)
			}
			if err := o.github.AddLabel(context.Background(), fix.PR.Repo, fix.PR.Number, "needs-human"); err != nil {
				log.Warn("failed to add needs-human label to PR", "error", err)
			}
		}
	}
}
