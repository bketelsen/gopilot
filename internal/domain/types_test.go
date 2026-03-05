// internal/domain/types_test.go
package domain

import (
	"testing"
	"time"
)

func TestIssueIdentifier(t *testing.T) {
	issue := Issue{
		ID:   42,
		Repo: "owner/repo",
	}
	if got := issue.Identifier(); got != "owner/repo#42" {
		t.Errorf("Identifier() = %q, want %q", got, "owner/repo#42")
	}
}

func TestIssueIsTerminal(t *testing.T) {
	tests := []struct {
		status   string
		terminal bool
	}{
		{"Todo", false},
		{"In Progress", false},
		{"In Review", false},
		{"Done", true},
		{"Closed", true},
		{"Canceled", true},
	}
	for _, tt := range tests {
		issue := Issue{Status: tt.status}
		if got := issue.IsTerminal(); got != tt.terminal {
			t.Errorf("IsTerminal() for status %q = %v, want %v", tt.status, got, tt.terminal)
		}
	}
}

func TestIssueIsEligible(t *testing.T) {
	issue := Issue{
		ID:     1,
		Repo:   "owner/repo",
		Labels: []string{"gopilot"},
		Status: "Todo",
	}
	eligible := []string{"gopilot", "autopilot"}
	excluded := []string{"blocked", "needs-design"}

	if !issue.IsEligible(eligible, excluded) {
		t.Error("expected issue to be eligible")
	}

	// No eligible label
	issue.Labels = []string{"other"}
	if issue.IsEligible(eligible, excluded) {
		t.Error("expected issue without eligible label to be ineligible")
	}

	// Has excluded label
	issue.Labels = []string{"gopilot", "blocked"}
	if issue.IsEligible(eligible, excluded) {
		t.Error("expected issue with excluded label to be ineligible")
	}

	// Wrong status
	issue.Labels = []string{"gopilot"}
	issue.Status = "In Progress"
	if issue.IsEligible(eligible, excluded) {
		t.Error("expected issue with non-Todo status to be ineligible")
	}
}

func TestPrioritySort(t *testing.T) {
	now := time.Now()
	issues := []Issue{
		{ID: 1, Priority: 4, CreatedAt: now},                 // low
		{ID: 2, Priority: 1, CreatedAt: now},                 // urgent
		{ID: 3, Priority: 0, CreatedAt: now},                 // none (last)
		{ID: 4, Priority: 1, CreatedAt: now.Add(-time.Hour)}, // urgent, older
	}
	SortByPriority(issues)

	expected := []int{4, 2, 1, 3} // urgent-older, urgent-newer, low, none
	for i, want := range expected {
		if issues[i].ID != want {
			t.Errorf("position %d: got ID %d, want %d", i, issues[i].ID, want)
		}
	}
}
