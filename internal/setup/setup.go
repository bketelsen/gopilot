package setup

import (
	"context"
	"fmt"
	"strings"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/github"
)

// LabelClient is the subset of github.RESTClient needed for label operations.
type LabelClient interface {
	GetRepoLabel(ctx context.Context, repo, name string) (*github.RepoLabel, error)
	CreateRepoLabel(ctx context.Context, repo, name, color, description string) error
	UpdateRepoLabel(ctx context.Context, repo, name, color, description string) error
}

// LabelAction describes what happened for a single label.
type LabelAction struct {
	Name   string
	Action string // "created", "updated", "ok"
}

// RepoResult collects the actions taken for one repository.
type RepoResult struct {
	Repo    string
	Actions []LabelAction
}

// EnsureLabels ensures all required labels exist on each configured repo.
func EnsureLabels(ctx context.Context, cfg *config.Config, client LabelClient) ([]RepoResult, error) {
	labels := RequiredLabels(cfg)
	var results []RepoResult

	for _, repo := range cfg.GitHub.Repos {
		result := RepoResult{Repo: repo}
		for _, def := range labels {
			existing, err := client.GetRepoLabel(ctx, repo, def.Name)
			if err != nil {
				return nil, fmt.Errorf("%s: getting label %q: %w", repo, def.Name, err)
			}

			if existing == nil {
				if err := client.CreateRepoLabel(ctx, repo, def.Name, def.Color, def.Description); err != nil {
					return nil, fmt.Errorf("%s: creating label %q: %w", repo, def.Name, err)
				}
				result.Actions = append(result.Actions, LabelAction{Name: def.Name, Action: "created"})
			} else if existing.Color != def.Color || existing.Description != def.Description {
				if err := client.UpdateRepoLabel(ctx, repo, def.Name, def.Color, def.Description); err != nil {
					return nil, fmt.Errorf("%s: updating label %q: %w", repo, def.Name, err)
				}
				result.Actions = append(result.Actions, LabelAction{Name: def.Name, Action: "updated"})
			} else {
				result.Actions = append(result.Actions, LabelAction{Name: def.Name, Action: "ok"})
			}
		}
		results = append(results, result)
	}

	return results, nil
}

// FormatResults returns a human-readable summary of setup results.
func FormatResults(results []RepoResult) string {
	var b strings.Builder
	for _, r := range results {
		b.WriteString(r.Repo)
		b.WriteString(": ")
		for i, a := range r.Actions {
			if i > 0 {
				b.WriteString(", ")
			}
			switch a.Action {
			case "ok":
				b.WriteString(a.Name)
				b.WriteString(" (ok)")
			default:
				b.WriteString(a.Action)
				b.WriteByte(' ')
				b.WriteString(a.Name)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
