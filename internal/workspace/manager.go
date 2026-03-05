package workspace

import (
	"context"

	"github.com/bketelsen/gopilot/internal/domain"
)

// Manager handles per-issue workspace lifecycle.
type Manager interface {
	Ensure(ctx context.Context, issue domain.Issue) (string, error)
	RunHook(ctx context.Context, hook string, workspacePath string, issue domain.Issue) error
	Cleanup(ctx context.Context, issue domain.Issue) error
	Path(issue domain.Issue) string
}
