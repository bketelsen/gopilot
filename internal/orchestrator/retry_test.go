package orchestrator

import (
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	maxBackoff := 300 * time.Second

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 20 * time.Second},
		{2, 40 * time.Second},
		{3, 80 * time.Second},
		{4, 160 * time.Second},
		{5, 300 * time.Second},
		{10, 300 * time.Second},
	}

	for _, tt := range tests {
		got := BackoffDelay(tt.attempt, maxBackoff)
		if got != tt.want {
			t.Errorf("BackoffDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestRetryQueueEnqueueAndDue(t *testing.T) {
	q := NewRetryQueue()

	q.Enqueue(42, "o/r", "o/r#42", 1, "agent crashed", 300*time.Second)
	q.Enqueue(43, "o/r", "o/r#43", 2, "timeout", 300*time.Second)

	due := q.DueEntries()
	if len(due) != 0 {
		t.Errorf("expected 0 due entries, got %d", len(due))
	}

	q.mu.Lock()
	for _, e := range q.entries {
		e.DueAt = time.Now().Add(-1 * time.Second)
	}
	q.mu.Unlock()

	due = q.DueEntries()
	if len(due) != 2 {
		t.Errorf("expected 2 due entries, got %d", len(due))
	}
}

func TestRetryEntryHasRepo(t *testing.T) {
	q := NewRetryQueue()
	q.Enqueue(1, "o/r", "o/r#1", 2, "error", 5*time.Minute)
	entries := q.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Repo != "o/r" {
		t.Errorf("repo = %q, want %q", entries[0].Repo, "o/r")
	}
}

func TestRetryQueueRemove(t *testing.T) {
	q := NewRetryQueue()
	q.Enqueue(42, "o/r", "o/r#42", 1, "err", 300*time.Second)
	q.Remove(42)

	if q.Has(42) {
		t.Error("entry should be removed")
	}
}
