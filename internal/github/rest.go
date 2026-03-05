package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// RESTClient implements GitHub REST API operations.
type RESTClient struct {
	cfg     config.GitHubConfig
	baseURL string
	http    *http.Client
}

// NewRESTClient creates a REST client. baseURL should end with "/".
func NewRESTClient(cfg config.GitHubConfig, baseURL string) *RESTClient {
	return &RESTClient{
		cfg:     cfg,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

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
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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

func (c *RESTClient) FetchIssueState(ctx context.Context, repo string, id int) (*domain.Issue, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d", c.baseURL, parts[0], parts[1], id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var raw ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	issue := raw.toDomain(repo)
	return &issue, nil
}

func (c *RESTClient) AddComment(ctx context.Context, repo string, id int, body string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/comments", c.baseURL, parts[0], parts[1], id)

	payload := fmt.Sprintf(`{"body":%q}`, body)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (c *RESTClient) AddLabel(ctx context.Context, repo string, id int, label string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("%srepos/%s/%s/issues/%d/labels", c.baseURL, parts[0], parts[1], id)

	payload := fmt.Sprintf(`{"labels":[%q]}`, label)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ghIssue is the raw GitHub API response shape.
type ghIssue struct {
	Number    int       `json:"number"`
	NodeID    string    `json:"node_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	HTMLURL   string    `json:"html_url"`
	Labels    []ghLabel `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ghLabel struct {
	Name string `json:"name"`
}

func (g ghIssue) toDomain(repo string) domain.Issue {
	labels := make([]string, len(g.Labels))
	for i, l := range g.Labels {
		labels[i] = l.Name
	}
	return domain.Issue{
		ID:        g.Number,
		NodeID:    g.NodeID,
		Repo:      repo,
		URL:       g.HTMLURL,
		Title:     g.Title,
		Body:      g.Body,
		Labels:    normalizeLabels(labels),
		Status:    "Todo",
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}

func normalizeLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = strings.ToLower(l)
	}
	return out
}
