package orchestrator

import (
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/github"
)

func TestRetryQueueAdd(t *testing.T) {
	q := NewRetryQueue(3, 300000)
	issue := github.Issue{ID: 1, Repo: "owner/repo"}

	// First three attempts should succeed
	for i := 0; i < 3; i++ {
		if !q.Add(issue) {
			t.Fatalf("attempt %d: expected Add to return true", i+1)
		}
	}

	// Fourth attempt should fail (exceeds max 3)
	if q.Add(issue) {
		t.Fatal("expected Add to return false after max retries")
	}

	// Should be removed from queue after exceeding max
	if q.Len() != 0 {
		t.Fatalf("expected empty queue, got %d", q.Len())
	}
}

func TestRetryQueueReady(t *testing.T) {
	q := NewRetryQueue(3, 300000)
	// Override base backoff to make it testable
	q.baseBackoffMS = 1 // 1ms base

	issue := github.Issue{ID: 1, Repo: "owner/repo"}
	q.Add(issue)

	// Wait for backoff to expire
	time.Sleep(10 * time.Millisecond)

	ready := q.Ready()
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready issue, got %d", len(ready))
	}
	if ready[0].ID != 1 {
		t.Fatalf("expected issue ID 1, got %d", ready[0].ID)
	}
}

func TestRetryQueueRemove(t *testing.T) {
	q := NewRetryQueue(3, 300000)
	issue := github.Issue{ID: 1, Repo: "owner/repo"}

	q.Add(issue)
	if q.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", q.Len())
	}

	q.Remove(issue)
	if q.Len() != 0 {
		t.Fatalf("expected 0 entries after remove, got %d", q.Len())
	}
}

func TestRetryQueueExponentialBackoff(t *testing.T) {
	q := NewRetryQueue(5, 300000)

	issue := github.Issue{ID: 1, Repo: "owner/repo"}

	q.Add(issue)
	entry1 := q.Get(issue)
	backoff1 := entry1.NextTry.Sub(entry1.LastFail)

	q.Add(issue)
	entry2 := q.Get(issue)
	backoff2 := entry2.NextTry.Sub(entry2.LastFail)

	// Second backoff should be roughly double the first
	if backoff2 < backoff1 {
		t.Errorf("expected increasing backoff: %v then %v", backoff1, backoff2)
	}
}
