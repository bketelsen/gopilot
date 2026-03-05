package agent

import (
	"strings"
	"testing"
)

func TestClaudeBuildArgs(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	args := runner.buildArgs("/tmp/ws/.gopilot-prompt.md")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--dangerously-skip-permissions") {
		t.Error("missing --dangerously-skip-permissions flag")
	}
	if !strings.Contains(joined, "--print") {
		t.Error("missing --print flag")
	}
	if !strings.Contains(joined, ".gopilot-prompt.md") {
		t.Error("missing prompt file path")
	}
}

func TestClaudeName(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	if runner.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", runner.Name(), "claude")
	}
}
