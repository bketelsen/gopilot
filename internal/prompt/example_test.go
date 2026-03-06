package prompt_test

import (
	"fmt"

	"github.com/bketelsen/gopilot/internal/domain"
	"github.com/bketelsen/gopilot/internal/prompt"
)

func ExampleRender() {
	tmpl := `Fix {{.Issue.Repo}}#{{.Issue.ID}}: {{.Issue.Title}} (attempt {{.Attempt}})`

	issue := domain.Issue{
		ID:    42,
		Repo:  "owner/repo",
		Title: "Fix login bug",
	}

	result, err := prompt.Render(tmpl, issue, 1, "")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(result)
	// Output:
	// Fix owner/repo#42: Fix login bug (attempt 1)
}
