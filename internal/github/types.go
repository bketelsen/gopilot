package github

import "time"

// Issue represents a normalized GitHub issue enriched with Projects v2 fields.
type Issue struct {
	// Identity
	ID     int    // GitHub issue number
	NodeID string // GitHub GraphQL node ID
	Repo   string // "owner/repo"
	URL    string // Full GitHub URL

	// Content
	Title     string
	Body      string
	Labels    []string // lowercase
	Assignees []string

	// Project fields (from Projects v2)
	Status   string // Todo, In Progress, In Review, Done
	Priority int    // 0=none, 1=urgent, 2=high, 3=medium, 4=low

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CandidateOpts configures how candidates are fetched and filtered.
type CandidateOpts struct {
	Repos          []string
	EligibleLabels []string
	ExcludedLabels []string
	ProjectMeta    *ProjectMeta
	ExcludeIDs     map[int]bool // issue IDs to skip (running/claimed)
}

// ProjectMeta holds discovered field IDs for a GitHub Project.
type ProjectMeta struct {
	ProjectID       string            // GraphQL node ID of the project
	StatusFieldID   string            // Field ID for Status
	PriorityFieldID string            // Field ID for Priority
	StatusOptions   map[string]string // "Todo" -> option_id, "In Progress" -> option_id
	PriorityOptions map[string]int    // "Urgent" -> 1, "High" -> 2
}
