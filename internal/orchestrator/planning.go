package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/prompt"
	"github.com/bketelsen/gopilot/internal/skills"
)

// commentFetcher is the subset of github.Client needed for comment checking.
type commentFetcher interface {
	FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error)
}

// partitionPlanningIssues splits issues into planning and coding sets.
func partitionPlanningIssues(issues []domain.Issue, planningLabel string) (planning, coding []domain.Issue) {
	for _, issue := range issues {
		isPlan := false
		for _, label := range issue.Labels {
			if strings.EqualFold(label, planningLabel) {
				isPlan = true
				break
			}
		}
		if isPlan {
			planning = append(planning, issue)
		} else {
			coding = append(coding, issue)
		}
	}
	return
}

// hasNewHumanComment checks if there are new non-bot comments after lastCommentID.
func hasNewHumanComment(client commentFetcher, ctx context.Context, repo string, issueID, lastCommentID int) (bool, int) {
	comments, err := client.FetchIssueComments(ctx, repo, issueID)
	if err != nil {
		return false, lastCommentID
	}

	latestID := lastCommentID
	hasNew := false
	for _, c := range comments {
		if c.ID > lastCommentID && !isBot(c.Author) {
			hasNew = true
		}
		if c.ID > latestID {
			latestID = c.ID
		}
	}
	return hasNew, latestID
}

// isBot returns true if the author looks like a bot account.
func isBot(author string) bool {
	return strings.HasSuffix(author, "[bot]")
}

// processPlanningIssues handles planning-labeled issues.
func (o *Orchestrator) processPlanningIssues(ctx context.Context, issues []domain.Issue) {
	for _, issue := range issues {
		if o.state.IsPlanning(issue.ID) {
			entry := o.state.GetPlanning(issue.ID)
			if entry.Phase == PlanningPhaseAwaitingReply || entry.Phase == PlanningPhasePlanProposed || entry.Phase == PlanningPhaseAwaitingApproval {
				hasNew, lastID := hasNewHumanComment(o.github, ctx, issue.Repo, issue.ID, entry.LastCommentID)
				if hasNew {
					entry.LastCommentID = lastID
					if o.sseHub != nil {
						o.sseHub.Broadcast("planning:reply_detected", fmt.Sprintf(`{"issue_id":%d}`, issue.ID))
					}
					o.dispatchPlanningAgent(ctx, issue, entry)
				}
			}
			continue
		}

		// New planning issue
		entry := &PlanningEntry{
			IssueID: issue.ID,
			Repo:    issue.Repo,
			Phase:   PlanningPhaseDetected,
		}
		o.state.AddPlanning(issue.ID, entry)
		o.dispatchPlanningAgent(ctx, issue, entry)
	}
}

// dispatchPlanningAgent invokes the agent for one planning interaction.
func (o *Orchestrator) dispatchPlanningAgent(ctx context.Context, issue domain.Issue, entry *PlanningEntry) {
	if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
		return
	}
	if o.state.IsClaimed(issue.ID) || o.state.GetRunning(issue.ID) != nil {
		return
	}
	if !o.state.Claim(issue.ID) {
		return
	}

	log := slog.With("issue", issue.Identifier(), "planning_phase", string(entry.Phase))

	comments, err := o.github.FetchIssueComments(ctx, issue.Repo, issue.ID)
	if err != nil {
		log.Error("failed to fetch comments for planning", "error", err)
		o.state.Release(issue.ID)
		return
	}

	planningSkillText := skills.InjectSkills(o.skills, []string{"planning"}, nil)
	rendered := prompt.RenderPlanning(issue, comments, planningSkillText)

	agentCmd := o.cfg.Planning.Agent
	if agentCmd == "" {
		agentCmd = o.cfg.Agent.Command
	}
	runner, ok := o.agents[agentCmd]
	if !ok {
		runner = o.agentForIssue(issue)
	}
	if runner == nil {
		log.Error("no agent runner available for planning")
		o.state.Release(issue.ID)
		return
	}

	model := o.cfg.Planning.Model
	if model == "" {
		model = o.cfg.Agent.Model
	}

	opts := agent.AgentOpts{
		Model:            model,
		MaxContinuations: o.cfg.Agent.MaxAutopilotContinues,
	}
	sess, err := runner.Start(ctx, "", rendered, opts)
	if err != nil {
		log.Error("planning agent start failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	o.sessionsMu.Lock()
	o.sessions[issue.ID] = sess
	o.sessionsMu.Unlock()

	now := time.Now()
	runEntry := &domain.RunEntry{
		Issue:       issue,
		SessionID:   sess.ID,
		ProcessPID:  sess.PID,
		StartedAt:   now,
		LastEventAt: now,
		Attempt:     1,
	}
	o.state.AddRunning(issue.ID, runEntry)

	entry.Phase = PlanningPhaseAwaitingReply
	entry.QuestionsAsked++

	log.Info("planning agent dispatched", "session_id", sess.ID)
	if o.sseHub != nil {
		o.sseHub.Broadcast("planning:question_posted", fmt.Sprintf(`{"issue_id":%d}`, issue.ID))
	}

	go o.monitorAgent(issue, sess, runEntry)
}
