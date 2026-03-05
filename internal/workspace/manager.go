package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Manager handles workspace lifecycle: creation, preparation, cleanup.
type Manager struct {
	root          string
	hooks         Hooks
	hookTimeoutMS int
}

type Hooks struct {
	AfterCreate  string
	BeforeRun    string
	AfterRun     string
	BeforeRemove string
}

func NewManager(root string, hooks Hooks, hookTimeoutMS int) *Manager {
	return &Manager{
		root:          root,
		hooks:         hooks,
		hookTimeoutMS: hookTimeoutMS,
	}
}

// WorkspacePath returns the deterministic path for a repo+issue workspace.
func (m *Manager) WorkspacePath(repo string, issueID int) string {
	safe := sanitizeRepo(repo)
	return filepath.Join(m.root, safe, fmt.Sprintf("issue-%d", issueID))
}

// Ensure creates the workspace directory if it doesn't exist.
// Runs the after_create hook on first creation.
func (m *Manager) Ensure(ctx context.Context, repo string, issueID int) (string, error) {
	path := m.WorkspacePath(repo, issueID)

	// Validate path is under root
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	absRoot, err := filepath.Abs(m.root)
	if err != nil {
		return "", fmt.Errorf("resolve root path: %w", err)
	}
	if !strings.HasPrefix(absPath, absRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("workspace path %q escapes root %q", absPath, absRoot)
	}

	created := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", fmt.Errorf("create workspace: %w", err)
		}
		created = true
		slog.Info("created workspace", "path", path, "repo", repo, "issue", issueID)
	}

	if created && m.hooks.AfterCreate != "" {
		if err := m.runHook(ctx, m.hooks.AfterCreate, path, repo, issueID); err != nil {
			slog.Error("after_create hook failed", "error", err, "path", path)
			return path, fmt.Errorf("after_create hook: %w", err)
		}
	}

	return path, nil
}

// PrepareForRun runs the before_run hook in the workspace.
func (m *Manager) PrepareForRun(ctx context.Context, repo string, issueID int) error {
	if m.hooks.BeforeRun == "" {
		return nil
	}
	path := m.WorkspacePath(repo, issueID)
	return m.runHook(ctx, m.hooks.BeforeRun, path, repo, issueID)
}

// FinishRun runs the after_run hook in the workspace.
func (m *Manager) FinishRun(ctx context.Context, repo string, issueID int) error {
	if m.hooks.AfterRun == "" {
		return nil
	}
	path := m.WorkspacePath(repo, issueID)
	return m.runHook(ctx, m.hooks.AfterRun, path, repo, issueID)
}

// Cleanup runs the before_remove hook and removes the workspace.
func (m *Manager) Cleanup(ctx context.Context, repo string, issueID int) error {
	path := m.WorkspacePath(repo, issueID)

	if m.hooks.BeforeRemove != "" {
		if err := m.runHook(ctx, m.hooks.BeforeRemove, path, repo, issueID); err != nil {
			slog.Warn("before_remove hook failed", "error", err, "path", path)
		}
	}

	return os.RemoveAll(path)
}

func (m *Manager) runHook(ctx context.Context, script, workDir, repo string, issueID int) error {
	timeout := time.Duration(m.hookTimeoutMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Expand template variables in the script
	expanded := expandHookVars(script, repo, issueID, workDir)

	cmd := exec.CommandContext(ctx, "bash", "-c", expanded)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOPILOT_REPO=%s", repo),
		fmt.Sprintf("GOPILOT_ISSUE_ID=%d", issueID),
		fmt.Sprintf("GOPILOT_WORKSPACE=%s", workDir),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hook failed: %w\noutput: %s", err, string(output))
	}

	slog.Debug("hook completed", "script", script[:min(len(script), 50)], "dir", workDir)
	return nil
}

func expandHookVars(script, repo string, issueID int, workDir string) string {
	parts := strings.SplitN(repo, "/", 2)
	repoName := repo
	if len(parts) == 2 {
		repoName = parts[1]
	}

	r := strings.NewReplacer(
		"{{repo}}", repo,
		"{{repo_name}}", repoName,
		"{{issue_id}}", fmt.Sprintf("%d", issueID),
		"{{workspace}}", workDir,
		"{{branch}}", fmt.Sprintf("gopilot/issue-%d", issueID),
	)
	return r.Replace(script)
}

func sanitizeRepo(repo string) string {
	// Replace slashes and any path-dangerous characters
	safe := strings.ReplaceAll(repo, "/", "_")
	safe = strings.ReplaceAll(safe, "..", "")
	safe = strings.ReplaceAll(safe, "~", "")
	return safe
}
