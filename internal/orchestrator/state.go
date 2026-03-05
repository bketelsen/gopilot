package orchestrator

import (
	"sync"

	"github.com/bketelsen/gopilot/internal/domain"
)

// State manages the orchestrator's runtime state. Thread-safe.
type State struct {
	mu      sync.RWMutex
	running map[int]*domain.RunEntry
	claimed map[int]bool
	retry   map[int]*domain.RetryEntry
	totals  domain.TokenTotals
}

// NewState creates an empty state.
func NewState() *State {
	return &State{
		running: make(map[int]*domain.RunEntry),
		claimed: make(map[int]bool),
		retry:   make(map[int]*domain.RetryEntry),
	}
}

func (s *State) Claim(issueID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed[issueID] {
		return false
	}
	s.claimed[issueID] = true
	return true
}

func (s *State) Release(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claimed, issueID)
}

func (s *State) IsClaimed(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.claimed[issueID]
}

func (s *State) AddRunning(issueID int, entry *domain.RunEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running[issueID] = entry
}

func (s *State) RemoveRunning(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, issueID)
}

func (s *State) GetRunning(issueID int) *domain.RunEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running[issueID]
}

func (s *State) RunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running)
}

func (s *State) SlotsAvailable(max int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running) < max
}

func (s *State) AllRunning() []*domain.RunEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.RunEntry, 0, len(s.running))
	for _, e := range s.running {
		entries = append(entries, e)
	}
	return entries
}

func (s *State) AddRetry(issueID int, entry *domain.RetryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retry[issueID] = entry
}

func (s *State) RemoveRetry(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.retry, issueID)
}

func (s *State) AllRetries() []*domain.RetryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.RetryEntry, 0, len(s.retry))
	for _, e := range s.retry {
		entries = append(entries, e)
	}
	return entries
}

func (s *State) IsInRetryQueue(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.retry[issueID]
	return ok
}
