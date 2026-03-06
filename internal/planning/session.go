package planning

import (
	"sync"
	"time"
)

// Status represents the lifecycle state of a planning session.
type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

// ChatMessage is a single message in a planning conversation.
type ChatMessage struct {
	Role      string    `json:"role"` // "user" or "agent"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Session holds the state and message history of a planning session.
type Session struct {
	ID          string        `json:"id"`
	Repo        string        `json:"repo"`
	LinkedIssue *int          `json:"linked_issue,omitempty"`
	Status      Status        `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	Messages    []ChatMessage `json:"messages"`

	mu sync.Mutex // protects Messages and Status
}

// AddMessage appends a message to the session's conversation history.
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ChatMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

// SetStatus updates the session's lifecycle status.
func (s *Session) SetStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
}
