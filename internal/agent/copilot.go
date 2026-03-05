package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CopilotRunner implements AgentRunner for GitHub Copilot CLI.
type CopilotRunner struct {
	command string // e.g. "copilot" or full path
}

func NewCopilotRunner(command string) *CopilotRunner {
	if command == "" {
		command = "copilot"
	}
	return &CopilotRunner{command: command}
}

func (r *CopilotRunner) Name() string { return "copilot-cli" }

func (r *CopilotRunner) Start(ctx context.Context, opts AgentOpts) (*Session, error) {
	sessionID := randomHex(8)

	sessionFile := opts.SessionFile
	if sessionFile == "" {
		sessionFile = filepath.Join(opts.WorkDir, ".gopilot-session.md")
	}

	args := r.buildArgs(opts, sessionFile)

	slog.Info("launching copilot agent",
		"session_id", sessionID,
		"workdir", opts.WorkDir,
		"model", opts.Model,
		"max_continues", opts.MaxContinues,
	)

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)

	cmd := exec.CommandContext(ctx, r.command, args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOPILOT_SESSION_ID=%s", sessionID),
		fmt.Sprintf("GOPILOT_REPO=%s", opts.Repo),
		fmt.Sprintf("GOPILOT_ISSUE_ID=%d", opts.IssueID),
	)

	// Capture output to a log file
	logPath := filepath.Join(opts.WorkDir, fmt.Sprintf(".gopilot-agent-%s.log", sessionID))
	logFile, err := os.Create(logPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create agent log: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		cancel()
		logFile.Close()
		return nil, fmt.Errorf("start copilot: %w", err)
	}

	done := make(chan struct{})
	session := &Session{
		ID:        sessionID,
		PID:       cmd.Process.Pid,
		StartedAt: time.Now(),
		Done:      done,
		cancel:    cancel,
	}

	go func() {
		defer close(done)
		defer logFile.Close()
		defer cancel()
		session.Err = cmd.Wait()
		slog.Info("copilot agent exited",
			"session_id", sessionID,
			"error", session.Err,
			"log", logPath,
		)
	}()

	return session, nil
}

func (r *CopilotRunner) buildArgs(opts AgentOpts, sessionFile string) []string {
	args := []string{
		"-p", opts.Prompt,
		"--allow-all",
		"--no-ask-user",
		"--autopilot",
	}

	if opts.MaxContinues > 0 {
		args = append(args, "--max-autopilot-continues", fmt.Sprintf("%d", opts.MaxContinues))
	}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	args = append(args,
		"--share", sessionFile,
		"-s", // headless/silent mode
	)

	return args
}
