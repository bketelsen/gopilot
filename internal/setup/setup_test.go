package setup

import (
	"context"
	"testing"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/github"
)

// mockLabelClient implements the LabelClient interface for testing.
type mockLabelClient struct {
	labels  map[string]map[string]*github.RepoLabel // repo -> name -> label
	created []string                                 // "repo:name" log
	updated []string                                 // "repo:name" log
}

func newMockClient() *mockLabelClient {
	return &mockLabelClient{
		labels: make(map[string]map[string]*github.RepoLabel),
	}
}

func (m *mockLabelClient) GetRepoLabel(_ context.Context, repo, name string) (*github.RepoLabel, error) {
	if repoLabels, ok := m.labels[repo]; ok {
		if l, ok := repoLabels[name]; ok {
			return l, nil
		}
	}
	return nil, nil
}

func (m *mockLabelClient) CreateRepoLabel(_ context.Context, repo, name, color, description string) error {
	if m.labels[repo] == nil {
		m.labels[repo] = make(map[string]*github.RepoLabel)
	}
	m.labels[repo][name] = &github.RepoLabel{Name: name, Color: color, Description: description}
	m.created = append(m.created, repo+":"+name)
	return nil
}

func (m *mockLabelClient) UpdateRepoLabel(_ context.Context, repo, name, color, description string) error {
	if m.labels[repo] == nil {
		m.labels[repo] = make(map[string]*github.RepoLabel)
	}
	m.labels[repo][name] = &github.RepoLabel{Name: name, Color: color, Description: description}
	m.updated = append(m.updated, repo+":"+name)
	return nil
}

func TestEnsureLabels_CreatesAll(t *testing.T) {
	client := newMockClient()
	cfg := &config.Config{}
	cfg.GitHub.Repos = []string{"owner/repo"}
	cfg.GitHub.EligibleLabels = []string{"gopilot"}
	cfg.ApplyDefaults()

	results, err := EnsureLabels(context.Background(), cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d repo results, want 1", len(results))
	}
	wantCount := len(RequiredLabels(cfg))
	if len(client.created) != wantCount {
		t.Errorf("created %d labels, want %d: %v", len(client.created), wantCount, client.created)
	}
	if len(client.updated) != 0 {
		t.Errorf("updated %d labels, want 0", len(client.updated))
	}
}

func TestEnsureLabels_SkipsExisting(t *testing.T) {
	client := newMockClient()
	client.labels["owner/repo"] = map[string]*github.RepoLabel{
		"gopilot": {Name: "gopilot", Color: "0052CC", Description: "Eligible for Gopilot agent dispatch"},
	}

	cfg := &config.Config{}
	cfg.GitHub.Repos = []string{"owner/repo"}
	cfg.GitHub.EligibleLabels = []string{"gopilot"}
	cfg.ApplyDefaults()

	_, err := EnsureLabels(context.Background(), cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 3 {
		t.Errorf("created %d labels, want 3 (skipped existing gopilot): %v", len(client.created), client.created)
	}
}

func TestEnsureLabels_UpdatesStaleColor(t *testing.T) {
	client := newMockClient()
	client.labels["owner/repo"] = map[string]*github.RepoLabel{
		"gopilot": {Name: "gopilot", Color: "ff0000", Description: "Eligible for Gopilot agent dispatch"},
	}

	cfg := &config.Config{}
	cfg.GitHub.Repos = []string{"owner/repo"}
	cfg.GitHub.EligibleLabels = []string{"gopilot"}
	cfg.ApplyDefaults()

	_, err := EnsureLabels(context.Background(), cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.updated) != 1 {
		t.Errorf("updated %d labels, want 1 (stale color): %v", len(client.updated), client.updated)
	}
}

func TestEnsureLabels_CustomEligibleLabel(t *testing.T) {
	client := newMockClient()
	cfg := &config.Config{}
	cfg.GitHub.Repos = []string{"owner/repo"}
	cfg.GitHub.EligibleLabels = []string{"my-bot"}
	cfg.ApplyDefaults()

	_, err := EnsureLabels(context.Background(), cfg, client)
	if err != nil {
		t.Fatal(err)
	}

	// Should create "my-bot" instead of "gopilot"
	found := false
	for _, c := range client.created {
		if c == "owner/repo:my-bot" {
			found = true
		}
		if c == "owner/repo:gopilot" {
			t.Error("should not create default 'gopilot' when custom label configured")
		}
	}
	if !found {
		t.Error("did not create custom eligible label 'my-bot'")
	}
}

func TestFormatResults(t *testing.T) {
	results := []RepoResult{
		{
			Repo: "owner/repo",
			Actions: []LabelAction{
				{Name: "gopilot", Action: "created"},
				{Name: "gopilot:plan", Action: "ok"},
				{Name: "gopilot-failed", Action: "updated"},
			},
		},
	}
	got := FormatResults(results)
	want := "owner/repo: created gopilot, gopilot:plan (ok), updated gopilot-failed\n"
	if got != want {
		t.Errorf("FormatResults =\n%q\nwant:\n%q", got, want)
	}
}
