package workspace

import (
	"testing"
)

func TestWorkspacePathDeterminism(t *testing.T) {
	m := NewManager("/tmp/ws", Hooks{}, 60000)

	path1 := m.WorkspacePath("owner/repo", 42)
	path2 := m.WorkspacePath("owner/repo", 42)

	if path1 != path2 {
		t.Errorf("paths differ: %q vs %q", path1, path2)
	}

	if path1 != "/tmp/ws/owner_repo/issue-42" {
		t.Errorf("unexpected path: %q", path1)
	}
}

func TestSanitizeRepo(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"owner/repo", "owner_repo"},
		{"../evil", "_evil"},
		{"~root", "root"},
		{"normal-repo/name", "normal-repo_name"},
	}

	for _, tt := range tests {
		got := sanitizeRepo(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeRepo(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExpandHookVars(t *testing.T) {
	script := "git clone {{repo}} && cd {{workspace}} && git checkout -b {{branch}}"
	expanded := expandHookVars(script, "owner/repo", 42, "/tmp/ws/owner_repo/issue-42")

	want := "git clone owner/repo && cd /tmp/ws/owner_repo/issue-42 && git checkout -b gopilot/issue-42"
	if expanded != want {
		t.Errorf("expandHookVars() = %q\nwant %q", expanded, want)
	}
}
