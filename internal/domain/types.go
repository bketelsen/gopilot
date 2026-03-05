// Package domain defines the core types for the gopilot orchestrator.
package domain

import (
	"fmt"
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
