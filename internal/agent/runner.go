package agent

import "context"

// Session represents a running agent subprocess.
type Session struct {
	ID       string
	PID      int
	Cancel   context.CancelFunc
	Done     <-chan struct{} // closed when process exits
	ExitCode int
	ExitErr  error
}

// AgentOpts configures an agent launch.
type AgentOpts struct {
	Model            string
	MaxContinuations int
	Env              []string
}

// Runner launches and manages agent subprocesses.
type Runner interface {
	Start(ctx context.Context, workspace string, prompt string, opts AgentOpts) (*Session, error)
	Stop(session *Session) error
	Name() string
}
