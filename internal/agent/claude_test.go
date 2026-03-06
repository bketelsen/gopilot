package agent

import (
	"strings"
	"testing"
)

func TestClaudeBuildArgs(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	args := runner.buildArgs("/tmp/ws/.gopilot-prompt.md", AgentOpts{})

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

func TestClaudeBuildArgsReadOnly(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	args := runner.buildArgs("/tmp/ws/.gopilot-prompt.md", AgentOpts{ReadOnly: true})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--dangerously-skip-permissions") {
		t.Error("read-only should still have --dangerously-skip-permissions")
	}
	if !strings.Contains(joined, "--disallowedTools") {
		t.Error("read-only should have --disallowedTools")
	}
	for _, tool := range []string{"Write", "Edit", "Bash", "EnterPlanMode"} {
		if !strings.Contains(joined, tool) {
			t.Errorf("read-only should disallow %s", tool)
		}
	}
}

func TestClaudeBuildArgsStreamJSON(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	args := runner.buildArgs("/tmp/ws/.gopilot-prompt.md", AgentOpts{})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--output-format stream-json") {
		t.Error("missing --output-format stream-json flag")
	}
	if !strings.Contains(joined, "--verbose") {
		t.Error("missing --verbose flag")
	}
}

func TestClaudeName(t *testing.T) {
	runner := &ClaudeRunner{Command: "claude"}
	if runner.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", runner.Name(), "claude")
	}
}
