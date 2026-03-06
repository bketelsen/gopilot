package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// ClaudeRunner launches Claude Code CLI as an agent subprocess.
type ClaudeRunner struct {
	Command string
	Token   string
}

// Name returns "claude".
func (r *ClaudeRunner) Name() string { return "claude" }

// Start launches a Claude Code subprocess in the given workspace.
func (r *ClaudeRunner) Start(ctx context.Context, workspace string, prompt string, opts AgentOpts) (*Session, error) {
	promptPath := filepath.Join(workspace, ".gopilot-prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0644); err != nil {
		return nil, fmt.Errorf("writing prompt file: %w", err)
	}

	args := r.buildArgs(promptPath, opts)
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
		"GH_TOKEN="+r.Token,
	)
	cmd.Env = append(cmd.Env, opts.Env...)

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting claude: %w", err)
	}

	done := make(chan struct{})
	sess := &Session{
		ID:     fmt.Sprintf("claude-%d-%d", cmd.Process.Pid, time.Now().Unix()),
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

// Stop terminates a running Claude session with SIGTERM, then SIGKILL.
func (r *ClaudeRunner) Stop(sess *Session) error {
	if sess.Cancel != nil {
		sess.Cancel()
	}
	proc, err := os.FindProcess(sess.PID)
	if err != nil {
		return nil
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		slog.Debug("SIGTERM failed", "pid", sess.PID, "error", err)
		return nil
	}
	select {
	case <-sess.Done:
		return nil
	case <-time.After(10 * time.Second):
		proc.Signal(syscall.SIGKILL) //nolint:errcheck // best-effort kill after SIGTERM timeout
		<-sess.Done
		return nil
	}
}

func (r *ClaudeRunner) buildArgs(promptPath string, opts AgentOpts) []string {
	args := []string{
		"--print", promptPath,
		"--output-format", "stream-json",
		"--verbose",
	}
	if opts.ReadOnly {
		args = append(args, "--permission-mode", "plan")
	} else {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
}

var _ Runner = (*ClaudeRunner)(nil)
