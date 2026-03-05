package orchestrator

import (
	"testing"
	"time"

	"github.com/bketelsen/gopilot/internal/domain"
)

func TestStateClaimAndRelease(t *testing.T) {
	s := NewState()

	if !s.Claim(42) {
		t.Error("first claim should succeed")
	}
	if s.Claim(42) {
		t.Error("second claim should fail")
	}

	s.Release(42)
	if !s.Claim(42) {
		t.Error("claim after release should succeed")
	}
}

func TestStateRunning(t *testing.T) {
	s := NewState()

	entry := &domain.RunEntry{
		Issue:     domain.Issue{ID: 42, Repo: "o/r"},
		SessionID: "sess-1",
		StartedAt: time.Now(),
	}
	s.AddRunning(42, entry)

	if got := s.GetRunning(42); got != entry {
		t.Error("GetRunning should return the entry")
	}
	if s.RunningCount() != 1 {
		t.Errorf("RunningCount = %d, want 1", s.RunningCount())
	}

	s.RemoveRunning(42)
	if s.GetRunning(42) != nil {
		t.Error("GetRunning after remove should return nil")
	}
}

func TestStateSlotsAvailable(t *testing.T) {
	s := NewState()

	if !s.SlotsAvailable(3) {
		t.Error("should have slots when empty")
	}

	for i := 0; i < 3; i++ {
		s.AddRunning(i, &domain.RunEntry{})
	}
	if s.SlotsAvailable(3) {
		t.Error("should not have slots when full")
	}
}

func TestStateAllRunning(t *testing.T) {
	s := NewState()
	s.AddRunning(1, &domain.RunEntry{Issue: domain.Issue{ID: 1}})
	s.AddRunning(2, &domain.RunEntry{Issue: domain.Issue{ID: 2}})

	all := s.AllRunning()
	if len(all) != 2 {
		t.Errorf("AllRunning len = %d, want 2", len(all))
	}
}

func TestStateHistory(t *testing.T) {
	s := NewState()
	s.AddHistory(1, domain.CompletedRun{SessionID: "s1", Attempt: 1, ExitCode: 0})
	s.AddHistory(1, domain.CompletedRun{SessionID: "s2", Attempt: 2, ExitCode: 1, Error: "crashed"})

	history := s.GetHistory(1)
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].SessionID != "s1" {
		t.Errorf("first run session = %q, want s1", history[0].SessionID)
	}

	empty := s.GetHistory(999)
	if len(empty) != 0 {
		t.Errorf("expected empty history for unknown issue, got %d", len(empty))
	}
}
