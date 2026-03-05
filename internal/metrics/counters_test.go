package metrics

import "testing"

func TestCounters(t *testing.T) {
	c := NewCounters()

	c.Increment("issues_dispatched")
	c.Increment("issues_dispatched")
	c.Increment("issues_completed")

	if c.Get("issues_dispatched") != 2 {
		t.Errorf("dispatched = %d, want 2", c.Get("issues_dispatched"))
	}
	if c.Get("issues_completed") != 1 {
		t.Errorf("completed = %d, want 1", c.Get("issues_completed"))
	}
	if c.Get("nonexistent") != 0 {
		t.Error("nonexistent counter should be 0")
	}

	all := c.All()
	if len(all) != 2 {
		t.Errorf("All() len = %d, want 2", len(all))
	}
}
