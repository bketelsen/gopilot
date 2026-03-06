package planning

import (
	"sync"
	"time"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type ChatMessage struct {
	Role      string    `json:"role"` // "user" or "agent"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Session struct {
	ID          string        `json:"id"`
	Repo        string        `json:"repo"`
	LinkedIssue *int          `json:"linked_issue,omitempty"`
	Status      Status        `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	Messages    []ChatMessage `json:"messages"`

	mu sync.Mutex // protects Messages and Status
}

func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ChatMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

func (s *Session) SetStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
}
