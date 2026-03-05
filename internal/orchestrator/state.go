package orchestrator

import (
	"sync"

	"github.com/bketelsen/gopilot/internal/domain"
)

// PlanningPhase represents the current phase of an interactive planning workflow.
type PlanningPhase string

const (
	PlanningPhaseDetected         PlanningPhase = "detected"
	PlanningPhaseQuestioning      PlanningPhase = "questioning"
	PlanningPhaseAwaitingReply    PlanningPhase = "awaiting_reply"
	PlanningPhasePlanProposed     PlanningPhase = "plan_proposed"
	PlanningPhaseAwaitingApproval PlanningPhase = "awaiting_approval"
	PlanningPhaseCreatingIssues   PlanningPhase = "creating_issues"
	PlanningPhaseComplete         PlanningPhase = "complete"
)

// PlanningEntry tracks the state of an interactive planning session for an issue.
type PlanningEntry struct {
	IssueID        int
	Repo           string
	Phase          PlanningPhase
	LastCommentID  int
	QuestionsAsked int
}

// State manages the orchestrator's runtime state. Thread-safe.
type State struct {
	mu        sync.RWMutex
	running   map[int]*domain.RunEntry
	claimed   map[int]bool
	retry     map[int]*domain.RetryEntry
	history   map[int][]domain.CompletedRun
	completed map[int]bool
	planning  map[int]*PlanningEntry
	totals    domain.TokenTotals
}

// NewState creates an empty state.
func NewState() *State {
	return &State{
		running:   make(map[int]*domain.RunEntry),
		claimed:   make(map[int]bool),
		retry:     make(map[int]*domain.RetryEntry),
		history:   make(map[int][]domain.CompletedRun),
		completed: make(map[int]bool),
		planning:  make(map[int]*PlanningEntry),
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

func (s *State) MarkCompleted(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed[issueID] = true
}

func (s *State) IsCompleted(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.completed[issueID]
}

func (s *State) AddHistory(issueID int, run domain.CompletedRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[issueID] = append(s.history[issueID], run)
}

func (s *State) GetHistory(issueID int) []domain.CompletedRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := s.history[issueID]
	out := make([]domain.CompletedRun, len(h))
	copy(out, h)
	return out
}

func (s *State) AddPlanning(issueID int, entry *PlanningEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planning[issueID] = entry
}

func (s *State) GetPlanning(issueID int) *PlanningEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.planning[issueID]
}

func (s *State) RemovePlanning(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.planning, issueID)
}

func (s *State) IsPlanning(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.planning[issueID]
	return ok
}

func (s *State) PlanningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.planning)
}

func (s *State) AllPlanning() []*PlanningEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*PlanningEntry, 0, len(s.planning))
	for _, e := range s.planning {
		entries = append(entries, e)
	}
	return entries
}

// StatePlanningAdapter wraps State to satisfy web.PlanningProvider.
type StatePlanningAdapter struct {
	State *State
}

// AllPlanning returns planning entries as domain types.
func (a *StatePlanningAdapter) AllPlanning() []*domain.PlanningEntry {
	return a.State.AllDomainPlanning()
}

// PlanningCount returns the number of active planning sessions.
func (a *StatePlanningAdapter) PlanningCount() int {
	return a.State.PlanningCount()
}

// AllDomainPlanning returns planning entries as domain types for the web layer.
func (s *State) AllDomainPlanning() []*domain.PlanningEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.PlanningEntry, 0, len(s.planning))
	for _, e := range s.planning {
		entries = append(entries, &domain.PlanningEntry{
			IssueID:        e.IssueID,
			Repo:           e.Repo,
			Phase:          string(e.Phase),
			QuestionsAsked: e.QuestionsAsked,
		})
	}
	return entries
}
