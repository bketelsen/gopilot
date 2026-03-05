package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/workspace"
)

// OnComplete is called when an agent run finishes.
type OnComplete func(issue github.Issue, err error)

// Dispatcher handles the lifecycle of dispatching an agent to work on an issue.
type Dispatcher struct {
	gh         *github.Client
	workspace  *workspace.Manager
	agent      agent.AgentRunner
	state      *State
	meta       *github.ProjectMeta
	prompt     string
	skillsText string
	agentCfg   agentDispatchConfig
	onComplete OnComplete
}

type agentDispatchConfig struct {
	MaxContinues int
	Model        string
	TurnTimeout  time.Duration
}

func NewDispatcher(
	ghClient *github.Client,
	ws *workspace.Manager,
	runner agent.AgentRunner,
	state *State,
	meta *github.ProjectMeta,
	prompt string,
	skillsText string,
	cfg agentDispatchConfig,
	onComplete OnComplete,
) *Dispatcher {
	return &Dispatcher{
		gh:         ghClient,
		workspace:  ws,
		agent:      runner,
		state:      state,
		meta:       meta,
		prompt:     prompt,
		skillsText: skillsText,
		agentCfg:   cfg,
		onComplete: onComplete,
	}
}

// Dispatch claims an issue and launches an agent to work on it.
func (d *Dispatcher) Dispatch(ctx context.Context, issue github.Issue) error {
	log := slog.With("issue", fmt.Sprintf("%s#%d", issue.Repo, issue.ID), "title", issue.Title)

	// Claim the issue
	if !d.state.Claim(issue) {
		return fmt.Errorf("issue already claimed or running")
	}

	// Set project status to "In Progress"
	if d.meta != nil {
		if err := d.gh.SetProjectStatus(ctx, d.meta, issue.NodeID, "In Progress"); err != nil {
			d.state.ReleaseClaim(issue)
			return fmt.Errorf("set status In Progress: %w", err)
		}
		log.Info("set status to In Progress")
	}

	// Ensure workspace exists
	workDir, err := d.workspace.Ensure(ctx, issue.Repo, issue.ID)
	if err != nil {
		d.state.ReleaseClaim(issue)
		return fmt.Errorf("ensure workspace: %w", err)
	}

	// Run before_run hook
	if err := d.workspace.PrepareForRun(ctx, issue.Repo, issue.ID); err != nil {
		d.state.ReleaseClaim(issue)
		return fmt.Errorf("before_run hook: %w", err)
	}

	// Render prompt
	branch := fmt.Sprintf("gopilot/issue-%d", issue.ID)
	rendered, err := RenderPrompt(d.prompt, PromptData{
		Issue:  issue,
		Repo:   issue.Repo,
		Branch: branch,
		Skills: d.skillsText,
	})
	if err != nil {
		d.state.ReleaseClaim(issue)
		return fmt.Errorf("render prompt: %w", err)
	}

	// Launch agent
	session, err := d.agent.Start(ctx, agent.AgentOpts{
		Prompt:       rendered,
		WorkDir:      workDir,
		Repo:         issue.Repo,
		IssueID:      issue.ID,
		MaxContinues: d.agentCfg.MaxContinues,
		Model:        d.agentCfg.Model,
		Timeout:      d.agentCfg.TurnTimeout,
	})
	if err != nil {
		d.state.ReleaseClaim(issue)
		return fmt.Errorf("start agent: %w", err)
	}

	d.state.AddRunning(issue, session)
	log.Info("dispatched agent", "session_id", session.ID, "pid", session.PID)

	// Comment on the issue
	comment := fmt.Sprintf("gopilot dispatched agent `%s` (session `%s`) to work on this issue.", d.agent.Name(), session.ID)
	owner, repo := ownerRepo(issue.Repo)
	if err := d.gh.AddComment(ctx, owner, repo, issue.ID, comment); err != nil {
		log.Warn("failed to comment on issue", "error", err)
	}

	// Goroutine to wait for agent exit and clean up
	go d.waitForCompletion(issue, session, log)

	return nil
}

// StopRun terminates a running agent (e.g. for stall detection).
func (d *Dispatcher) StopRun(entry *RunEntry) {
	slog.Info("stopping stalled agent",
		"issue", fmt.Sprintf("%s#%d", entry.Issue.Repo, entry.Issue.ID),
		"session", entry.Session.ID,
	)
	entry.Session.Stop()
}

func (d *Dispatcher) waitForCompletion(issue github.Issue, session *agent.Session, log *slog.Logger) {
	<-session.Done

	log.Info("agent completed", "session_id", session.ID, "error", session.Err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Run after_run hook
	if err := d.workspace.FinishRun(ctx, issue.Repo, issue.ID); err != nil {
		log.Warn("after_run hook failed", "error", err)
	}

	// Set status to "In Review" on success, back to "Todo" on failure
	if d.meta != nil {
		status := "In Review"
		if session.Err != nil {
			status = "Todo"
		}
		if err := d.gh.SetProjectStatus(ctx, d.meta, issue.NodeID, status); err != nil {
			log.Warn("failed to set post-run status", "status", status, "error", err)
		} else {
			log.Info("set status", "status", status)
		}
	}

	d.state.RemoveRunning(issue)

	// Notify orchestrator of completion
	if d.onComplete != nil {
		d.onComplete(issue, session.Err)
	}
}

func ownerRepo(repo string) (string, string) {
	for i := range repo {
		if repo[i] == '/' {
			return repo[:i], repo[i+1:]
		}
	}
	return repo, ""
}
