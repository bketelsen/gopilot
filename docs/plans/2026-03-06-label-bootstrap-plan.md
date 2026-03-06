# Label Bootstrap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `gopilot setup` command that provisions required GitHub labels (with canonical colors and descriptions) on all configured repositories.

**Architecture:** New `internal/setup/` package with canonical label definitions and an `EnsureLabels` function. Three new methods on the GitHub REST client (`GetRepoLabel`, `CreateRepoLabel`, `UpdateRepoLabel`) for repo-level label CRUD. CLI wires it together via a `"setup"` case in main.

**Tech Stack:** Go stdlib, `net/http/httptest` for tests, existing `internal/config` and `internal/github` packages.

---

### Task 1: Add GitHub client label methods — failing tests

**Files:**
- Modify: `internal/github/rest_test.go`

**Step 1: Write failing tests for GetRepoLabel, CreateRepoLabel, UpdateRepoLabel**

Add to `internal/github/rest_test.go`:

```go
func TestGetRepoLabel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/labels/gopilot", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"name":        "gopilot",
			"color":       "0052CC",
			"description": "Eligible for Gopilot agent dispatch",
		})
	})
	mux.HandleFunc("GET /repos/owner/repo/labels/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	// Existing label
	label, err := client.GetRepoLabel(context.Background(), "owner/repo", "gopilot")
	if err != nil {
		t.Fatal(err)
	}
	if label == nil {
		t.Fatal("expected label, got nil")
	}
	if label.Name != "gopilot" {
		t.Errorf("name = %q, want %q", label.Name, "gopilot")
	}
	if label.Color != "0052CC" {
		t.Errorf("color = %q, want %q", label.Color, "0052CC")
	}

	// Missing label
	label, err = client.GetRepoLabel(context.Background(), "owner/repo", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if label != nil {
		t.Errorf("expected nil for missing label, got %+v", label)
	}
}

func TestCreateRepoLabel(t *testing.T) {
	var received map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/labels", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	err := client.CreateRepoLabel(context.Background(), "owner/repo", "gopilot", "0052CC", "Eligible for Gopilot agent dispatch")
	if err != nil {
		t.Fatal(err)
	}
	if received["name"] != "gopilot" {
		t.Errorf("name = %v, want gopilot", received["name"])
	}
	if received["color"] != "0052CC" {
		t.Errorf("color = %v, want 0052CC", received["color"])
	}
	if received["description"] != "Eligible for Gopilot agent dispatch" {
		t.Errorf("description = %v, want correct description", received["description"])
	}
}

func TestUpdateRepoLabel(t *testing.T) {
	var received map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /repos/owner/repo/labels/gopilot", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	err := client.UpdateRepoLabel(context.Background(), "owner/repo", "gopilot", "0052CC", "Updated description")
	if err != nil {
		t.Fatal(err)
	}
	if received["color"] != "0052CC" {
		t.Errorf("color = %v, want 0052CC", received["color"])
	}
	if received["description"] != "Updated description" {
		t.Errorf("description = %v, want Updated description", received["description"])
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -run "TestGetRepoLabel|TestCreateRepoLabel|TestUpdateRepoLabel" ./internal/github/...`
Expected: FAIL — methods don't exist yet

---

### Task 2: Add GitHub client label methods — implementation

**Files:**
- Modify: `internal/github/rest.go`

**Step 1: Add the RepoLabel type and three methods to rest.go**

Add the `RepoLabel` struct and methods to `internal/github/rest.go`:

```go
// RepoLabel represents a GitHub repository label.
type RepoLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// GetRepoLabel fetches a repository-level label by name.
// Returns nil, nil if the label does not exist (404).
func (c *RESTClient) GetRepoLabel(ctx context.Context, repo string, name string) (*RepoLabel, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/labels/%s", c.baseURL, parts[0], parts[1], neturl.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var label RepoLabel
	if err := json.NewDecoder(resp.Body).Decode(&label); err != nil {
		return nil, err
	}
	return &label, nil
}

// CreateRepoLabel creates a new label on a repository.
func (c *RESTClient) CreateRepoLabel(ctx context.Context, repo, name, color, description string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}

	payload, err := json.Marshal(map[string]string{
		"name":        name,
		"color":       color,
		"description": description,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%srepos/%s/%s/labels", c.baseURL, parts[0], parts[1])
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

// UpdateRepoLabel updates an existing label's color and description.
func (c *RESTClient) UpdateRepoLabel(ctx context.Context, repo, name, color, description string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}

	payload, err := json.Marshal(map[string]string{
		"color":       color,
		"description": description,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%srepos/%s/%s/labels/%s", c.baseURL, parts[0], parts[1], neturl.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}
```

**Step 2: Run tests to verify they pass**

Run: `go test -race -run "TestGetRepoLabel|TestCreateRepoLabel|TestUpdateRepoLabel" ./internal/github/...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/github/rest.go internal/github/rest_test.go
git commit -m "feat: add repo-level label CRUD methods to GitHub REST client"
```

---

### Task 3: Create setup package — failing tests

**Files:**
- Create: `internal/setup/setup_test.go`

**Step 1: Write failing tests for EnsureLabels**

Create `internal/setup/setup_test.go`:

```go
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
	if len(client.created) != 4 {
		t.Errorf("created %d labels, want 4: %v", len(client.created), client.created)
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

	results, err := EnsureLabels(context.Background(), cfg, client)
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
```

**Step 2: Run tests to verify they fail**

Run: `go test -race ./internal/setup/...`
Expected: FAIL — package doesn't exist yet

---

### Task 4: Create setup package — implementation

**Files:**
- Create: `internal/setup/labels.go`
- Create: `internal/setup/setup.go`

**Step 1: Create labels.go with canonical label definitions**

Create `internal/setup/labels.go`:

```go
package setup

import "github.com/bketelsen/gopilot/internal/config"

// LabelDef defines a label with its canonical color and description.
type LabelDef struct {
	Name        string
	Color       string
	Description string
}

// RequiredLabels returns the set of labels that gopilot setup should ensure
// exist on each configured repository. It merges config-customized names
// with canonical colors/descriptions.
func RequiredLabels(cfg *config.Config) []LabelDef {
	var labels []LabelDef

	// Eligible labels from config (each gets the blue color)
	for _, name := range cfg.GitHub.EligibleLabels {
		labels = append(labels, LabelDef{
			Name:        name,
			Color:       "0052CC",
			Description: "Eligible for Gopilot agent dispatch",
		})
	}

	// Planning label
	labels = append(labels, LabelDef{
		Name:        cfg.Planning.Label,
		Color:       "7B61FF",
		Description: "Gopilot interactive planning",
	})

	// Completed planning label
	labels = append(labels, LabelDef{
		Name:        cfg.Planning.CompletedLabel,
		Color:       "1D7644",
		Description: "Planning completed by Gopilot",
	})

	// Failure label (hard-coded in orchestrator)
	labels = append(labels, LabelDef{
		Name:        "gopilot-failed",
		Color:       "D93F0B",
		Description: "Gopilot agent failed after max retries",
	})

	return labels
}
```

**Step 2: Create setup.go with EnsureLabels function**

Create `internal/setup/setup.go`:

```go
package setup

import (
	"context"
	"fmt"

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
	var out string
	for _, r := range results {
		out += r.Repo + ": "
		for i, a := range r.Actions {
			if i > 0 {
				out += ", "
			}
			switch a.Action {
			case "ok":
				out += a.Name + " (ok)"
			default:
				out += a.Action + " " + a.Name
			}
		}
		out += "\n"
	}
	return out
}
```

**Step 3: Run tests to verify they pass**

Run: `go test -race ./internal/setup/...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/setup/
git commit -m "feat: add setup package with label bootstrapping"
```

---

### Task 5: Wire setup command into CLI

**Files:**
- Modify: `cmd/gopilot/main.go`

**Step 1: Add "setup" case and update init message**

In `cmd/gopilot/main.go`, add the `"setup"` case to the switch and update `runInit`:

Add import:
```go
"github.com/bketelsen/gopilot/internal/setup"
```

Add case in the switch (after the `"init"` case, before the closing `}`):
```go
case "setup":
    runSetup()
    return
```

Add `runSetup` function:
```go
func runSetup() {
	configPath := "gopilot.yaml"
	if len(os.Args) > 2 {
		configPath = os.Args[2]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	client := ghclient.NewRESTClient(cfg.GitHub, "https://api.github.com/")
	results, err := setup.EnsureLabels(context.Background(), cfg, client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(setup.FormatResults(results))
}
```

Update `runInit` output message — change:
```go
fmt.Printf("Created %s — edit it with your GitHub token and repos.\n", path)
```
to:
```go
fmt.Printf("Created %s — edit it with your GitHub token and repos, then run `gopilot setup` to create labels.\n", path)
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/gopilot/...`
Expected: success

**Step 3: Commit**

```bash
git add cmd/gopilot/main.go
git commit -m "feat: add gopilot setup command and update init message"
```

---

### Task 6: Update documentation

**Files:**
- Modify: `docs/cli.md`
- Modify: `docs/getting-started.md`
- Modify: `README.md`
- Modify: `CLAUDE.md`

**Step 1: Read current docs to understand what needs updating**

Read `docs/cli.md`, `docs/getting-started.md`, and `README.md` to see their current content.

**Step 2: Add `gopilot setup` to CLI reference**

Add a section to `docs/cli.md` documenting the `setup` subcommand, its purpose, and example output.

**Step 3: Mention `gopilot setup` in getting-started flow**

Update `docs/getting-started.md` to include the setup step after editing the config.

**Step 4: Update README if it has a quick-start section**

Add `gopilot setup` to the quick-start flow in `README.md` if applicable.

**Step 5: Commit**

```bash
git add docs/cli.md docs/getting-started.md README.md CLAUDE.md
git commit -m "docs: add gopilot setup command to CLI reference and getting started"
```

---

### Task 7: Run full test suite

**Step 1: Run all tests**

Run: `go test -race ./...`
Expected: PASS

**Step 2: Run linter**

Run: `go vet ./...`
Expected: clean
