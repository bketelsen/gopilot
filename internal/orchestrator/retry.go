package orchestrator

import (
	"math"
	"sync"
	"time"

	"github.com/bketelsen/gopilot/internal/github"
)

// RetryEntry tracks a failed issue awaiting retry.
type RetryEntry struct {
	Issue    github.Issue
	Attempts int
	LastFail time.Time
	NextTry  time.Time
}

// RetryQueue manages exponential-backoff retries for failed agent runs.
type RetryQueue struct {
	mu             sync.Mutex
	entries        map[string]*RetryEntry // keyed by "owner/repo#number"
	maxRetries     int
	maxBackoffMS   int
	baseBackoffMS  int
}

func NewRetryQueue(maxRetries, maxBackoffMS int) *RetryQueue {
	return &RetryQueue{
		entries:       make(map[string]*RetryEntry),
		maxRetries:    maxRetries,
		maxBackoffMS:  maxBackoffMS,
		baseBackoffMS: 5000, // 5 second base
	}
}

// Add enqueues an issue for retry. Returns false if max retries exceeded.
func (q *RetryQueue) Add(issue github.Issue) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := issueKey(issue)
	entry, exists := q.entries[key]
	if !exists {
		entry = &RetryEntry{Issue: issue}
		q.entries[key] = entry
	}

	entry.Attempts++
	entry.LastFail = time.Now()

	if entry.Attempts > q.maxRetries {
		delete(q.entries, key)
		return false
	}

	// Exponential backoff: base * 2^(attempts-1), capped at max
	backoff := float64(q.baseBackoffMS) * math.Pow(2, float64(entry.Attempts-1))
	if backoff > float64(q.maxBackoffMS) {
		backoff = float64(q.maxBackoffMS)
	}
	entry.NextTry = entry.LastFail.Add(time.Duration(backoff) * time.Millisecond)

	return true
}

// Ready returns issues that are past their backoff period and ready for retry.
func (q *RetryQueue) Ready() []github.Issue {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var ready []github.Issue
	for _, entry := range q.entries {
		if now.After(entry.NextTry) {
			ready = append(ready, entry.Issue)
		}
	}
	return ready
}

// Remove removes an issue from the retry queue (e.g. on successful dispatch).
func (q *RetryQueue) Remove(issue github.Issue) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.entries, issueKey(issue))
}

// Get returns the retry entry for an issue, or nil.
func (q *RetryQueue) Get(issue github.Issue) *RetryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.entries[issueKey(issue)]
}

// Len returns the number of issues in the retry queue.
func (q *RetryQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

// All returns a snapshot of all retry entries.
func (q *RetryQueue) All() []*RetryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()

	entries := make([]*RetryEntry, 0, len(q.entries))
	for _, e := range q.entries {
		entries = append(entries, e)
	}
	return entries
}

// ExcludeIDs returns issue IDs in the retry queue for filtering.
func (q *RetryQueue) ExcludeIDs() map[int]bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	ids := make(map[int]bool, len(q.entries))
	for _, e := range q.entries {
		ids[e.Issue.ID] = true
	}
	return ids
}
