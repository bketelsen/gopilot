package domain_test

import (
	"fmt"

	"github.com/bketelsen/gopilot/internal/domain"
)

func ExampleIssue_IsEligible() {
	issue := domain.Issue{
		ID:     42,
		Repo:   "owner/repo",
		Labels: []string{"gopilot", "bug"},
		Status: "Todo",
	}

	eligible := []string{"gopilot"}
	excluded := []string{"blocked"}

	fmt.Println(issue.IsEligible(eligible, excluded))

	issue.Labels = []string{"blocked"}
	fmt.Println(issue.IsEligible(eligible, excluded))
	// Output:
	// true
	// false
}

func ExampleParseBlockedBy() {
	body := `This task depends on other work.
Blocked by #10
Also blocked by #25
Unrelated text here.`

	blockers := domain.ParseBlockedBy(body)
	fmt.Println(blockers)
	// Output:
	// [10 25]
}
