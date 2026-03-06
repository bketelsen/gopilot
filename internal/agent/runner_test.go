package agent_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bketelsen/gopilot/internal/agent"
)

func TestClaudeRunner_StdoutCapture(t *testing.T) {
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "claude")
	os.WriteFile(script, []byte("#!/bin/bash\necho 'hello from agent'"), 0755)

	var buf bytes.Buffer
	runner := &agent.ClaudeRunner{Command: script, Token: "test"}
	sess, err := runner.Start(context.Background(), tmpDir, "test prompt", agent.AgentOpts{
		Stdout: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	<-sess.Done
	if !bytes.Contains(buf.Bytes(), []byte("hello from agent")) {
		t.Errorf("expected stdout capture, got: %s", buf.String())
	}
}

func TestClaudeRunner_StdoutDefaultsToStderr(t *testing.T) {
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "claude")
	os.WriteFile(script, []byte("#!/bin/bash\nexit 0"), 0755)

	runner := &agent.ClaudeRunner{Command: script, Token: "test"}
	sess, err := runner.Start(context.Background(), tmpDir, "test prompt", agent.AgentOpts{})
	if err != nil {
		t.Fatal(err)
	}
	<-sess.Done
	if sess.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", sess.ExitCode)
	}
}
