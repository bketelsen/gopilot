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

	// PR monitoring state (keyed by PR number)
	prFixes   map[int]*domain.PRFixEntry
	prRunning map[int]*domain.RunEntry
	prHistory map[int][]domain.CompletedRun
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
		prFixes:   make(map[int]*domain.PRFixEntry),
		prRunning: make(map[int]*domain.RunEntry),
		prHistory: make(map[int][]domain.CompletedRun),
	}
}

// Claim atomically claims an issue. Returns false if already claimed.
func (s *State) Claim(issueID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed[issueID] {
		return false
	}
	s.claimed[issueID] = true
	return true
}

// Release releases a previously claimed issue.
func (s *State) Release(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claimed, issueID)
}

// IsClaimed reports whether the given issue is currently claimed.
func (s *State) IsClaimed(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.claimed[issueID]
}

// AddRunning registers an active agent session for the given issue.
func (s *State) AddRunning(issueID int, entry *domain.RunEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running[issueID] = entry
}

// RemoveRunning removes an active agent session for the given issue.
func (s *State) RemoveRunning(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, issueID)
}

// GetRunning returns the active run entry for the given issue, or nil.
func (s *State) GetRunning(issueID int) *domain.RunEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running[issueID]
}

// RunningCount returns the number of currently running agents.
func (s *State) RunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running)
}

// SlotsAvailable reports whether there is capacity for another agent.
func (s *State) SlotsAvailable(max int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running) < max
}

// AllRunning returns a snapshot of all active run entries.
func (s *State) AllRunning() []*domain.RunEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.RunEntry, 0, len(s.running))
	for _, e := range s.running {
		entries = append(entries, e)
	}
	return entries
}

// AddRetry records a retry entry for the given issue.
func (s *State) AddRetry(issueID int, entry *domain.RetryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retry[issueID] = entry
}

// RemoveRetry removes the retry entry for the given issue.
func (s *State) RemoveRetry(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.retry, issueID)
}

// AllRetries returns a snapshot of all retry entries.
func (s *State) AllRetries() []*domain.RetryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.RetryEntry, 0, len(s.retry))
	for _, e := range s.retry {
		entries = append(entries, e)
	}
	return entries
}

// IsInRetryQueue reports whether the given issue is awaiting retry.
func (s *State) IsInRetryQueue(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.retry[issueID]
	return ok
}

// MarkCompleted records that an issue has been successfully completed.
func (s *State) MarkCompleted(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed[issueID] = true
}

// IsCompleted reports whether the given issue has been completed.
func (s *State) IsCompleted(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.completed[issueID]
}

// AddHistory appends a completed run to the issue's history.
func (s *State) AddHistory(issueID int, run domain.CompletedRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[issueID] = append(s.history[issueID], run)
}

// GetHistory returns a copy of the completed run history for the given issue.
func (s *State) GetHistory(issueID int) []domain.CompletedRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := s.history[issueID]
	out := make([]domain.CompletedRun, len(h))
	copy(out, h)
	return out
}

// AddPlanning registers a planning session for the given issue.
func (s *State) AddPlanning(issueID int, entry *PlanningEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planning[issueID] = entry
}

// GetPlanning returns the planning entry for the given issue, or nil.
func (s *State) GetPlanning(issueID int) *PlanningEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.planning[issueID]
}

// RemovePlanning removes the planning entry for the given issue.
func (s *State) RemovePlanning(issueID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.planning, issueID)
}

// UpdatePlanning applies fn to the planning entry for the given issue under the lock.
func (s *State) UpdatePlanning(issueID int, fn func(*PlanningEntry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.planning[issueID]; ok {
		fn(entry)
	}
}

// IsPlanning reports whether the given issue has an active planning session.
func (s *State) IsPlanning(issueID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.planning[issueID]
	return ok
}

// PlanningCount returns the number of active planning sessions.
func (s *State) PlanningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.planning)
}

// AllPlanning returns a snapshot of all planning entries.
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

// --- PR Monitoring State ---

// AddPRFix registers a PR fix entry for the given PR number.
func (s *State) AddPRFix(prNumber int, entry *domain.PRFixEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prFixes[prNumber] = entry
}

// GetPRFix returns the PR fix entry for the given PR number, or nil.
func (s *State) GetPRFix(prNumber int) *domain.PRFixEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prFixes[prNumber]
}

// RemovePRFix removes the PR fix entry for the given PR number.
func (s *State) RemovePRFix(prNumber int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.prFixes, prNumber)
}

// AllPRFixes returns a snapshot of all pending PR fix entries.
func (s *State) AllPRFixes() []*domain.PRFixEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.PRFixEntry, 0, len(s.prFixes))
	for _, e := range s.prFixes {
		entries = append(entries, e)
	}
	return entries
}

// IsPRBeingFixed reports whether the given PR has an active fix session.
func (s *State) IsPRBeingFixed(prNumber int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, running := s.prRunning[prNumber]
	_, queued := s.prFixes[prNumber]
	return running || queued
}

// AddPRRunning registers an active fix session for the given PR.
func (s *State) AddPRRunning(prNumber int, entry *domain.RunEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prRunning[prNumber] = entry
}

// RemovePRRunning removes the active fix session for the given PR.
func (s *State) RemovePRRunning(prNumber int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.prRunning, prNumber)
}

// GetPRRunning returns the active run entry for the given PR, or nil.
func (s *State) GetPRRunning(prNumber int) *domain.RunEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prRunning[prNumber]
}

// AllPRRunning returns a snapshot of all active PR fix sessions.
func (s *State) AllPRRunning() []*domain.RunEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]*domain.RunEntry, 0, len(s.prRunning))
	for _, e := range s.prRunning {
		entries = append(entries, e)
	}
	return entries
}

// PRRunningCount returns the number of active PR fix sessions.
func (s *State) PRRunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.prRunning)
}

// AddPRHistory appends a completed run to the PR's fix history.
func (s *State) AddPRHistory(prNumber int, run domain.CompletedRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prHistory[prNumber] = append(s.prHistory[prNumber], run)
}

// GetPRHistory returns a copy of the completed fix history for the given PR.
func (s *State) GetPRHistory(prNumber int) []domain.CompletedRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := s.prHistory[prNumber]
	out := make([]domain.CompletedRun, len(h))
	copy(out, h)
	return out
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
