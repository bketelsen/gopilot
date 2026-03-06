package planning_test

import (
	"testing"

	"github.com/bketelsen/gopilot/internal/planning"
)

func TestManager_CreateAndGet(t *testing.T) {
	m := planning.NewManager()
	sess, err := m.Create("owner/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.Repo != "owner/repo" {
		t.Errorf("expected repo owner/repo, got %s", sess.Repo)
	}
	if sess.Status != planning.StatusPending {
		t.Errorf("expected status pending, got %s", sess.Status)
	}

	got := m.Get(sess.ID)
	if got == nil {
		t.Fatal("expected to find session")
	}
	if got.ID != sess.ID {
		t.Errorf("expected ID %s, got %s", sess.ID, got.ID)
	}
}

func TestManager_CreateWithLinkedIssue(t *testing.T) {
	m := planning.NewManager()
	issueID := 42
	sess, err := m.Create("owner/repo", &issueID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.LinkedIssue == nil || *sess.LinkedIssue != 42 {
		t.Errorf("expected linked issue 42, got %v", sess.LinkedIssue)
	}
}

func TestManager_List(t *testing.T) {
	m := planning.NewManager()
	m.Create("owner/repo1", nil)
	m.Create("owner/repo2", nil)

	sessions := m.List()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestManager_Close(t *testing.T) {
	m := planning.NewManager()
	sess, _ := m.Create("owner/repo", nil)
	m.Close(sess.ID)

	got := m.Get(sess.ID)
	if got.Status != planning.StatusDone {
		t.Errorf("expected status done, got %s", got.Status)
	}
}

func TestSession_AddMessage(t *testing.T) {
	m := planning.NewManager()
	sess, _ := m.Create("owner/repo", nil)
	sess.AddMessage("user", "hello")
	sess.AddMessage("agent", "hi there")

	if len(sess.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sess.Messages))
	}
	if sess.Messages[0].Role != "user" || sess.Messages[0].Content != "hello" {
		t.Errorf("unexpected first message: %+v", sess.Messages[0])
	}
}
