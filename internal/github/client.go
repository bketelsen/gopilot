package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	gh "github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

// Client wraps GitHub REST and GraphQL access.
type Client struct {
	rest  *gh.Client
	http  *http.Client
	token string
}

// NewClient creates a new GitHub client authenticated with the given token.
func NewClient(token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), ts)
	return &Client{
		rest:  gh.NewClient(httpClient),
		http:  httpClient,
		token: token,
	}
}

// FetchIssues retrieves open issues from a repo that have any of the given labels.
func (c *Client) FetchIssues(ctx context.Context, owner, repo string, labels []string) ([]Issue, error) {
	var allIssues []Issue

	opts := &gh.IssueListByRepoOptions{
		State:  "open",
		Labels: labels,
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}

	for {
		issues, resp, err := c.rest.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list issues %s/%s: %w", owner, repo, err)
		}

		for _, issue := range issues {
			// Skip pull requests (GitHub API returns them as issues too)
			if issue.IsPullRequest() {
				continue
			}
			allIssues = append(allIssues, normalizeIssue(issue, owner, repo))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allIssues, nil
}

// FetchIssue retrieves a single issue by number.
func (c *Client) FetchIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	issue, _, err := c.rest.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("get issue %s/%s#%d: %w", owner, repo, number, err)
	}
	normalized := normalizeIssue(issue, owner, repo)
	return &normalized, nil
}

// AddComment posts a comment on an issue.
func (c *Client) AddComment(ctx context.Context, owner, repo string, number int, body string) error {
	_, _, err := c.rest.Issues.CreateComment(ctx, owner, repo, number, &gh.IssueComment{
		Body: gh.Ptr(body),
	})
	if err != nil {
		return fmt.Errorf("comment on %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

func normalizeIssue(issue *gh.Issue, owner, repo string) Issue {
	i := Issue{
		ID:     issue.GetNumber(),
		NodeID: issue.GetNodeID(),
		Repo:   owner + "/" + repo,
		URL:    issue.GetHTMLURL(),
		Title:  issue.GetTitle(),
		Body:   issue.GetBody(),
	}

	for _, l := range issue.Labels {
		i.Labels = append(i.Labels, strings.ToLower(l.GetName()))
	}

	for _, a := range issue.Assignees {
		i.Assignees = append(i.Assignees, a.GetLogin())
	}

	if issue.CreatedAt != nil {
		i.CreatedAt = issue.CreatedAt.Time
	}
	if issue.UpdatedAt != nil {
		i.UpdatedAt = issue.UpdatedAt.Time
	}

	return i
}

// graphqlRequest executes a GraphQL query against the GitHub API.
func (c *Client) graphqlRequest(ctx context.Context, query string, variables map[string]interface{}) (json.RawMessage, error) {
	body := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/graphql", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create graphql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read graphql response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql response status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal graphql response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql errors: %s", result.Errors[0].Message)
	}

	return result.Data, nil
}

// FetchCandidates fetches and filters eligible issues across all configured repos.
// Returns issues sorted by priority (ascending: 1=urgent first) then created_at (oldest first).
func (c *Client) FetchCandidates(ctx context.Context, opts CandidateOpts) ([]Issue, error) {
	var candidates []Issue

	for _, repoStr := range opts.Repos {
		parts := strings.SplitN(repoStr, "/", 2)
		if len(parts) != 2 {
			slog.Warn("invalid repo format, skipping", "repo", repoStr)
			continue
		}
		owner, repo := parts[0], parts[1]

		issues, err := c.FetchIssues(ctx, owner, repo, opts.EligibleLabels)
		if err != nil {
			slog.Error("fetch issues failed", "repo", repoStr, "error", err)
			continue
		}

		for _, issue := range issues {
			if !isEligible(issue, opts) {
				continue
			}

			// Enrich with project fields if project meta is available
			if opts.ProjectMeta != nil {
				status, priority, err := c.FetchProjectFields(ctx, opts.ProjectMeta, issue.NodeID)
				if err != nil {
					slog.Debug("fetch project fields failed", "issue", fmt.Sprintf("%s#%d", issue.Repo, issue.ID), "error", err)
					continue
				}
				issue.Status = status
				issue.Priority = priority

				// Only dispatch issues with status "Todo"
				if issue.Status != "Todo" {
					continue
				}
			}

			candidates = append(candidates, issue)
		}
	}

	sortCandidates(candidates)
	return candidates, nil
}

func isEligible(issue Issue, opts CandidateOpts) bool {
	// Skip if already running or claimed
	if opts.ExcludeIDs != nil && opts.ExcludeIDs[issue.ID] {
		return false
	}

	// Must have at least one eligible label
	hasEligible := false
	for _, el := range opts.EligibleLabels {
		for _, il := range issue.Labels {
			if strings.EqualFold(el, il) {
				hasEligible = true
				break
			}
		}
		if hasEligible {
			break
		}
	}
	if !hasEligible {
		return false
	}

	// Must not have any excluded label
	for _, xl := range opts.ExcludedLabels {
		for _, il := range issue.Labels {
			if strings.EqualFold(xl, il) {
				return false
			}
		}
	}

	return true
}

func sortCandidates(issues []Issue) {
	// Sort by priority ascending (1=urgent first, 0=none treated as 5/last),
	// then by created_at ascending (oldest first)
	for i := 1; i < len(issues); i++ {
		for j := i; j > 0; j-- {
			pi := prioritySortKey(issues[j].Priority)
			pj := prioritySortKey(issues[j-1].Priority)
			if pi < pj || (pi == pj && issues[j].CreatedAt.Before(issues[j-1].CreatedAt)) {
				issues[j], issues[j-1] = issues[j-1], issues[j]
			} else {
				break
			}
		}
	}
}

func prioritySortKey(p int) int {
	if p == 0 {
		return 5 // no priority sorts last
	}
	return p
}

// FetchProjectFields retrieves the Status and Priority for an issue in a project.
func (c *Client) FetchProjectFields(ctx context.Context, meta *ProjectMeta, issueNodeID string) (status string, priority int, err error) {
	if meta == nil {
		return "", 0, nil
	}

	query := `query($nodeId: ID!) {
		node(id: $nodeId) {
			... on Issue {
				projectItems(first: 10) {
					nodes {
						project { id }
						fieldValues(first: 20) {
							nodes {
								... on ProjectV2ItemFieldSingleSelectValue {
									field { ... on ProjectV2SingleSelectField { id name } }
									name
								}
							}
						}
					}
				}
			}
		}
	}`

	data, err := c.graphqlRequest(ctx, query, map[string]interface{}{
		"nodeId": issueNodeID,
	})
	if err != nil {
		return "", 0, err
	}

	var result struct {
		Node struct {
			ProjectItems struct {
				Nodes []struct {
					Project struct {
						ID string `json:"id"`
					} `json:"project"`
					FieldValues struct {
						Nodes []struct {
							Field struct {
								ID   string `json:"id"`
								Name string `json:"name"`
							} `json:"field"`
							Name string `json:"name"`
						} `json:"nodes"`
					} `json:"fieldValues"`
				} `json:"nodes"`
			} `json:"projectItems"`
		} `json:"node"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return "", 0, fmt.Errorf("unmarshal project fields: %w", err)
	}

	for _, item := range result.Node.ProjectItems.Nodes {
		if item.Project.ID != meta.ProjectID {
			continue
		}
		for _, fv := range item.FieldValues.Nodes {
			if fv.Field.ID == meta.StatusFieldID {
				status = fv.Name
			}
			if fv.Field.ID == meta.PriorityFieldID {
				if p, ok := meta.PriorityOptions[fv.Name]; ok {
					priority = p
				}
			}
		}
	}

	return status, priority, nil
}

// DiscoverProject fetches the project's field metadata (IDs, option values).
func (c *Client) DiscoverProject(ctx context.Context, owner string, projectNumber int) (*ProjectMeta, error) {
	// Determine if owner is a user or org
	ownerType := "user"
	if owner != "@me" {
		// Check if it's an org
		_, _, err := c.rest.Organizations.Get(ctx, owner)
		if err == nil {
			ownerType = "organization"
		}
	}

	var query string
	if owner == "@me" {
		query = fmt.Sprintf(`query {
			viewer {
				projectV2(number: %d) {
					id
					fields(first: 50) {
						nodes {
							... on ProjectV2SingleSelectField {
								id
								name
								options { id name }
							}
						}
					}
				}
			}
		}`, projectNumber)
	} else if ownerType == "organization" {
		query = fmt.Sprintf(`query {
			organization(login: %q) {
				projectV2(number: %d) {
					id
					fields(first: 50) {
						nodes {
							... on ProjectV2SingleSelectField {
								id
								name
								options { id name }
							}
						}
					}
				}
			}
		}`, owner, projectNumber)
	} else {
		query = fmt.Sprintf(`query {
			user(login: %q) {
				projectV2(number: %d) {
					id
					fields(first: 50) {
						nodes {
							... on ProjectV2SingleSelectField {
								id
								name
								options { id name }
							}
						}
					}
				}
			}
		}`, owner, projectNumber)
	}

	data, err := c.graphqlRequest(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("discover project: %w", err)
	}

	// Parse the response — the project object is nested under viewer/user/organization
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal discover response: %w", err)
	}

	// Find the projectV2 object regardless of the owner type
	var projectJSON json.RawMessage
	for _, v := range raw {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(v, &inner); err != nil {
			continue
		}
		if pj, ok := inner["projectV2"]; ok {
			projectJSON = pj
			break
		}
	}

	if projectJSON == nil {
		return nil, fmt.Errorf("project #%d not found for owner %q", projectNumber, owner)
	}

	var project struct {
		ID     string `json:"id"`
		Fields struct {
			Nodes []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Options []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"options"`
			} `json:"nodes"`
		} `json:"fields"`
	}

	if err := json.Unmarshal(projectJSON, &project); err != nil {
		return nil, fmt.Errorf("unmarshal project fields: %w", err)
	}

	meta := &ProjectMeta{
		ProjectID:       project.ID,
		StatusOptions:   make(map[string]string),
		PriorityOptions: make(map[string]int),
	}

	priorityMap := map[string]int{
		"Urgent": 1, "High": 2, "Medium": 3, "Low": 4,
		// Also handle P1-P4 style
		"P1": 1, "P2": 2, "P3": 3, "P4": 4,
	}

	for _, field := range project.Fields.Nodes {
		if field.Name == "" {
			continue
		}

		switch strings.ToLower(field.Name) {
		case "status":
			meta.StatusFieldID = field.ID
			for _, opt := range field.Options {
				meta.StatusOptions[opt.Name] = opt.ID
			}
		case "priority":
			meta.PriorityFieldID = field.ID
			for _, opt := range field.Options {
				if p, ok := priorityMap[opt.Name]; ok {
					meta.PriorityOptions[opt.Name] = p
				}
			}
		}
	}

	if meta.StatusFieldID == "" {
		slog.Warn("no Status field found in project", "project_id", project.ID)
	}

	slog.Info("discovered project",
		"project_id", project.ID,
		"status_field", meta.StatusFieldID,
		"priority_field", meta.PriorityFieldID,
		"status_options", len(meta.StatusOptions),
	)

	return meta, nil
}

// SetProjectStatus updates an issue's Status field in the project.
func (c *Client) SetProjectStatus(ctx context.Context, meta *ProjectMeta, issueNodeID string, status string) error {
	if meta == nil || meta.StatusFieldID == "" {
		return fmt.Errorf("no status field configured")
	}

	optionID, ok := meta.StatusOptions[status]
	if !ok {
		return fmt.Errorf("unknown status %q (available: %v)", status, mapKeys(meta.StatusOptions))
	}

	// First, find the project item ID for this issue
	itemID, err := c.findProjectItemID(ctx, meta.ProjectID, issueNodeID)
	if err != nil {
		return fmt.Errorf("find project item: %w", err)
	}

	mutation := `mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String!) {
		updateProjectV2ItemFieldValue(input: {
			projectId: $projectId
			itemId: $itemId
			fieldId: $fieldId
			value: { singleSelectOptionId: $optionId }
		}) {
			projectV2Item { id }
		}
	}`

	_, err = c.graphqlRequest(ctx, mutation, map[string]interface{}{
		"projectId": meta.ProjectID,
		"itemId":    itemID,
		"fieldId":   meta.StatusFieldID,
		"optionId":  optionID,
	})
	if err != nil {
		return fmt.Errorf("set status to %q: %w", status, err)
	}

	return nil
}

func (c *Client) findProjectItemID(ctx context.Context, projectID, issueNodeID string) (string, error) {
	query := `query($nodeId: ID!) {
		node(id: $nodeId) {
			... on Issue {
				projectItems(first: 10) {
					nodes {
						id
						project { id }
					}
				}
			}
		}
	}`

	data, err := c.graphqlRequest(ctx, query, map[string]interface{}{
		"nodeId": issueNodeID,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		Node struct {
			ProjectItems struct {
				Nodes []struct {
					ID      string `json:"id"`
					Project struct {
						ID string `json:"id"`
					} `json:"project"`
				} `json:"nodes"`
			} `json:"projectItems"`
		} `json:"node"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("unmarshal project items: %w", err)
	}

	for _, item := range result.Node.ProjectItems.Nodes {
		if item.Project.ID == projectID {
			return item.ID, nil
		}
	}

	return "", fmt.Errorf("issue not found in project %s", projectID)
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// AddLabel adds a label to an issue.
func (c *Client) AddLabel(ctx context.Context, owner, repo string, number int, label string) error {
	_, _, err := c.rest.Issues.AddLabelsToIssue(ctx, owner, repo, number, []string{label})
	if err != nil {
		return fmt.Errorf("add label %q to %s/%s#%d: %w", label, owner, repo, number, err)
	}
	return nil
}

// Ensure time import is used
var _ = time.Now
