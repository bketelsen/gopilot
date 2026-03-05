package orchestrator

import (
	"sync"
	"testing"

	"github.com/bketelsen/gopilot/internal/github"
)

func testIssue(id int) github.Issue {
	return github.Issue{ID: id, Repo: "owner/repo"}
}

func TestClaimAndRelease(t *testing.T) {
	s := NewState()
	issue := testIssue(1)

	if !s.Claim(issue) {
		t.Fatal("first claim should succeed")
	}
	if s.Claim(issue) {
		t.Fatal("second claim should fail")
	}

	s.ReleaseClaim(issue)
	if !s.Claim(issue) {
		t.Fatal("claim after release should succeed")
	}
}

func TestRunningCount(t *testing.T) {
	s := NewState()
	if s.RunningCount() != 0 {
		t.Fatal("initial count should be 0")
	}

	issue := testIssue(1)
	s.Claim(issue)
	s.AddRunning(issue, nil)

	if s.RunningCount() != 1 {
		t.Fatalf("running count = %d, want 1", s.RunningCount())
	}

	s.RemoveRunning(issue)
	if s.RunningCount() != 0 {
		t.Fatalf("running count = %d, want 0", s.RunningCount())
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewState()
	var wg sync.WaitGroup

	// Hammer the state from multiple goroutines
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			issue := testIssue(id)
			s.Claim(issue)
			s.IsRunningOrClaimed(issue)
			s.RunningCount()
			s.ReleaseClaim(issue)
		}(i)
	}

	wg.Wait()

	if s.RunningCount() != 0 {
		t.Fatalf("expected 0 running after all released, got %d", s.RunningCount())
	}
}
