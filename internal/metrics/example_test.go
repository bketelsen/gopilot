package metrics_test

import (
	"fmt"

	"github.com/bketelsen/gopilot/internal/metrics"
)

func ExampleTokenTracker_Record() {
	tracker := metrics.NewTokenTracker()

	tracker.Record(42, metrics.TokenUsage{InputTokens: 1000, OutputTokens: 500})
	tracker.Record(42, metrics.TokenUsage{InputTokens: 2000, OutputTokens: 1000})
	tracker.Record(99, metrics.TokenUsage{InputTokens: 500, OutputTokens: 250})

	issue42 := tracker.ForIssue(42)
	fmt.Printf("Issue 42: input=%d output=%d\n", issue42.InputTokens, issue42.OutputTokens)

	totals := tracker.Totals()
	fmt.Printf("Totals: input=%d output=%d\n", totals.InputTokens, totals.OutputTokens)
	// Output:
	// Issue 42: input=3000 output=1500
	// Totals: input=3500 output=1750
}
