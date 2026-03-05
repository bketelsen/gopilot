// Package domain defines the core types for the gopilot orchestrator.
package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

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

	// Hierarchy
	ParentID  *int  // Parent issue number (sub-issues)
	ChildIDs  []int // Child issue numbers
	BlockedBy []int // Issues blocking this one

	// Project fields (from Projects v2)
	Status    string // Todo, In Progress, In Review, Done, Closed, Canceled
	Priority  int    // 0=none, 1=urgent, 2=high, 3=medium, 4=low
	Iteration string // Sprint/iteration name
	Effort    int    // Story points

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Identifier returns the "owner/repo#N" string for logging.
func (i Issue) Identifier() string {
	return fmt.Sprintf("%s#%d", i.Repo, i.ID)
}

// IsTerminal returns true if the issue is in a terminal state.
func (i Issue) IsTerminal() bool {
	switch i.Status {
	case "Done", "Closed", "Canceled":
		return true
	}
	return false
}

// IsEligible checks whether the issue can be dispatched.
// Requires at least one eligible label, no excluded labels, and Status "Todo".
func (i Issue) IsEligible(eligible, excluded []string) bool {
	if i.Status != "Todo" {
		return false
	}

	hasEligible := false
	for _, label := range i.Labels {
		for _, el := range eligible {
			if strings.EqualFold(label, el) {
				hasEligible = true
			}
		}
		for _, ex := range excluded {
			if strings.EqualFold(label, ex) {
				return false
			}
		}
	}
	return hasEligible
}

// IsBlocked returns true if any issue in BlockedBy is not resolved.
func (i Issue) IsBlocked(resolved map[int]bool) bool {
	for _, blocker := range i.BlockedBy {
		if !resolved[blocker] {
			return true
		}
	}
	return false
}

var blockedByRegex = regexp.MustCompile(`(?i)blocked\s+by\s+#(\d+)`)

// ParseBlockedBy extracts "blocked by #N" references from issue body text.
func ParseBlockedBy(body string) []int {
	matches := blockedByRegex.FindAllStringSubmatch(body, -1)
	var ids []int
	for _, m := range matches {
		if len(m) >= 2 {
			var id int
			fmt.Sscanf(m[1], "%d", &id)
			if id > 0 {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// SortByPriority sorts issues by priority (1=urgent first, 0=none last),
// then by CreatedAt (oldest first).
func SortByPriority(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		pi, pj := issues[i].Priority, issues[j].Priority
		// 0 means "none" — sort last
		if pi == 0 && pj != 0 {
			return false
		}
		if pi != 0 && pj == 0 {
			return true
		}
		if pi != pj {
			return pi < pj
		}
		return issues[i].CreatedAt.Before(issues[j].CreatedAt)
	})
}

// RunEntry tracks an active agent session.
type RunEntry struct {
	Issue       Issue
	SessionID   string
	ProcessPID  int
	StartedAt   time.Time
	LastEventAt time.Time
	LastEvent   string
	LastMessage string
	TurnCount   int
	Attempt     int
	Tokens      TokenCounts
}

// Duration returns time since the agent started.
func (r RunEntry) Duration() time.Duration {
	return time.Since(r.StartedAt)
}

// IsStalled returns true if no events received within the timeout.
func (r RunEntry) IsStalled(timeout time.Duration) bool {
	return time.Since(r.LastEventAt) > timeout
}

// RetryEntry tracks an issue waiting for retry.
type RetryEntry struct {
	IssueID    int
	Repo       string // "owner/repo"
	Identifier string // "owner/repo#42"
	Attempt    int
	DueAt      time.Time
	Error      string
}

// TokenCounts tracks token usage for a session.
type TokenCounts struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// Add returns the sum of two TokenCounts.
func (t TokenCounts) Add(other TokenCounts) TokenCounts {
	return TokenCounts{
		InputTokens:  t.InputTokens + other.InputTokens,
		OutputTokens: t.OutputTokens + other.OutputTokens,
		TotalTokens:  t.InputTokens + other.InputTokens + t.OutputTokens + other.OutputTokens,
	}
}

// TokenTotals extends TokenCounts with aggregate metrics.
type TokenTotals struct {
	TokenCounts
	SecondsRunning float64
	CostEstimate   float64 // estimated USD
}

// AgentEvent represents an event from a running agent.
type AgentEvent struct {
	Type      string // agent_started, agent_output, agent_completed, agent_failed, agent_timeout
	SessionID string
	IssueID   int
	Message   string
	Timestamp time.Time
}
