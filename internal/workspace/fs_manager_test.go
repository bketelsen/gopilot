package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

func TestFSManagerPath(t *testing.T) {
	mgr := NewFSManager(config.WorkspaceConfig{Root: "/tmp/workspaces"})
	issue := domain.Issue{ID: 42, Repo: "owner/my-repo"}
	got := mgr.Path(issue)
	want := "/tmp/workspaces/my-repo/issue-42"
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestFSManagerPathSafety(t *testing.T) {
	mgr := NewFSManager(config.WorkspaceConfig{Root: "/tmp/workspaces"})
	issue := domain.Issue{ID: 42, Repo: "owner/../../../etc"}
	path := mgr.Path(issue)
	if !strings.HasPrefix(path, "/tmp/workspaces/") {
		t.Errorf("Path() = %q, escapes root", path)
	}
	if strings.Contains(path, "..") {
		t.Errorf("Path() = %q, contains traversal", path)
	}
}

func TestFSManagerEnsure(t *testing.T) {
	root := t.TempDir()
	mgr := NewFSManager(config.WorkspaceConfig{Root: root})
	issue := domain.Issue{ID: 1, Repo: "owner/repo"}

	path, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("workspace dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("workspace path is not a directory")
	}

	// Second call reuses
	path2, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	if path2 != path {
		t.Errorf("second Ensure() = %q, want %q", path2, path)
	}
}

func TestFSManagerCleanup(t *testing.T) {
	root := t.TempDir()
	mgr := NewFSManager(config.WorkspaceConfig{Root: root})
	issue := domain.Issue{ID: 1, Repo: "owner/repo"}

	path, _ := mgr.Ensure(context.Background(), issue)
	os.WriteFile(filepath.Join(path, "test.txt"), []byte("hello"), 0644)

	err := mgr.Cleanup(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("workspace not cleaned up")
	}
}

func TestExpandHookVarsBranchOverride(t *testing.T) {
	issue := domain.Issue{ID: 42, Repo: "owner/repo", Branch: "feature/my-pr-branch"}
	got := expandHookVars("checkout {{branch}}", issue, "/tmp/ws")
	want := "checkout feature/my-pr-branch"
	if got != want {
		t.Errorf("expandHookVars() = %q, want %q", got, want)
	}
}

func TestExpandHookVarsBranchDefault(t *testing.T) {
	issue := domain.Issue{ID: 42, Repo: "owner/repo"}
	got := expandHookVars("checkout {{branch}}", issue, "/tmp/ws")
	want := "checkout gopilot/issue-42"
	if got != want {
		t.Errorf("expandHookVars() = %q, want %q", got, want)
	}
}

func TestRunHookBeforePRFix(t *testing.T) {
	root := t.TempDir()
	cfg := config.WorkspaceConfig{
		Root:          root,
		HookTimeoutMS: 5000,
		Hooks: config.HooksConfig{
			BeforeRun:   "echo before_run > hook_output.txt",
			BeforePRFix: "echo branch={{branch}} > hook_output.txt",
		},
	}
	mgr := NewFSManager(cfg)
	issue := domain.Issue{ID: 99, Repo: "myorg/myrepo", Branch: "gopilot/issue-11"}

	path, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}

	err = mgr.RunHook(context.Background(), "before_pr_fix", path, issue)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(path, "hook_output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	want := "branch=gopilot/issue-11"
	if got != want {
		t.Errorf("hook output = %q, want %q", got, want)
	}
}

func TestRunHookBeforePRFixFallsBackToBeforeRun(t *testing.T) {
	root := t.TempDir()
	cfg := config.WorkspaceConfig{
		Root:          root,
		HookTimeoutMS: 5000,
		Hooks: config.HooksConfig{
			BeforeRun: "echo before_run > hook_output.txt",
			// BeforePRFix is empty — should fall back to BeforeRun
		},
	}
	mgr := NewFSManager(cfg)
	issue := domain.Issue{ID: 99, Repo: "myorg/myrepo", Branch: "gopilot/issue-11"}

	path, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}

	err = mgr.RunHook(context.Background(), "before_pr_fix", path, issue)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(path, "hook_output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	want := "before_run"
	if got != want {
		t.Errorf("hook output = %q, want %q (should fall back to before_run)", got, want)
	}
}

func TestFSManagerHookExpansion(t *testing.T) {
	root := t.TempDir()
	cfg := config.WorkspaceConfig{
		Root:          root,
		HookTimeoutMS: 5000,
		Hooks: config.HooksConfig{
			AfterCreate: `echo "repo={{repo}} issue={{issue_id}}" > hook_output.txt`,
		},
	}
	mgr := NewFSManager(cfg)
	issue := domain.Issue{ID: 99, Repo: "myorg/myrepo"}

	path, err := mgr.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(path, "hook_output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	want := "repo=myorg/myrepo issue=99"
	if got != want {
		t.Errorf("hook output = %q, want %q", got, want)
	}
}
