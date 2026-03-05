package orchestrator

import (
	"sync"
	"time"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/github"
)

// RunEntry tracks an in-flight agent run.
type RunEntry struct {
	Issue     github.Issue
	Session   *agent.Session
	StartedAt time.Time
}

// State manages in-memory tracking of claimed and running issues.
// All methods are safe for concurrent use.
type State struct {
	mu       sync.Mutex
	running  map[string]*RunEntry // keyed by "owner/repo#number"
	claimed  map[string]bool      // same key format
}

func NewState() *State {
	return &State{
		running: make(map[string]*RunEntry),
		claimed: make(map[string]bool),
	}
}

func issueKey(issue github.Issue) string {
	return issue.Repo + "#" + itoa(issue.ID)
}

// Claim marks an issue as claimed for dispatch. Returns false if already claimed/running.
func (s *State) Claim(issue github.Issue) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := issueKey(issue)
	if s.claimed[key] || s.running[key] != nil {
		return false
	}
	s.claimed[key] = true
	return true
}

// ReleaseClaim removes a claim (e.g. if dispatch fails).
func (s *State) ReleaseClaim(issue github.Issue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claimed, issueKey(issue))
}

// AddRunning transitions an issue from claimed to running.
func (s *State) AddRunning(issue github.Issue, session *agent.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := issueKey(issue)
	delete(s.claimed, key)
	s.running[key] = &RunEntry{
		Issue:     issue,
		Session:   session,
		StartedAt: time.Now(),
	}
}

// RemoveRunning removes a completed run.
func (s *State) RemoveRunning(issue github.Issue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, issueKey(issue))
}

// GetRunning returns the run entry for an issue, or nil.
func (s *State) GetRunning(issue github.Issue) *RunEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[issueKey(issue)]
}

// AllRunning returns a snapshot of all running entries.
func (s *State) AllRunning() []*RunEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := make([]*RunEntry, 0, len(s.running))
	for _, e := range s.running {
		entries = append(entries, e)
	}
	return entries
}

// RunningCount returns the number of currently running agents.
func (s *State) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}

// IsRunningOrClaimed checks if an issue is currently running or claimed.
func (s *State) IsRunningOrClaimed(issue github.Issue) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := issueKey(issue)
	return s.claimed[key] || s.running[key] != nil
}

// RunningIssueIDs returns a map of issue IDs that are running or claimed.
func (s *State) RunningIssueIDs() map[int]bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make(map[int]bool, len(s.running)+len(s.claimed))
	for _, e := range s.running {
		ids[e.Issue.ID] = true
	}
	// claimed issues don't have entries in running, so we can't get their IDs
	// directly. We'll handle this through IsRunningOrClaimed instead.
	return ids
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
