package orchestrator

import (
	"math"
	"sync"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

const baseDelay = 10 * time.Second

// BackoffDelay calculates an exponential backoff duration for the given attempt,
// capped at maxBackoff.
func BackoffDelay(attempt int, maxBackoff time.Duration) time.Duration {
	delay := baseDelay * time.Duration(math.Pow(2, float64(attempt)))
	if delay > maxBackoff {
		delay = maxBackoff
	}
	return delay
}

// RetryQueue is a thread-safe queue of issues waiting for retry with exponential backoff.
type RetryQueue struct {
	mu      sync.Mutex
	entries map[int]*domain.RetryEntry
}

// NewRetryQueue creates an empty retry queue.
func NewRetryQueue() *RetryQueue {
	return &RetryQueue{
		entries: make(map[int]*domain.RetryEntry),
	}
}

// Enqueue adds an issue to the retry queue with a backoff delay based on the attempt number.
func (q *RetryQueue) Enqueue(issueID int, repo string, identifier string, attempt int, errMsg string, maxBackoff time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delay := BackoffDelay(attempt, maxBackoff)
	q.entries[issueID] = &domain.RetryEntry{
		IssueID:    issueID,
		Repo:       repo,
		Identifier: identifier,
		Attempt:    attempt,
		DueAt:      time.Now().Add(delay),
		Error:      errMsg,
	}
}

// DueEntries removes and returns all entries whose backoff period has elapsed.
func (q *RetryQueue) DueEntries() []*domain.RetryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var due []*domain.RetryEntry
	for id, entry := range q.entries {
		if entry.DueAt.Before(now) || entry.DueAt.Equal(now) {
			due = append(due, entry)
			delete(q.entries, id)
		}
	}
	return due
}

// Remove deletes an issue from the retry queue.
func (q *RetryQueue) Remove(issueID int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.entries, issueID)
}

// Has reports whether the given issue is in the retry queue.
func (q *RetryQueue) Has(issueID int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.entries[issueID]
	return ok
}

// All returns a snapshot of all entries in the retry queue.
func (q *RetryQueue) All() []*domain.RetryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := make([]*domain.RetryEntry, 0, len(q.entries))
	for _, e := range q.entries {
		entries = append(entries, e)
	}
	return entries
}

// Len returns the number of entries in the retry queue.
func (q *RetryQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}
