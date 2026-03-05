package orchestrator

import (
	"math"
	"sync"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

const baseDelay = 10 * time.Second

func BackoffDelay(attempt int, maxBackoff time.Duration) time.Duration {
	delay := baseDelay * time.Duration(math.Pow(2, float64(attempt)))
	if delay > maxBackoff {
		delay = maxBackoff
	}
	return delay
}

type RetryQueue struct {
	mu      sync.Mutex
	entries map[int]*domain.RetryEntry
}

func NewRetryQueue() *RetryQueue {
	return &RetryQueue{
		entries: make(map[int]*domain.RetryEntry),
	}
}

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

func (q *RetryQueue) Remove(issueID int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.entries, issueID)
}

func (q *RetryQueue) Has(issueID int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.entries[issueID]
	return ok
}

func (q *RetryQueue) All() []*domain.RetryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries := make([]*domain.RetryEntry, 0, len(q.entries))
	for _, e := range q.entries {
		entries = append(entries, e)
	}
	return entries
}

func (q *RetryQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}
