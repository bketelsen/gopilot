package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// CopilotRunner implements Runner for GitHub Copilot CLI.
type CopilotRunner struct {
	Command string
	Token   string
}

// Name returns "copilot".
func (r *CopilotRunner) Name() string {
	return "copilot"
}

// Start launches a GitHub Copilot CLI subprocess in the given workspace.
func (r *CopilotRunner) Start(ctx context.Context, workspace string, prompt string, opts AgentOpts) (*Session, error) {
	args := r.buildArgs(prompt, workspace, opts)

	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, r.Command, args...)
	cmd.Dir = workspace
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = os.Stderr
	}
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Env = append(os.Environ(),
		"GITHUB_TOKEN="+r.Token,
		"COPILOT_GITHUB_TOKEN="+r.Token,
		"GH_TOKEN="+r.Token,
	)
	for _, e := range opts.Env {
		cmd.Env = append(cmd.Env, e)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting agent: %w", err)
	}

	done := make(chan struct{})
	sess := &Session{
		ID:     fmt.Sprintf("sess-%d-%d", cmd.Process.Pid, time.Now().Unix()),
		PID:    cmd.Process.Pid,
		Cancel: cancel,
		Done:   done,
	}

	go func() {
		defer close(done)
		err := cmd.Wait()
		sess.ExitErr = err
		if cmd.ProcessState != nil {
			sess.ExitCode = cmd.ProcessState.ExitCode()
		}
	}()

	return sess, nil
}

// Stop terminates a running Copilot session with SIGTERM, then SIGKILL.
func (r *CopilotRunner) Stop(sess *Session) error {
	if sess.Cancel != nil {
		sess.Cancel()
	}

	proc, err := os.FindProcess(sess.PID)
	if err != nil {
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		slog.Debug("SIGTERM failed, process may already be gone", "pid", sess.PID, "error", err)
		return nil
	}

	select {
	case <-sess.Done:
		return nil
	case <-time.After(10 * time.Second):
		slog.Warn("agent did not exit after SIGTERM, sending SIGKILL", "pid", sess.PID)
		proc.Signal(syscall.SIGKILL)
		<-sess.Done
		return nil
	}
}

func (r *CopilotRunner) buildArgs(prompt string, workspace string, opts AgentOpts) []string {
	sharePath := ".gopilot-session.md"
	args := []string{
		"-p", prompt,
		"--no-ask-user",
		"--autopilot",
		"--share", sharePath,
		"-s",
	}
	if opts.ReadOnly {
		args = append(args, "--allow-all", "--deny-tool", "write", "--deny-tool", "shell")
	} else {
		args = append(args, "--allow-all")
	}
	if opts.MaxContinuations > 0 {
		args = append(args, "--max-autopilot-continues", strconv.Itoa(opts.MaxContinuations))
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return args
}

var _ Runner = (*CopilotRunner)(nil)
