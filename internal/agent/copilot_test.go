package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCopilotBuildArgs(t *testing.T) {
	runner := &CopilotRunner{
		Command: "copilot",
	}
	opts := AgentOpts{
		Model:            "claude-sonnet-4.6",
		MaxContinuations: 20,
	}
	args := runner.buildArgs("Do the work", "/tmp/ws", opts)

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-p") {
		t.Error("missing -p flag")
	}
	if !strings.Contains(joined, "--autopilot") {
		t.Error("missing --autopilot flag")
	}
	if !strings.Contains(joined, "--allow-all") {
		t.Error("missing --allow-all flag")
	}
	if !strings.Contains(joined, "--no-ask-user") {
		t.Error("missing --no-ask-user flag")
	}
	if !strings.Contains(joined, "--model claude-sonnet-4.6") {
		t.Error("missing --model flag")
	}
	if !strings.Contains(joined, "--max-autopilot-continues 20") {
		t.Error("missing --max-autopilot-continues flag")
	}
	if !strings.Contains(joined, "-s") {
		t.Error("missing -s flag")
	}
}

func TestCopilotStartStop(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-agent.sh")
	os.WriteFile(script, []byte("#!/bin/bash\nsleep 30"), 0755)

	runner := &CopilotRunner{Command: script}
	ctx := context.Background()
	sess, err := runner.Start(ctx, dir, "test prompt", AgentOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if sess.PID == 0 {
		t.Error("PID should be non-zero")
	}

	err = runner.Stop(sess)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-sess.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit after Stop")
	}
}

func TestCopilotName(t *testing.T) {
	runner := &CopilotRunner{Command: "copilot"}
	if runner.Name() != "copilot" {
		t.Errorf("Name() = %q, want %q", runner.Name(), "copilot")
	}
}
