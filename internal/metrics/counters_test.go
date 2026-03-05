package metrics

import (
	"testing"
	"time"
)

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

func TestDurationStats(t *testing.T) {
	c := NewCounters()
	c.RecordDuration("session_duration", 30*time.Second)
	c.RecordDuration("session_duration", 60*time.Second)
	c.RecordDuration("session_duration", 90*time.Second)

	stats := c.DurationStats("session_duration")
	if stats.Count != 3 {
		t.Errorf("count = %d, want 3", stats.Count)
	}
	if stats.Min != 30*time.Second {
		t.Errorf("min = %v, want 30s", stats.Min)
	}
	if stats.Max != 90*time.Second {
		t.Errorf("max = %v, want 90s", stats.Max)
	}
	if stats.Avg < 59*time.Second || stats.Avg > 61*time.Second {
		t.Errorf("avg = %v, want ~60s", stats.Avg)
	}
}
