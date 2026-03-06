package metrics

import "sync"

// TokenUsage records input and output token counts for a single interaction.
type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
}

// EstimateCost returns the estimated USD cost based on the given pricing.
func (t TokenUsage) EstimateCost(pricing ModelPricing) float64 {
	input := float64(t.InputTokens) / 1_000_000 * pricing.InputPricePerMillion
	output := float64(t.OutputTokens) / 1_000_000 * pricing.OutputPricePerMillion
	return input + output
}

// ModelPricing defines per-million-token pricing for a model.
type ModelPricing struct {
	InputPricePerMillion  float64
	OutputPricePerMillion float64
}

// DefaultPricing maps model names to their per-token pricing.
var DefaultPricing = map[string]ModelPricing{
	"claude-sonnet-4.6": {InputPricePerMillion: 3.0, OutputPricePerMillion: 15.0},
	"claude-opus-4.6":   {InputPricePerMillion: 15.0, OutputPricePerMillion: 75.0},
}

// TokenTracker accumulates token usage per issue and in aggregate.
type TokenTracker struct {
	mu       sync.Mutex
	perIssue map[int]TokenUsage
	total    TokenUsage
}

// NewTokenTracker creates an empty TokenTracker.
func NewTokenTracker() *TokenTracker {
	return &TokenTracker{
		perIssue: make(map[int]TokenUsage),
	}
}

// Record adds token usage for the given issue.
func (t *TokenTracker) Record(issueID int, usage TokenUsage) {
	t.mu.Lock()
	defer t.mu.Unlock()

	existing := t.perIssue[issueID]
	existing.InputTokens += usage.InputTokens
	existing.OutputTokens += usage.OutputTokens
	t.perIssue[issueID] = existing

	t.total.InputTokens += usage.InputTokens
	t.total.OutputTokens += usage.OutputTokens
}

// ForIssue returns the accumulated token usage for the given issue.
func (t *TokenTracker) ForIssue(issueID int) TokenUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.perIssue[issueID]
}

// Totals returns the aggregate token usage across all issues.
func (t *TokenTracker) Totals() TokenUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.total
}
