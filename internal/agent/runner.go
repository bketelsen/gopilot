package agent

import (
	"context"
	"time"
)

// AgentRunner defines the interface for a pluggable AI agent backend.
type AgentRunner interface {
	// Start launches the agent in the given workspace with the given prompt.
	// Returns a Session that can be used to monitor and stop the agent.
	Start(ctx context.Context, opts AgentOpts) (*Session, error)

	// Name returns the name of this agent backend.
	Name() string
}

// AgentOpts configures a single agent run.
type AgentOpts struct {
	Prompt       string
	WorkDir      string
	SessionFile  string // path for session transcript
	Repo         string
	IssueID      int
	MaxContinues int
	Model        string
	Timeout      time.Duration
}

// Session represents a running agent process.
type Session struct {
	ID        string
	PID       int
	StartedAt time.Time
	Done      <-chan struct{} // closed when process exits
	Err       error          // set after Done is closed
	cancel    context.CancelFunc
}

// Stop sends a termination signal to the agent process.
func (s *Session) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}
