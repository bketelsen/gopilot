package metrics

import "sync"

type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
}

func (t TokenUsage) EstimateCost(pricing ModelPricing) float64 {
	input := float64(t.InputTokens) / 1_000_000 * pricing.InputPricePerMillion
	output := float64(t.OutputTokens) / 1_000_000 * pricing.OutputPricePerMillion
	return input + output
}

type ModelPricing struct {
	InputPricePerMillion  float64
	OutputPricePerMillion float64
}

var DefaultPricing = map[string]ModelPricing{
	"claude-sonnet-4.6": {InputPricePerMillion: 3.0, OutputPricePerMillion: 15.0},
	"claude-opus-4.6":   {InputPricePerMillion: 15.0, OutputPricePerMillion: 75.0},
}

type TokenTracker struct {
	mu       sync.Mutex
	perIssue map[int]TokenUsage
	total    TokenUsage
}

func NewTokenTracker() *TokenTracker {
	return &TokenTracker{
		perIssue: make(map[int]TokenUsage),
	}
}

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

func (t *TokenTracker) ForIssue(issueID int) TokenUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.perIssue[issueID]
}

func (t *TokenTracker) Totals() TokenUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.total
}
