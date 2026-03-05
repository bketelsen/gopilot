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

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// FSManager implements the Manager interface using the local filesystem.
type FSManager struct {
	cfg config.WorkspaceConfig
}

// NewFSManager creates a new FSManager with the given workspace configuration.
func NewFSManager(cfg config.WorkspaceConfig) *FSManager {
	return &FSManager{cfg: cfg}
}

// Path returns the filesystem path for the given issue's workspace.
func (m *FSManager) Path(issue domain.Issue) string {
	repoName := sanitizePath(repoShortName(issue.Repo))
	dirName := fmt.Sprintf("issue-%d", issue.ID)
	return filepath.Join(m.cfg.Root, repoName, dirName)
}

// Ensure creates the workspace directory if it does not already exist,
// running the after_create hook on first creation.
func (m *FSManager) Ensure(ctx context.Context, issue domain.Issue) (string, error) {
	path := m.Path(issue)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path, nil
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("creating workspace: %w", err)
	}
	if m.cfg.Hooks.AfterCreate != "" {
		if err := m.runHook(ctx, m.cfg.Hooks.AfterCreate, path, issue); err != nil {
			os.RemoveAll(path)
			return "", fmt.Errorf("after_create hook: %w", err)
		}
	}
	return path, nil
}

// RunHook executes a named hook (before_run, after_run, before_remove) in the
// given workspace directory.
func (m *FSManager) RunHook(ctx context.Context, hook string, workspacePath string, issue domain.Issue) error {
	var script string
	switch hook {
	case "before_run":
		script = m.cfg.Hooks.BeforeRun
	case "after_run":
		script = m.cfg.Hooks.AfterRun
	case "before_remove":
		script = m.cfg.Hooks.BeforeRemove
	default:
		return fmt.Errorf("unknown hook: %s", hook)
	}
	if script == "" {
		return nil
	}
	return m.runHook(ctx, script, workspacePath, issue)
}

// Cleanup removes the workspace directory, running the before_remove hook first.
func (m *FSManager) Cleanup(ctx context.Context, issue domain.Issue) error {
	path := m.Path(issue)
	if m.cfg.Hooks.BeforeRemove != "" {
		if err := m.runHook(ctx, m.cfg.Hooks.BeforeRemove, path, issue); err != nil {
			slog.Warn("before_remove hook failed", "error", err, "path", path)
		}
	}
	return os.RemoveAll(path)
}

func (m *FSManager) runHook(ctx context.Context, script string, dir string, issue domain.Issue) error {
	expanded := expandHookVars(script, issue, dir)
	timeout := time.Duration(m.cfg.HookTimeoutMS) * time.Millisecond
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", expanded)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func expandHookVars(script string, issue domain.Issue, workspace string) string {
	r := strings.NewReplacer(
		"{{repo}}", issue.Repo,
		"{{issue_id}}", fmt.Sprintf("%d", issue.ID),
		"{{branch}}", fmt.Sprintf("gopilot/issue-%d", issue.ID),
		"{{workspace}}", workspace,
	)
	return r.Replace(script)
}

func repoShortName(repo string) string {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return repo
}

func sanitizePath(s string) string {
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	return s
}

// Verify FSManager implements Manager interface.
var _ Manager = (*FSManager)(nil)
