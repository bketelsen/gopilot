package metrics

import "testing"

func TestTokenTrackerRecord(t *testing.T) {
	tracker := NewTokenTracker()

	tracker.Record(42, TokenUsage{InputTokens: 1000, OutputTokens: 500})
	tracker.Record(42, TokenUsage{InputTokens: 2000, OutputTokens: 1000})
	tracker.Record(43, TokenUsage{InputTokens: 500, OutputTokens: 250})

	issue42 := tracker.ForIssue(42)
	if issue42.InputTokens != 3000 {
		t.Errorf("issue 42 input = %d, want 3000", issue42.InputTokens)
	}
	if issue42.OutputTokens != 1500 {
		t.Errorf("issue 42 output = %d, want 1500", issue42.OutputTokens)
	}

	totals := tracker.Totals()
	if totals.InputTokens != 3500 {
		t.Errorf("total input = %d, want 3500", totals.InputTokens)
	}
}

func TestCostEstimation(t *testing.T) {
	pricing := ModelPricing{
		InputPricePerMillion:  3.0,
		OutputPricePerMillion: 15.0,
	}

	usage := TokenUsage{InputTokens: 1_000_000, OutputTokens: 100_000}
	cost := usage.EstimateCost(pricing)

	if cost < 4.4 || cost > 4.6 {
		t.Errorf("cost = %f, want ~4.5", cost)
	}
}
