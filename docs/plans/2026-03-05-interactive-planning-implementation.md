# Interactive Planning Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add interactive planning to gopilot — issues labeled `gopilot:plan` trigger a planning agent that conducts Q&A in comments, proposes a structured plan, and creates child issues on approval.

**Architecture:** Orchestrator-managed lifecycle with stateless agent invocations. A `PlanningDispatcher` partitions planning issues from coding issues. The agent is invoked per-comment with full thread context, guided by a planning skill. New GitHub client methods handle issue creation, sub-issues, comment fetching, and label removal.

**Tech Stack:** Go, GitHub REST + GraphQL APIs, existing agent Runner interface, SKILL.md behavioral contracts

---

### Task 1: Add Comment domain type

**Files:**
- Modify: `internal/domain/types.go:1-203`
- Test: `internal/domain/types_test.go`

**Step 1: Write the failing test**

Add to `internal/domain/types_test.go`:

```go
func TestCommentSorting(t *testing.T) {
	comments := []Comment{
		{ID: 2, Author: "user", Body: "second", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ID: 1, Author: "bot", Body: "first", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	SortCommentsByTime(comments)
	if comments[0].ID != 1 {
		t.Errorf("first comment ID = %d, want 1", comments[0].ID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/domain/ -run TestCommentSorting -v`
Expected: FAIL — `Comment` type and `SortCommentsByTime` not defined

**Step 3: Write minimal implementation**

Add to `internal/domain/types.go`:

```go
// Comment represents a GitHub issue comment.
type Comment struct {
	ID        int
	Author    string
	Body      string
	CreatedAt time.Time
}

// SortCommentsByTime sorts comments by creation time (oldest first).
func SortCommentsByTime(comments []Comment) {
	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/debian/gopilot && go test ./internal/domain/ -run TestCommentSorting -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/types.go internal/domain/types_test.go
git commit -m "feat: add Comment domain type with time sorting"
```

---

### Task 2: Add PlanningConfig to config

**Files:**
- Modify: `internal/config/config.go:1-142`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestPlanningConfigDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()

	if cfg.Planning.Label != "gopilot:plan" {
		t.Errorf("Planning.Label = %q, want %q", cfg.Planning.Label, "gopilot:plan")
	}
	if cfg.Planning.CompletedLabel != "gopilot:planned" {
		t.Errorf("Planning.CompletedLabel = %q, want %q", cfg.Planning.CompletedLabel, "gopilot:planned")
	}
	if cfg.Planning.ApproveCommand != "/approve" {
		t.Errorf("Planning.ApproveCommand = %q, want %q", cfg.Planning.ApproveCommand, "/approve")
	}
	if cfg.Planning.MaxQuestions != 10 {
		t.Errorf("Planning.MaxQuestions = %d, want 10", cfg.Planning.MaxQuestions)
	}
}

func TestPlanningConfigFromYAML(t *testing.T) {
	yaml := `
github:
  token: tok
  repos: [o/r]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
planning:
  label: "custom:plan"
  agent: "claude"
  model: "claude-sonnet-4-6"
  max_questions: 5
`
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Planning.Label != "custom:plan" {
		t.Errorf("Planning.Label = %q, want %q", cfg.Planning.Label, "custom:plan")
	}
	if cfg.Planning.Agent != "claude" {
		t.Errorf("Planning.Agent = %q, want %q", cfg.Planning.Agent, "claude")
	}
	if cfg.Planning.Model != "claude-sonnet-4-6" {
		t.Errorf("Planning.Model = %q, want %q", cfg.Planning.Model, "claude-sonnet-4-6")
	}
	if cfg.Planning.MaxQuestions != 5 {
		t.Errorf("Planning.MaxQuestions = %d, want 5", cfg.Planning.MaxQuestions)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/config/ -run TestPlanningConfig -v`
Expected: FAIL — `PlanningConfig` not defined

**Step 3: Write minimal implementation**

Add to `internal/config/config.go`:

```go
// Add Planning field to Config struct:
type Config struct {
	GitHub    GitHubConfig    `yaml:"github"`
	Polling   PollingConfig   `yaml:"polling"`
	Workspace WorkspaceConfig `yaml:"workspace"`
	Agent     AgentConfig     `yaml:"agent"`
	Skills    SkillsConfig    `yaml:"skills"`
	Dashboard DashboardConfig `yaml:"dashboard"`
	Planning  PlanningConfig  `yaml:"planning"`
	Prompt    string          `yaml:"prompt"`
}

// New type:
type PlanningConfig struct {
	Label          string `yaml:"label"`
	CompletedLabel string `yaml:"completed_label"`
	ApproveCommand string `yaml:"approve_command"`
	MaxQuestions   int    `yaml:"max_questions"`
	Agent          string `yaml:"agent"`
	Model          string `yaml:"model"`
}
```

Add defaults in `ApplyDefaults()`:

```go
if cfg.Planning.Label == "" {
	cfg.Planning.Label = "gopilot:plan"
}
if cfg.Planning.CompletedLabel == "" {
	cfg.Planning.CompletedLabel = "gopilot:planned"
}
if cfg.Planning.ApproveCommand == "" {
	cfg.Planning.ApproveCommand = "/approve"
}
if cfg.Planning.MaxQuestions == 0 {
	cfg.Planning.MaxQuestions = 10
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/debian/gopilot && go test ./internal/config/ -run TestPlanningConfig -v`
Expected: PASS

**Step 5: Run all config tests**

Run: `cd /home/debian/gopilot && go test ./internal/config/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add PlanningConfig with label, agent, model, and defaults"
```

---

### Task 3: Add FetchIssueComments to GitHub REST client

**Files:**
- Modify: `internal/github/client.go:1-18`
- Modify: `internal/github/rest.go:1-293`
- Test: `internal/github/rest_test.go`

**Step 1: Write the failing test**

Add to `internal/github/rest_test.go`:

```go
func TestFetchIssueComments(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/owner/repo/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
		comments := []map[string]any{
			{
				"id":         101,
				"body":       "First comment",
				"created_at": "2026-01-01T00:00:00Z",
				"user":       map[string]any{"login": "alice"},
			},
			{
				"id":         102,
				"body":       "Second comment",
				"created_at": "2026-01-02T00:00:00Z",
				"user":       map[string]any{"login": "gopilot[bot]"},
			},
		}
		json.NewEncoder(w).Encode(comments)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	comments, err := client.FetchIssueComments(context.Background(), "owner/repo", 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(comments) != 2 {
		t.Fatalf("got %d comments, want 2", len(comments))
	}
	if comments[0].Author != "alice" {
		t.Errorf("comment[0].Author = %q, want %q", comments[0].Author, "alice")
	}
	if comments[1].ID != 102 {
		t.Errorf("comment[1].ID = %d, want 102", comments[1].ID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/github/ -run TestFetchIssueComments -v`
Expected: FAIL — method not defined

**Step 3: Write minimal implementation**

Add to `internal/github/client.go` — extend the `Client` interface:

```go
type Client interface {
	FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error)
	FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error)
	FetchIssueStates(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error)
	FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error)
	SetProjectStatus(ctx context.Context, issue domain.Issue, status string) error
	AddComment(ctx context.Context, repo string, id int, body string) error
	AddLabel(ctx context.Context, repo string, id int, label string) error
	RemoveLabel(ctx context.Context, repo string, id int, label string) error
	CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*domain.Issue, error)
	AddSubIssue(ctx context.Context, repo string, parentID, childID int) error
	EnrichIssues(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error)
}
```

Add to `internal/github/rest.go`:

```go
// ghComment is the raw GitHub API comment shape.
type ghComment struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (c *RESTClient) FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/comments?per_page=100", c.baseURL, parts[0], parts[1], id)
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var raw []ghComment
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding comments: %w", err)
	}

	comments := make([]domain.Comment, len(raw))
	for i, r := range raw {
		comments[i] = domain.Comment{
			ID:        r.ID,
			Author:    r.User.Login,
			Body:      r.Body,
			CreatedAt: r.CreatedAt,
		}
	}
	return comments, nil
}
```

**Step 4: Add stub methods for new interface methods to RESTClient**

The Client interface now has `RemoveLabel`, `CreateIssue`, `AddSubIssue`. Add stubs so the code compiles (full implementations in Tasks 4-5):

```go
func (c *RESTClient) RemoveLabel(ctx context.Context, repo string, id int, label string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/labels/%s", c.baseURL, parts[0], parts[1], id, label)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (c *RESTClient) CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*domain.Issue, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *RESTClient) AddSubIssue(ctx context.Context, repo string, parentID, childID int) error {
	return fmt.Errorf("not implemented")
}
```

**Step 5: Update mock clients in orchestrator tests**

Add new methods to `mockGitHub` and `mockGitHubSplit` in `internal/orchestrator/orchestrator_test.go`:

```go
// Add to mockGitHub:
func (m *mockGitHub) FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error) {
	return nil, nil
}
func (m *mockGitHub) RemoveLabel(ctx context.Context, repo string, id int, label string) error {
	return nil
}
func (m *mockGitHub) CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*domain.Issue, error) {
	return nil, nil
}
func (m *mockGitHub) AddSubIssue(ctx context.Context, repo string, parentID, childID int) error {
	return nil
}

// Add same to mockGitHubSplit
```

**Step 6: Run test to verify it passes**

Run: `cd /home/debian/gopilot && go test ./internal/github/ -run TestFetchIssueComments -v`
Expected: PASS

**Step 7: Run all tests to verify nothing broke**

Run: `cd /home/debian/gopilot && go test ./... -v`
Expected: ALL PASS

**Step 8: Commit**

```bash
git add internal/github/client.go internal/github/rest.go internal/github/rest_test.go internal/orchestrator/orchestrator_test.go
git commit -m "feat: add FetchIssueComments, RemoveLabel, CreateIssue, AddSubIssue to GitHub client interface"
```

---

### Task 4: Implement CreateIssue REST method

**Files:**
- Modify: `internal/github/rest.go`
- Test: `internal/github/rest_test.go`

**Step 1: Write the failing test**

Add to `internal/github/rest_test.go`:

```go
func TestCreateIssue(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)

		if payload["title"] != "New issue" {
			t.Errorf("title = %q, want %q", payload["title"], "New issue")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"number":     42,
			"node_id":    "MDU6SXNzdWU0Mg==",
			"title":      payload["title"],
			"body":       payload["body"],
			"state":      "open",
			"html_url":   "https://github.com/owner/repo/issues/42",
			"labels":     []map[string]any{{"name": "gopilot"}},
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}, EligibleLabels: []string{"gopilot"}}
	client := NewRESTClient(cfg, server.URL+"/")

	issue, err := client.CreateIssue(context.Background(), "owner/repo", "New issue", "Body text", []string{"gopilot"})
	if err != nil {
		t.Fatal(err)
	}
	if issue.ID != 42 {
		t.Errorf("issue.ID = %d, want 42", issue.ID)
	}
	if issue.Title != "New issue" {
		t.Errorf("issue.Title = %q, want %q", issue.Title, "New issue")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/github/ -run TestCreateIssue -v`
Expected: FAIL — returns "not implemented" error

**Step 3: Replace the stub implementation**

In `internal/github/rest.go`, replace the `CreateIssue` stub:

```go
func (c *RESTClient) CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*domain.Issue, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}

	payload := map[string]any{
		"title":  title,
		"body":   body,
		"labels": labels,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%srepos/%s/%s/issues", c.baseURL, parts[0], parts[1])
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, respBody)
	}

	var raw ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	issue := raw.toDomain(repo)
	return &issue, nil
}
```

Note: add `"bytes"` to the imports if not already present.

**Step 4: Run test to verify it passes**

Run: `cd /home/debian/gopilot && go test ./internal/github/ -run TestCreateIssue -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/github/rest.go internal/github/rest_test.go
git commit -m "feat: implement CreateIssue REST method"
```

---

### Task 5: Implement AddSubIssue REST method

**Files:**
- Modify: `internal/github/rest.go`
- Test: `internal/github/rest_test.go`

**Step 1: Write the failing test**

Add to `internal/github/rest_test.go`:

```go
func TestAddSubIssue(t *testing.T) {
	var called bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/owner/repo/issues/1/sub_issues", func(w http.ResponseWriter, r *http.Request) {
		called = true
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)

		// sub_issue_id should be the child issue ID
		if payload["sub_issue_id"] != float64(2) {
			t.Errorf("sub_issue_id = %v, want 2", payload["sub_issue_id"])
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := config.GitHubConfig{Token: "test-token", Repos: []string{"owner/repo"}}
	client := NewRESTClient(cfg, server.URL+"/")

	err := client.AddSubIssue(context.Background(), "owner/repo", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("API was not called")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/github/ -run TestAddSubIssue -v`
Expected: FAIL — returns "not implemented"

**Step 3: Replace the stub**

In `internal/github/rest.go`, replace `AddSubIssue` stub:

```go
func (c *RESTClient) AddSubIssue(ctx context.Context, repo string, parentID, childID int) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}

	payload := fmt.Sprintf(`{"sub_issue_id":%d}`, childID)
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/sub_issues", c.baseURL, parts[0], parts[1], parentID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
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
```

**Step 4: Run test to verify it passes**

Run: `cd /home/debian/gopilot && go test ./internal/github/ -run TestAddSubIssue -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/github/rest.go internal/github/rest_test.go
git commit -m "feat: implement AddSubIssue REST method"
```

---

### Task 6: Add PlanningState to orchestrator state

**Files:**
- Modify: `internal/orchestrator/state.go:1-148`
- Test: `internal/orchestrator/state_test.go`

**Step 1: Write the failing test**

Add to `internal/orchestrator/state_test.go`:

```go
func TestPlanningState(t *testing.T) {
	s := NewState()

	// Initially no planning entries
	if s.PlanningCount() != 0 {
		t.Errorf("initial PlanningCount = %d, want 0", s.PlanningCount())
	}

	// Add a planning entry
	entry := &PlanningEntry{
		IssueID:        1,
		Repo:           "o/r",
		Phase:          PlanningPhaseDetected,
		LastCommentID:  0,
		QuestionsAsked: 0,
	}
	s.AddPlanning(1, entry)

	if s.PlanningCount() != 1 {
		t.Errorf("PlanningCount = %d, want 1", s.PlanningCount())
	}
	if s.GetPlanning(1) == nil {
		t.Error("GetPlanning(1) = nil, want entry")
	}
	if !s.IsPlanning(1) {
		t.Error("IsPlanning(1) = false, want true")
	}

	// Update phase
	s.GetPlanning(1).Phase = PlanningPhaseQuestioning
	if s.GetPlanning(1).Phase != PlanningPhaseQuestioning {
		t.Errorf("Phase = %q, want %q", s.GetPlanning(1).Phase, PlanningPhaseQuestioning)
	}

	// Remove
	s.RemovePlanning(1)
	if s.PlanningCount() != 0 {
		t.Errorf("after remove, PlanningCount = %d, want 0", s.PlanningCount())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/orchestrator/ -run TestPlanningState -v`
Expected: FAIL — types not defined

**Step 3: Write minimal implementation**

Add to `internal/orchestrator/state.go`:

```go
// PlanningPhase represents the current phase of a planning conversation.
type PlanningPhase string

const (
	PlanningPhaseDetected        PlanningPhase = "detected"
	PlanningPhaseQuestioning     PlanningPhase = "questioning"
	PlanningPhaseAwaitingReply   PlanningPhase = "awaiting_reply"
	PlanningPhasePlanProposed    PlanningPhase = "plan_proposed"
	PlanningPhaseAwaitingApproval PlanningPhase = "awaiting_approval"
	PlanningPhaseCreatingIssues  PlanningPhase = "creating_issues"
	PlanningPhaseComplete        PlanningPhase = "complete"
)

// PlanningEntry tracks an active planning conversation.
type PlanningEntry struct {
	IssueID        int
	Repo           string
	Phase          PlanningPhase
	LastCommentID  int
	QuestionsAsked int
}
```

Add `planning` field to `State` struct:

```go
type State struct {
	mu        sync.RWMutex
	running   map[int]*domain.RunEntry
	claimed   map[int]bool
	retry     map[int]*domain.RetryEntry
	history   map[int][]domain.CompletedRun
	completed map[int]bool
	planning  map[int]*PlanningEntry
	totals    domain.TokenTotals
}
```

Update `NewState`:

```go
func NewState() *State {
	return &State{
		running:   make(map[int]*domain.RunEntry),
		claimed:   make(map[int]bool),
		retry:     make(map[int]*domain.RetryEntry),
		history:   make(map[int][]domain.CompletedRun),
		completed: make(map[int]bool),
		planning:  make(map[int]*PlanningEntry),
	}
}
```

Add methods:

```go
func (s *State) AddPlanning(issueID int, entry *PlanningEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planning[issueID] = entry
}

func (s *State) GetPlanning(issueID int) *PlanningEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.planning[issueID]
}

func (s *State) RemovePlanning(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.planning, issueID)
}

func (s *State) IsPlanning(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.planning[issueID]
	return ok
}

func (s *State) PlanningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.planning)
}

func (s *State) AllPlanning() []*PlanningEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*PlanningEntry, 0, len(s.planning))
	for _, e := range s.planning {
		entries = append(entries, e)
	}
	return entries
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/debian/gopilot && go test ./internal/orchestrator/ -run TestPlanningState -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/state.go internal/orchestrator/state_test.go
git commit -m "feat: add PlanningEntry and planning state management"
```

---

### Task 7: Create the planning skill

**Files:**
- Create: `skills/planning/SKILL.md`

**Step 1: Write the skill file**

```markdown
---
name: planning
description: Guide interactive planning conversations in GitHub issues
type: rigid
---

## Role

You are a planning agent. Your job is to help decompose a feature idea into actionable GitHub issues through structured conversation.

## Phase Detection

Read the full comment thread to determine your current phase:

1. **No agent comments exist** -> You are in the INTRODUCTION phase. Post a greeting and your first clarifying question.
2. **You asked a question, human replied** -> You are in the QUESTIONING phase. Ask the next question or, if you have enough context, propose a plan.
3. **A plan comment with checkboxes exists** -> You are in the PLAN_PROPOSED phase. If the human has feedback, revise the plan. If they commented the approve command, proceed to issue creation.

## Rules

- Ask ONE question per response. Never ask multiple questions at once.
- Prefer multiple-choice questions when possible.
- Focus on: purpose, constraints, success criteria, dependencies, testing strategy.
- Keep questions concise. Do not over-explain.
- After gathering enough context (typically 3-8 questions), propose a plan.

## Plan Format

When proposing a plan, use this exact format:

```
## Proposed Plan: [Feature Name]

### Phase 1: [Phase Name]
- [ ] **Issue title** -- Description of what this issue covers. _Complexity: small/medium/large_

### Phase 2: [Phase Name]
- [ ] **Issue title** -- Description. `blocked by #N` if applicable. _Complexity: small/medium/large_

### Dependencies
- [List cross-phase dependencies]

### Notes
- [Rationale for granularity decisions]
- [Assumptions made]

Reply `/approve` to create these issues, or uncheck items you want to remove and comment with your changes.
```

## Granularity

Decide issue granularity based on complexity:
- **Simple features**: Fewer, coarser issues (one per shippable unit)
- **Complex features**: More granular breakdown (infrastructure, API, UI, tests as separate issues)
- Always explain your rationale in the Notes section.

## Issue Creation

When the human approves (comments the approve command):
1. Parse checked items only (`- [x]`)
2. Create Phase 1 issues first
3. Create Phase 2+ issues with `blocked by #N` referencing real issue numbers
4. Add all created issues as sub-issues of the planning issue
5. Post a summary comment with links to all created issues
```

**Step 2: Verify the skill loads**

Run: `cd /home/debian/gopilot && go test ./internal/skills/ -v`
Expected: PASS (existing tests should still pass; new skill discovered if skills dir is configured)

**Step 3: Commit**

```bash
git add skills/planning/SKILL.md
git commit -m "feat: add planning skill for interactive issue decomposition"
```

---

### Task 8: Create the PlanningDispatcher

**Files:**
- Create: `internal/orchestrator/planning.go`
- Test: `internal/orchestrator/planning_test.go`

**Step 1: Write the failing test**

Create `internal/orchestrator/planning_test.go`:

```go
package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

func TestPartitionPlanningIssues(t *testing.T) {
	issues := []domain.Issue{
		{ID: 1, Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo"},
		{ID: 2, Labels: []string{"gopilot"}, Status: "Todo"},
		{ID: 3, Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo"},
	}

	planning, coding := partitionPlanningIssues(issues, "gopilot:plan")

	if len(planning) != 2 {
		t.Errorf("planning = %d, want 2", len(planning))
	}
	if len(coding) != 1 {
		t.Errorf("coding = %d, want 1", len(coding))
	}
	if coding[0].ID != 2 {
		t.Errorf("coding[0].ID = %d, want 2", coding[0].ID)
	}
}

// mockPlanningGitHub extends mockGitHub with comment tracking.
type mockPlanningGitHub struct {
	mockGitHub
	comments      map[int][]domain.Comment
	postedComment string
}

func (m *mockPlanningGitHub) FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error) {
	return m.comments[id], nil
}

func (m *mockPlanningGitHub) AddComment(ctx context.Context, repo string, id int, body string) error {
	m.postedComment = body
	return nil
}

func TestPlanningDispatcherDetectsNewComments(t *testing.T) {
	cfg := &config.Config{
		Planning: config.PlanningConfig{
			Label:          "gopilot:plan",
			CompletedLabel: "gopilot:planned",
			ApproveCommand: "/approve",
			MaxQuestions:   10,
		},
	}
	state := NewState()

	// Add a planning entry that has seen comment 100
	state.AddPlanning(1, &PlanningEntry{
		IssueID:       1,
		Repo:          "o/r",
		Phase:         PlanningPhaseAwaitingReply,
		LastCommentID: 100,
	})

	gh := &mockPlanningGitHub{
		comments: map[int][]domain.Comment{
			1: {
				{ID: 100, Author: "gopilot[bot]", Body: "What is the goal?", CreatedAt: time.Now().Add(-time.Minute)},
				{ID: 101, Author: "user", Body: "Build a feature", CreatedAt: time.Now()},
			},
		},
	}

	hasNew, lastID := hasNewHumanComment(gh, context.Background(), "o/r", 1, 100)
	if !hasNew {
		t.Error("hasNew = false, want true")
	}
	if lastID != 101 {
		t.Errorf("lastID = %d, want 101", lastID)
	}
}

func TestPlanningDispatcherNoNewComments(t *testing.T) {
	gh := &mockPlanningGitHub{
		comments: map[int][]domain.Comment{
			1: {
				{ID: 100, Author: "gopilot[bot]", Body: "What is the goal?", CreatedAt: time.Now()},
			},
		},
	}

	hasNew, _ := hasNewHumanComment(gh, context.Background(), "o/r", 1, 100)
	if hasNew {
		t.Error("hasNew = true, want false (only bot comment)")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/orchestrator/ -run TestPartition -v`
Expected: FAIL — `partitionPlanningIssues` not defined

**Step 3: Write minimal implementation**

Create `internal/orchestrator/planning.go`:

```go
package orchestrator

import (
	"context"
	"strings"

	"github.com/bketelsen/gopilot/internal/domain"
	gh "github.com/bketelsen/gopilot/internal/github"
)

// partitionPlanningIssues splits issues into planning and coding sets.
func partitionPlanningIssues(issues []domain.Issue, planningLabel string) (planning, coding []domain.Issue) {
	for _, issue := range issues {
		isPlan := false
		for _, label := range issue.Labels {
			if strings.EqualFold(label, planningLabel) {
				isPlan = true
				break
			}
		}
		if isPlan {
			planning = append(planning, issue)
		} else {
			coding = append(coding, issue)
		}
	}
	return
}

// hasNewHumanComment checks if there are new non-bot comments after lastCommentID.
// Returns true and the latest comment ID if a new human comment is found.
func hasNewHumanComment(client gh.Client, ctx context.Context, repo string, issueID, lastCommentID int) (bool, int) {
	comments, err := client.FetchIssueComments(ctx, repo, issueID)
	if err != nil {
		return false, lastCommentID
	}

	latestID := lastCommentID
	hasNew := false
	for _, c := range comments {
		if c.ID > lastCommentID && !isBot(c.Author) {
			hasNew = true
		}
		if c.ID > latestID {
			latestID = c.ID
		}
	}
	return hasNew, latestID
}

// isBot returns true if the author looks like a bot account.
func isBot(author string) bool {
	return strings.HasSuffix(author, "[bot]")
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/debian/gopilot && go test ./internal/orchestrator/ -run "TestPartition|TestPlanningDispatcher" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/planning.go internal/orchestrator/planning_test.go
git commit -m "feat: add PlanningDispatcher with issue partitioning and comment detection"
```

---

### Task 9: Build the planning prompt renderer

**Files:**
- Create: `internal/prompt/planning.go`
- Test: `internal/prompt/planning_test.go`

**Step 1: Write the failing test**

Create `internal/prompt/planning_test.go`:

```go
package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

func TestRenderPlanningPrompt(t *testing.T) {
	issue := domain.Issue{
		ID:    1,
		Repo:  "o/r",
		Title: "Build auth system",
		Body:  "We need user authentication with OAuth",
	}
	comments := []domain.Comment{
		{ID: 100, Author: "gopilot[bot]", Body: "What providers?", CreatedAt: time.Now().Add(-time.Minute)},
		{ID: 101, Author: "user", Body: "Google and GitHub", CreatedAt: time.Now()},
	}
	skill := "## Skill: planning (rigid)\n\nYou are a planning agent."

	result := RenderPlanning(issue, comments, skill)

	if !strings.Contains(result, "Build auth system") {
		t.Error("missing issue title")
	}
	if !strings.Contains(result, "We need user authentication with OAuth") {
		t.Error("missing issue body")
	}
	if !strings.Contains(result, "gopilot[bot]: What providers?") {
		t.Error("missing bot comment")
	}
	if !strings.Contains(result, "user: Google and GitHub") {
		t.Error("missing user comment")
	}
	if !strings.Contains(result, "planning agent") {
		t.Error("missing skill content")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/prompt/ -run TestRenderPlanningPrompt -v`
Expected: FAIL — `RenderPlanning` not defined

**Step 3: Write minimal implementation**

Create `internal/prompt/planning.go`:

```go
package prompt

import (
	"fmt"
	"strings"

	"github.com/bketelsen/gopilot/internal/domain"
)

// RenderPlanning builds the prompt for a planning agent invocation.
func RenderPlanning(issue domain.Issue, comments []domain.Comment, skill string) string {
	var b strings.Builder

	b.WriteString("# Planning Task\n\n")
	b.WriteString(fmt.Sprintf("## Issue: %s (#%d)\n\n", issue.Title, issue.ID))
	b.WriteString(issue.Body)
	b.WriteString("\n\n")

	if len(comments) > 0 {
		b.WriteString("## Conversation History\n\n")
		for _, c := range comments {
			b.WriteString(fmt.Sprintf("%s: %s\n\n", c.Author, c.Body))
		}
	}

	if skill != "" {
		b.WriteString("## Instructions\n\n")
		b.WriteString(skill)
		b.WriteString("\n")
	}

	return b.String()
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/debian/gopilot && go test ./internal/prompt/ -run TestRenderPlanningPrompt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/prompt/planning.go internal/prompt/planning_test.go
git commit -m "feat: add planning prompt renderer with conversation history"
```

---

### Task 10: Integrate PlanningDispatcher into orchestrator Tick

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:157-232`
- Modify: `internal/orchestrator/planning.go`
- Test: `internal/orchestrator/planning_test.go`

**Step 1: Write the failing test**

Add to `internal/orchestrator/planning_test.go`:

```go
func TestOrchestratorPartitionsPlanningIssues(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 5},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Planning: config.PlanningConfig{
			Label: "gopilot:plan", CompletedLabel: "gopilot:planned",
			ApproveCommand: "/approve", MaxQuestions: 10, Agent: "mock",
		},
		Prompt: "Work",
	}

	gh := &mockPlanningGitHub{
		mockGitHub: mockGitHub{
			issues: []domain.Issue{
				{ID: 1, Repo: "o/r", Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
				{ID: 2, Repo: "o/r", Labels: []string{"gopilot"}, Status: "Todo", Priority: 1, CreatedAt: time.Now()},
			},
		},
		comments: map[int][]domain.Comment{},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	ctx := context.Background()
	orch.Tick(ctx)

	// Issue 2 should be dispatched as coding, issue 1 as planning
	// Planning issue should be tracked in planning state
	if !orch.state.IsPlanning(1) {
		t.Error("issue 1 should be in planning state")
	}
	// Issue 2 should be dispatched normally
	if ag.started < 1 {
		t.Errorf("coding agent started = %d, want >= 1", ag.started)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/debian/gopilot && go test ./internal/orchestrator/ -run TestOrchestratorPartitionsPlanningIssues -v`
Expected: FAIL — planning issues not partitioned yet

**Step 3: Modify the Tick method**

In `internal/orchestrator/orchestrator.go`, modify the `Tick` method. After the candidate filtering loop (around line 206-216), add partitioning before dispatch:

Replace the section that builds `candidates` and dispatches with:

```go
	// Partition planning vs coding issues
	planningIssues, codingCandidates := partitionPlanningIssues(candidates, o.cfg.Planning.Label)

	// Handle planning issues
	o.processPlanningIssues(ctx, planningIssues)

	// Dispatch coding issues
	domain.SortByPriority(codingCandidates)
	for _, issue := range codingCandidates {
		if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
			break
		}
		o.dispatch(ctx, issue, 1)
	}
```

Add the `processPlanningIssues` method to `planning.go`:

```go
// processPlanningIssues handles planning-labeled issues.
func (o *Orchestrator) processPlanningIssues(ctx context.Context, issues []domain.Issue) {
	for _, issue := range issues {
		if o.state.IsPlanning(issue.ID) {
			// Already tracking — check for new comments
			entry := o.state.GetPlanning(issue.ID)
			if entry.Phase == PlanningPhaseAwaitingReply || entry.Phase == PlanningPhasePlanProposed || entry.Phase == PlanningPhaseAwaitingApproval {
				hasNew, lastID := hasNewHumanComment(o.github, ctx, issue.Repo, issue.ID, entry.LastCommentID)
				if hasNew {
					entry.LastCommentID = lastID
					o.dispatchPlanningAgent(ctx, issue, entry)
				}
			}
			continue
		}

		// New planning issue — register and dispatch first interaction
		entry := &PlanningEntry{
			IssueID: issue.ID,
			Repo:    issue.Repo,
			Phase:   PlanningPhaseDetected,
		}
		o.state.AddPlanning(issue.ID, entry)
		o.dispatchPlanningAgent(ctx, issue, entry)
	}
}

// dispatchPlanningAgent invokes the agent for one planning interaction.
func (o *Orchestrator) dispatchPlanningAgent(ctx context.Context, issue domain.Issue, entry *PlanningEntry) {
	if !o.state.SlotsAvailable(o.cfg.Polling.MaxConcurrentAgents) {
		return
	}
	if o.state.IsClaimed(issue.ID) || o.state.GetRunning(issue.ID) != nil {
		return
	}
	if !o.state.Claim(issue.ID) {
		return
	}

	log := slog.With("issue", issue.Identifier(), "planning_phase", string(entry.Phase))

	comments, err := o.github.FetchIssueComments(ctx, issue.Repo, issue.ID)
	if err != nil {
		log.Error("failed to fetch comments for planning", "error", err)
		o.state.Release(issue.ID)
		return
	}

	planningSkillText := skills.InjectSkills(o.skills, []string{"planning"}, nil)
	rendered := prompt.RenderPlanning(issue, comments, planningSkillText)

	agentCmd := o.cfg.Planning.Agent
	if agentCmd == "" {
		agentCmd = o.cfg.Agent.Command
	}
	runner, ok := o.agents[agentCmd]
	if !ok {
		runner = o.agentForIssue(issue)
	}
	if runner == nil {
		log.Error("no agent runner available for planning")
		o.state.Release(issue.ID)
		return
	}

	model := o.cfg.Planning.Model
	if model == "" {
		model = o.cfg.Agent.Model
	}

	opts := agent.AgentOpts{
		Model:            model,
		MaxContinuations: o.cfg.Agent.MaxAutopilotContinues,
	}
	sess, err := runner.Start(ctx, "", rendered, opts)
	if err != nil {
		log.Error("planning agent start failed", "error", err)
		o.state.Release(issue.ID)
		return
	}

	o.sessionsMu.Lock()
	o.sessions[issue.ID] = sess
	o.sessionsMu.Unlock()

	now := time.Now()
	runEntry := &domain.RunEntry{
		Issue:       issue,
		SessionID:   sess.ID,
		ProcessPID:  sess.PID,
		StartedAt:   now,
		LastEventAt: now,
		Attempt:     1,
	}
	o.state.AddRunning(issue.ID, runEntry)

	entry.Phase = PlanningPhaseAwaitingReply
	entry.QuestionsAsked++

	log.Info("planning agent dispatched", "session_id", sess.ID)

	go o.monitorAgent(issue, sess, runEntry)
}
```

Add needed imports to planning.go:

```go
import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/domain"
	gh "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/prompt"
	"github.com/bketelsen/gopilot/internal/skills"
)
```

**Step 4: Run test to verify it passes**

Run: `cd /home/debian/gopilot && go test ./internal/orchestrator/ -run TestOrchestratorPartitionsPlanningIssues -v`
Expected: PASS

**Step 5: Run all orchestrator tests**

Run: `cd /home/debian/gopilot && go test ./internal/orchestrator/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/planning.go internal/orchestrator/planning_test.go
git commit -m "feat: integrate PlanningDispatcher into orchestrator Tick loop"
```

---

### Task 11: Add planning SSE events

**Files:**
- Modify: `internal/orchestrator/planning.go`

**Step 1: Add SSE broadcasts to planning methods**

In `processPlanningIssues`, after dispatching, broadcast events:

```go
// After dispatching a new planning issue:
if o.sseHub != nil {
	o.sseHub.Broadcast("planning:question_posted", fmt.Sprintf(`{"issue_id":%d}`, issue.ID))
}

// After detecting new human comment:
if o.sseHub != nil {
	o.sseHub.Broadcast("planning:reply_detected", fmt.Sprintf(`{"issue_id":%d}`, issue.ID))
}
```

This is a small addition — no separate test needed since SSE broadcasting is already tested via the existing hub tests.

**Step 2: Run all tests**

Run: `cd /home/debian/gopilot && go test ./... -v`
Expected: ALL PASS

**Step 3: Commit**

```bash
git add internal/orchestrator/planning.go
git commit -m "feat: add planning SSE event broadcasts"
```

---

### Task 12: Add planning badge to dashboard

**Files:**
- Modify: `internal/web/templates/pages/dashboard.templ` (or equivalent dashboard template)
- Modify: `internal/web/server.go` (expose planning state to templates)

This task depends on the exact templ template structure. The key changes:

**Step 1: Expose planning state in the web server**

In `internal/web/server.go`, the `State` interface (or struct passed to templates) needs a method to check if an issue is in planning mode. Add a method or pass planning state alongside running state.

**Step 2: Add planning badge in template**

In the dashboard template, where issue status badges are rendered, add:

```go
// Pseudocode for templ:
if isPlanningIssue {
    <span class="badge badge-planning">Planning</span>
}
```

Use a distinct color (e.g., purple/indigo) to differentiate from running (green) and failed (red).

**Step 3: Run build and verify**

Run: `cd /home/debian/gopilot && task generate && task css && task build`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add internal/web/
git commit -m "feat: add planning badge to dashboard for planning issues"
```

---

### Task 13: End-to-end integration test

**Files:**
- Create: `internal/orchestrator/planning_integration_test.go`

**Step 1: Write an integration test that simulates the full planning lifecycle**

```go
package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

func TestPlanningFullLifecycle(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "tok", Repos: []string{"o/r"}, EligibleLabels: []string{"gopilot"},
		},
		Polling: config.PollingConfig{IntervalMS: 1000, MaxConcurrentAgents: 5},
		Agent: config.AgentConfig{
			Command: "mock", TurnTimeoutMS: 60000, StallTimeoutMS: 60000,
			MaxRetries: 3, MaxRetryBackoffMS: 1000, MaxAutopilotContinues: 5,
		},
		Workspace: config.WorkspaceConfig{Root: t.TempDir(), HookTimeoutMS: 5000},
		Planning: config.PlanningConfig{
			Label: "gopilot:plan", CompletedLabel: "gopilot:planned",
			ApproveCommand: "/approve", MaxQuestions: 10, Agent: "mock",
		},
		Prompt: "Work",
	}

	planningIssue := domain.Issue{
		ID: 1, Repo: "o/r", Title: "Plan: Auth system",
		Labels: []string{"gopilot", "gopilot:plan"}, Status: "Todo",
		Body: "We need auth", CreatedAt: time.Now(),
	}
	codingIssue := domain.Issue{
		ID: 2, Repo: "o/r", Title: "Fix bug",
		Labels: []string{"gopilot"}, Status: "Todo",
		CreatedAt: time.Now(),
	}

	gh := &mockPlanningGitHub{
		mockGitHub: mockGitHub{issues: []domain.Issue{planningIssue, codingIssue}},
		comments:   map[int][]domain.Comment{},
	}
	ag := &mockAgent{}
	orch := NewOrchestrator(cfg, gh, map[string]agent.Runner{"mock": ag})

	ctx := context.Background()

	// Tick 1: Both issues picked up
	orch.Tick(ctx)

	if !orch.state.IsPlanning(1) {
		t.Fatal("issue 1 should be in planning state after tick 1")
	}

	// Wait for the mock agent to finish (it completes immediately via context)
	time.Sleep(100 * time.Millisecond)

	// Verify planning entry exists with awaiting_reply phase
	entry := orch.state.GetPlanning(1)
	if entry == nil {
		t.Fatal("planning entry is nil")
	}
	if entry.Phase != PlanningPhaseAwaitingReply {
		t.Errorf("phase = %q, want %q", entry.Phase, PlanningPhaseAwaitingReply)
	}
}
```

**Step 2: Run the integration test**

Run: `cd /home/debian/gopilot && go test ./internal/orchestrator/ -run TestPlanningFullLifecycle -v`
Expected: PASS

**Step 3: Run all tests**

Run: `cd /home/debian/gopilot && go test ./... -v`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add internal/orchestrator/planning_integration_test.go
git commit -m "test: add planning lifecycle integration test"
```

---

### Task 14: Update config hot-reload to include planning config

**Files:**
- Modify: `internal/orchestrator/orchestrator.go:74-97`

**Step 1: Add planning config to the reload callback**

In the `Run` method's config watcher callback, add:

```go
o.cfg.Planning = newCfg.Planning
```

**Step 2: Run all tests**

Run: `cd /home/debian/gopilot && go test ./... -v`
Expected: ALL PASS

**Step 3: Commit**

```bash
git add internal/orchestrator/orchestrator.go
git commit -m "feat: include planning config in hot-reload"
```

---

### Task 15: Update example config

**Files:**
- Modify: `internal/config/example.go`

**Step 1: Add planning section to ExampleConfig**

Add to the YAML template:

```yaml
# Planning configuration (optional)
# planning:
#   label: "gopilot:plan"           # Label that triggers planning mode
#   completed_label: "gopilot:planned"  # Label applied when planning completes
#   approve_command: "/approve"      # Comment text that triggers issue creation
#   max_questions: 10                # Max questions before forcing plan proposal
#   agent: "claude-code"             # Agent runner for planning (optional)
#   model: "claude-sonnet-4-6"      # Model override for planning (optional)
```

**Step 2: Run build**

Run: `cd /home/debian/gopilot && go build ./...`
Expected: Compiles

**Step 3: Commit**

```bash
git add internal/config/example.go
git commit -m "docs: add planning section to example config"
```

---

## Summary

| Task | Component | Key Files |
|------|-----------|-----------|
| 1 | Comment domain type | `domain/types.go` |
| 2 | PlanningConfig | `config/config.go` |
| 3 | FetchIssueComments + interface expansion | `github/client.go`, `github/rest.go` |
| 4 | CreateIssue REST | `github/rest.go` |
| 5 | AddSubIssue REST | `github/rest.go` |
| 6 | PlanningState | `orchestrator/state.go` |
| 7 | Planning skill | `skills/planning/SKILL.md` |
| 8 | PlanningDispatcher | `orchestrator/planning.go` |
| 9 | Planning prompt renderer | `prompt/planning.go` |
| 10 | Integrate into Tick | `orchestrator/orchestrator.go` |
| 11 | SSE events | `orchestrator/planning.go` |
| 12 | Dashboard badge | `web/` templates |
| 13 | Integration test | `orchestrator/planning_integration_test.go` |
| 14 | Config hot-reload | `orchestrator/orchestrator.go` |
| 15 | Example config | `config/example.go` |

**Dependencies:** Tasks 1-2 are independent. Task 3 depends on 1. Tasks 4-5 depend on 3. Task 6 is independent. Task 7 is independent. Task 8 depends on 6. Task 9 depends on 1. Task 10 depends on 2, 3, 6, 8, 9. Tasks 11-15 depend on 10.
