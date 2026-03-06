package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/domain"
)

// ProjectMeta holds discovered Projects v2 field IDs.
type ProjectMeta struct {
	ProjectID       string
	StatusFieldID   string
	StatusOptions   map[string]string // "In Progress" -> option ID
	PriorityFieldID string
	PriorityOptions map[string]string // "Urgent" -> option ID
}

// GraphQLClient handles GitHub GraphQL API calls.
type GraphQLClient struct {
	cfg      config.GitHubConfig
	endpoint string
	http     *http.Client
	meta     *ProjectMeta
}

// NewGraphQLClient creates a GraphQL client.
func NewGraphQLClient(cfg config.GitHubConfig, endpoint string) *GraphQLClient {
	return &GraphQLClient{
		cfg:      cfg,
		endpoint: endpoint,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// DiscoverProjectFields queries the project schema to find field IDs.
func (c *GraphQLClient) DiscoverProjectFields(ctx context.Context) (*ProjectMeta, error) {
	// Build query based on owner type
	// @me -> viewer, org name -> organization, user -> user
	query := fmt.Sprintf(`{
		user(login: %q) {
			projectV2(number: %d) {
				id
				fields(first: 20) {
					nodes {
						__typename
						... on ProjectV2SingleSelectField {
							id
							name
							options { id name }
						}
						... on ProjectV2IterationField {
							id
							name
						}
						... on ProjectV2Field {
							id
							name
						}
					}
				}
			}
		}
	}`, c.cfg.Project.Owner, c.cfg.Project.Number)

	if c.cfg.Project.Owner == "@me" {
		query = fmt.Sprintf(`{
			viewer {
				projectV2(number: %d) {
					id
					fields(first: 20) {
						nodes {
							__typename
							... on ProjectV2SingleSelectField {
								id
								name
								options { id name }
							}
							... on ProjectV2IterationField {
								id
								name
							}
							... on ProjectV2Field {
								id
								name
							}
						}
					}
				}
			}
		}`, c.cfg.Project.Number)
	}

	result, err := c.execute(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("discover project fields: %w", err)
	}

	// Navigate to the projectV2 node regardless of owner type
	var projectNode map[string]any
	if data, ok := result["data"].(map[string]any); ok {
		for _, v := range data {
			if obj, ok := v.(map[string]any); ok {
				if pv2, ok := obj["projectV2"].(map[string]any); ok {
					projectNode = pv2
					break
				}
			}
		}
	}
	if projectNode == nil {
		return nil, fmt.Errorf("project not found in response")
	}

	meta := &ProjectMeta{
		ProjectID:       projectNode["id"].(string),
		StatusOptions:   make(map[string]string),
		PriorityOptions: make(map[string]string),
	}

	fields := projectNode["fields"].(map[string]any)["nodes"].([]any)
	for _, f := range fields {
		field := f.(map[string]any)
		name, _ := field["name"].(string)
		id, _ := field["id"].(string)
		typename, _ := field["__typename"].(string)

		if typename == "ProjectV2SingleSelectField" {
			options, _ := field["options"].([]any)
			optMap := make(map[string]string)
			for _, o := range options {
				opt := o.(map[string]any)
				optMap[opt["name"].(string)] = opt["id"].(string)
			}

			switch name {
			case "Status":
				meta.StatusFieldID = id
				meta.StatusOptions = optMap
			case "Priority":
				meta.PriorityFieldID = id
				meta.PriorityOptions = optMap
			}
		}
	}

	c.meta = meta
	return meta, nil
}

// SetProjectStatus sets the Status field on a project item.
func (c *GraphQLClient) SetProjectStatus(ctx context.Context, itemID string, status string) error {
	if c.meta == nil {
		return fmt.Errorf("project metadata not discovered — call DiscoverProjectFields first")
	}
	optionID, ok := c.meta.StatusOptions[status]
	if !ok {
		return fmt.Errorf("unknown status %q", status)
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

	vars := map[string]any{
		"projectId": c.meta.ProjectID,
		"itemId":    itemID,
		"fieldId":   c.meta.StatusFieldID,
		"optionId":  optionID,
	}

	_, err := c.execute(ctx, mutation, vars)
	if err != nil {
		return fmt.Errorf("set project status %q on item %s: %w", status, itemID, err)
	}
	return nil
}

// EnrichIssues enriches issues with Projects v2 data (priority, iteration, etc.).
// This is a stub — full enrichment will be implemented later.
func (c *GraphQLClient) EnrichIssues(ctx context.Context, issues []domain.Issue) ([]domain.Issue, error) {
	if issues == nil || c.meta == nil {
		return issues, nil
	}
	return issues, nil
}

func (c *GraphQLClient) execute(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("graphql marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("graphql create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql execute: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("graphql read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL error %d: %s", resp.StatusCode, respBody)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("graphql unmarshal response: %w", err)
	}

	if errs, ok := result["errors"]; ok {
		return nil, fmt.Errorf("GraphQL errors: %w", fmt.Errorf("%v", errs))
	}

	return result, nil
}
