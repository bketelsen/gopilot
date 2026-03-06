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

func TestRunEntryDuration(t *testing.T) {
	entry := RunEntry{
		StartedAt: time.Now().Add(-5 * time.Minute),
	}
	d := entry.Duration()
	if d < 4*time.Minute || d > 6*time.Minute {
		t.Errorf("Duration() = %v, want ~5m", d)
	}
}

func TestRunEntryIsStalled(t *testing.T) {
	timeout := 5 * time.Minute
	fresh := RunEntry{LastEventAt: time.Now()}
	if fresh.IsStalled(timeout) {
		t.Error("fresh entry should not be stalled")
	}

	stale := RunEntry{LastEventAt: time.Now().Add(-10 * time.Minute)}
	if !stale.IsStalled(timeout) {
		t.Error("stale entry should be stalled")
	}
}

func TestIsBlockedBy(t *testing.T) {
	issue := Issue{
		ID:        3,
		BlockedBy: []int{1, 2},
		Status:    "Todo",
		Labels:    []string{"gopilot"},
	}

	resolved := map[int]bool{1: true, 2: true}
	if issue.IsBlocked(resolved) {
		t.Error("should not be blocked when all blockers are resolved")
	}

	partial := map[int]bool{1: true, 2: false}
	if !issue.IsBlocked(partial) {
		t.Error("should be blocked when some blockers are unresolved")
	}
}

func TestParseBlockedByFromBody(t *testing.T) {
	body := `This feature depends on:
- blocked by #42
- Blocked By #99
Some other text.`

	blockers := ParseBlockedBy(body)
	if len(blockers) != 2 {
		t.Fatalf("got %d blockers, want 2", len(blockers))
	}
	if blockers[0] != 42 || blockers[1] != 99 {
		t.Errorf("blockers = %v, want [42, 99]", blockers)
	}
}

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

func TestIssueHasOpenPR(t *testing.T) {
	tests := []struct {
		name      string
		linkedPRs []PullRequest
		want      bool
	}{
		{"no PRs", nil, false},
		{"open PR", []PullRequest{{Number: 1, State: "open"}}, true},
		{"closed PR", []PullRequest{{Number: 1, State: "closed"}}, false},
		{"merged PR", []PullRequest{{Number: 1, State: "closed", Merged: true}}, false},
		{"mixed", []PullRequest{{Number: 1, State: "closed", Merged: true}, {Number: 2, State: "open"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := Issue{LinkedPRs: tt.linkedPRs}
			if got := issue.HasOpenPR(); got != tt.want {
				t.Errorf("HasOpenPR() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIssueHasMergedPR(t *testing.T) {
	tests := []struct {
		name      string
		linkedPRs []PullRequest
		want      bool
	}{
		{"no PRs", nil, false},
		{"open PR", []PullRequest{{Number: 1, State: "open"}}, false},
		{"closed unmerged PR", []PullRequest{{Number: 1, State: "closed"}}, false},
		{"merged PR", []PullRequest{{Number: 1, State: "closed", Merged: true}}, true},
		{"mixed with merged", []PullRequest{{Number: 1, State: "open"}, {Number: 2, State: "closed", Merged: true}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := Issue{LinkedPRs: tt.linkedPRs}
			if got := issue.HasMergedPR(); got != tt.want {
				t.Errorf("HasMergedPR() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenCountsAdd(t *testing.T) {
	a := TokenCounts{InputTokens: 100, OutputTokens: 50}
	b := TokenCounts{InputTokens: 200, OutputTokens: 100}
	sum := a.Add(b)
	if sum.InputTokens != 300 || sum.OutputTokens != 150 || sum.TotalTokens != 450 {
		t.Errorf("Add() = %+v, want {300, 150, 450}", sum)
	}
}

func TestPullRequestIdentifier(t *testing.T) {
	pr := PullRequest{Number: 22, Repo: "owner/repo"}
	if got := pr.Identifier(); got != "owner/repo#22" {
		t.Errorf("Identifier() = %q, want %q", got, "owner/repo#22")
	}
}

func TestPullRequestHasFailedChecks(t *testing.T) {
	tests := []struct {
		name      string
		checkRuns []CheckRun
		want      bool
	}{
		{"no checks", nil, false},
		{"all passing", []CheckRun{
			{Name: "test", Status: "completed", Conclusion: "success"},
		}, false},
		{"one failure", []CheckRun{
			{Name: "test", Status: "completed", Conclusion: "success"},
			{Name: "lint", Status: "completed", Conclusion: "failure"},
		}, true},
		{"in progress ignored", []CheckRun{
			{Name: "test", Status: "in_progress", Conclusion: ""},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := PullRequest{CheckRuns: tt.checkRuns}
			if got := pr.HasFailedChecks(); got != tt.want {
				t.Errorf("HasFailedChecks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPullRequestFailedCheckRuns(t *testing.T) {
	pr := PullRequest{
		CheckRuns: []CheckRun{
			{Name: "test", Status: "completed", Conclusion: "success"},
			{Name: "lint", Status: "completed", Conclusion: "failure"},
			{Name: "build", Status: "completed", Conclusion: "failure"},
		},
	}
	failed := pr.FailedCheckRuns()
	if len(failed) != 2 {
		t.Fatalf("FailedCheckRuns() returned %d, want 2", len(failed))
	}
	if failed[0].Name != "lint" || failed[1].Name != "build" {
		t.Errorf("FailedCheckRuns() = %v, want [lint, build]", failed)
	}
}

func TestPullRequestChecksComplete(t *testing.T) {
	tests := []struct {
		name      string
		checkRuns []CheckRun
		want      bool
	}{
		{"no checks", nil, false},
		{"all completed", []CheckRun{
			{Status: "completed", Conclusion: "success"},
		}, true},
		{"some in progress", []CheckRun{
			{Status: "completed", Conclusion: "success"},
			{Status: "in_progress"},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := PullRequest{CheckRuns: tt.checkRuns}
			if got := pr.ChecksComplete(); got != tt.want {
				t.Errorf("ChecksComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}
