package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// RateLimit holds parsed GitHub API rate limit information.
type RateLimit struct {
	Remaining int
	Limit     int
	Reset     time.Time
}

// RESTClient implements GitHub REST API operations.
type RESTClient struct {
	cfg       config.GitHubConfig
	baseURL   string
	http      *http.Client
	rateLimit RateLimit
	mu        sync.RWMutex
}

// NewRESTClient creates a REST client. baseURL should end with "/".
func NewRESTClient(cfg config.GitHubConfig, baseURL string) *RESTClient {
	return &RESTClient{
		cfg:     cfg,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// GetRateLimit returns the most recently observed rate limit information.
func (c *RESTClient) GetRateLimit() RateLimit {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rateLimit
}

func (c *RESTClient) updateRateLimit(resp *http.Response) {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	limit := resp.Header.Get("X-RateLimit-Limit")
	reset := resp.Header.Get("X-RateLimit-Reset")

	c.mu.Lock()
	defer c.mu.Unlock()

	if remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil {
			c.rateLimit.Remaining = n
		}
	}
	if limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			c.rateLimit.Limit = n
		}
	}
	if reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			c.rateLimit.Reset = time.Unix(ts, 0)
		}
	}
}

// FetchCandidateIssues returns eligible open issues across all configured repos.
func (c *RESTClient) FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error) {
	var all []domain.Issue
	for _, repo := range c.cfg.Repos {
		issues, err := c.fetchRepoIssues(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", repo, err)
		}
		all = append(all, issues...)
	}
	return all, nil
}

func (c *RESTClient) fetchRepoIssues(ctx context.Context, repo string) ([]domain.Issue, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", repo)
	}
	owner, name := parts[0], parts[1]

	url := fmt.Sprintf("%srepos/%s/%s/issues?state=open&per_page=100", c.baseURL, owner, name)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch issues for %s: %w", repo, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch issues for %s: %w", repo, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var raw []ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding issues: %w", err)
	}

	var issues []domain.Issue
	for _, r := range raw {
		issue := r.toDomain(repo)
		if issue.IsEligible(c.cfg.EligibleLabels, c.cfg.ExcludedLabels) {
			issues = append(issues, issue)
		}
	}
	return issues, nil
}

// FetchIssueState retrieves the current state of a single issue.
func (c *RESTClient) FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d", c.baseURL, parts[0], parts[1], id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch issue state %s#%d: %w", repo, id, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch issue state %s#%d: %w", repo, id, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch issue state %s#%d: GitHub API error %d: %s", repo, id, resp.StatusCode, body)
	}

	var raw ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("fetch issue state %s#%d: %w", repo, id, err)
	}
	issue := raw.toDomain(repo)
	return &issue, nil
}

// AddComment posts a comment on a GitHub issue.
func (c *RESTClient) AddComment(ctx context.Context, repo string, id int, body string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/comments", c.baseURL, parts[0], parts[1], id)

	payload := fmt.Sprintf(`{"body":%q}`, body)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("add comment to %s#%d: %w", repo, id, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("add comment to %s#%d: %w", repo, id, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add comment to %s#%d: GitHub API error %d: %s", repo, id, resp.StatusCode, body)
	}
	return nil
}

// AddLabel adds a label to a GitHub issue.
func (c *RESTClient) AddLabel(ctx context.Context, repo string, id int, label string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/labels", c.baseURL, parts[0], parts[1], id)

	payload := fmt.Sprintf(`{"labels":[%q]}`, label)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("add label %q to %s#%d: %w", label, repo, id, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("add label %q to %s#%d: %w", label, repo, id, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add label %q to %s#%d: GitHub API error %d: %s", label, repo, id, resp.StatusCode, body)
	}
	return nil
}

// FetchIssueStates fetches the current state for each issue.
// This is a convenience wrapper that calls FetchIssueState for each issue.
func (c *RESTClient) FetchIssueStates(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	var result []domain.Issue
	for _, iss := range issues {
		updated, err := c.FetchIssueState(ctx, iss.Repo, iss.ID)
		if err != nil {
			return nil, fmt.Errorf("fetching state for %s#%d: %w", iss.Repo, iss.ID, err)
		}
		result = append(result, *updated)
	}
	return result, nil
}

// SetProjectStatus is a no-op for the REST client.
// Project status updates require the GraphQL API.
func (c *RESTClient) SetProjectStatus(_ context.Context, _ domain.Issue, _ string) error {
	return nil
}

// EnrichIssues is a no-op for the REST client.
// Enrichment with Projects v2 fields requires the GraphQL API.
func (c *RESTClient) EnrichIssues(_ context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	return issues, nil
}

// ghComment is the raw GitHub API response shape for issue comments.
type ghComment struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
}

// FetchIssueComments retrieves all comments on a GitHub issue.
func (c *RESTClient) FetchIssueComments(ctx context.Context, repo string, id int) ([]domain.Comment, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/comments?per_page=100", c.baseURL, parts[0], parts[1], id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch comments for %s#%d: %w", repo, id, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch comments for %s#%d: %w", repo, id, err)
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

// RemoveLabel removes a label from a GitHub issue.
func (c *RESTClient) RemoveLabel(ctx context.Context, repo string, id int, label string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/labels/%s", c.baseURL, parts[0], parts[1], id, neturl.PathEscape(label))
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("remove label %q from %s#%d: %w", label, repo, id, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("remove label %q from %s#%d: %w", label, repo, id, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

// CreateIssue creates a new GitHub issue with the given title, body, and labels.
func (c *RESTClient) CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*domain.Issue, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}

	payload := map[string]any{"title": title, "body": body, "labels": labels}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("create issue in %s: %w", repo, err)
	}

	url := fmt.Sprintf("%srepos/%s/%s/issues", c.baseURL, parts[0], parts[1])
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("create issue in %s: %w", repo, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create issue in %s: %w", repo, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create issue in %s: GitHub API error %d: %s", repo, resp.StatusCode, respBody)
	}

	var raw ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("create issue in %s: %w", repo, err)
	}
	issue := raw.toDomain(repo)
	return &issue, nil
}

// AddSubIssue adds a child issue to a parent issue.
func (c *RESTClient) AddSubIssue(ctx context.Context, repo string, parentID, childID int) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	payload := fmt.Sprintf(`{"sub_issue_id":%d}`, childID)
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/sub_issues", c.baseURL, parts[0], parts[1], parentID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("add sub-issue %d to %s#%d: %w", childID, repo, parentID, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("add sub-issue %d to %s#%d: %w", childID, repo, parentID, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

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
		return nil, fmt.Errorf("get label %q from %s: %w", name, repo, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get label %q from %s: %w", name, repo, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get label %q from %s: GitHub API error %d: %s", name, repo, resp.StatusCode, body)
	}

	var label RepoLabel
	if err := json.NewDecoder(resp.Body).Decode(&label); err != nil {
		return nil, fmt.Errorf("get label %q from %s: %w", name, repo, err)
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
		return fmt.Errorf("create label %q in %s: %w", name, repo, err)
	}

	url := fmt.Sprintf("%srepos/%s/%s/labels", c.baseURL, parts[0], parts[1])
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create label %q in %s: %w", name, repo, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("create label %q in %s: %w", name, repo, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create label %q in %s: GitHub API error %d: %s", name, repo, resp.StatusCode, body)
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
		return fmt.Errorf("update label %q in %s: %w", name, repo, err)
	}

	url := fmt.Sprintf("%srepos/%s/%s/labels/%s", c.baseURL, parts[0], parts[1], neturl.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("update label %q in %s: %w", name, repo, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("update label %q in %s: %w", name, repo, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ghIssue is the raw GitHub API response shape.
type ghIssue struct {
	Number      int       `json:"number"`
	NodeID      string    `json:"node_id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	HTMLURL     string    `json:"html_url"`
	Labels      []ghLabel `json:"labels"`
	PullRequest *struct{} `json:"pull_request,omitempty"` // non-nil means this is a PR
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ghLabel struct {
	Name string `json:"name"`
}

func (g ghIssue) toDomain(repo string) domain.Issue {
	labels := make([]string, len(g.Labels))
	for i, l := range g.Labels {
		labels[i] = l.Name
	}
	status := "Todo"
	if g.State == "closed" {
		status = "Done"
	}
	return domain.Issue{
		ID:        g.Number,
		NodeID:    g.NodeID,
		Repo:      repo,
		URL:       g.HTMLURL,
		Title:     g.Title,
		Body:      g.Body,
		Labels:    normalizeLabels(labels),
		Status:    status,
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}

// FetchLabeledIssues returns all issues (open and recently closed) with the given label
// across all configured repos. Pull requests are excluded.
func (c *RESTClient) FetchLabeledIssues(ctx context.Context, label string) ([]domain.Issue, error) {
	var all []domain.Issue
	for _, repo := range c.cfg.Repos {
		for _, state := range []string{"open", "closed"} {
			issues, err := c.fetchLabeledRepoIssues(ctx, repo, label, state)
			if err != nil {
				return nil, fmt.Errorf("fetching %s %s issues from %s: %w", state, label, repo, err)
			}
			all = append(all, issues...)
		}
	}
	return all, nil
}

func (c *RESTClient) fetchLabeledRepoIssues(ctx context.Context, repo, label, state string) ([]domain.Issue, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", repo)
	}
	owner, name := parts[0], parts[1]

	url := fmt.Sprintf("%srepos/%s/%s/issues?state=%s&labels=%s&per_page=100",
		c.baseURL, owner, name, neturl.QueryEscape(state), neturl.QueryEscape(label))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch labeled issues for %s: %w", repo, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch labeled issues for %s: %w", repo, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var raw []ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding issues: %w", err)
	}

	var issues []domain.Issue
	for _, r := range raw {
		if r.PullRequest != nil {
			continue // skip PRs (GitHub lists PRs as issues)
		}
		issues = append(issues, r.toDomain(repo))
	}
	return issues, nil
}

// ghTimelineEvent represents a timeline event from the GitHub API.
type ghTimelineEvent struct {
	Event  string `json:"event"`
	Source struct {
		Issue struct {
			Number      int    `json:"number"`
			Title       string `json:"title"`
			State       string `json:"state"`
			HTMLURL     string `json:"html_url"`
			PullRequest *struct {
				MergedAt *time.Time `json:"merged_at"`
			} `json:"pull_request"`
		} `json:"issue"`
	} `json:"source"`
}

// FetchLinkedPullRequests returns pull requests linked to the given issue
// by looking at timeline cross-reference events.
func (c *RESTClient) FetchLinkedPullRequests(ctx context.Context, repo string, issueNumber int) ([]domain.PullRequest, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/timeline?per_page=100",
		c.baseURL, parts[0], parts[1], issueNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch linked PRs for %s#%d: %w", repo, issueNumber, err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch linked PRs for %s#%d: %w", repo, issueNumber, err)
	}
	defer resp.Body.Close()
	c.updateRateLimit(resp)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch linked PRs for %s#%d: GitHub API error %d: %s", repo, issueNumber, resp.StatusCode, body)
	}

	var events []ghTimelineEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("decoding timeline for %s#%d: %w", repo, issueNumber, err)
	}

	seen := map[int]bool{}
	var prs []domain.PullRequest
	for _, ev := range events {
		if ev.Event != "cross-referenced" {
			continue
		}
		src := ev.Source.Issue
		if src.PullRequest == nil {
			continue // not a PR
		}
		if seen[src.Number] {
			continue
		}
		seen[src.Number] = true

		merged := src.PullRequest.MergedAt != nil
		prs = append(prs, domain.PullRequest{
			Number: src.Number,
			Title:  src.Title,
			State:  src.State,
			Merged: merged,
			URL:    src.HTMLURL,
		})
	}
	return prs, nil
}

// Ensure RESTClient satisfies the Client interface at compile time.
var _ Client = (*RESTClient)(nil)

func normalizeLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = strings.ToLower(l)
	}
	return out
}
