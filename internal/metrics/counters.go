package metrics

import (
	"sync"
	"time"
)

// DurationStat holds computed statistics for a set of recorded durations.
type DurationStat struct {
	Count int
	Min   time.Duration
	Max   time.Duration
	Avg   time.Duration
	Total time.Duration
}

// Counters is a thread-safe collection of named counters and duration recorders.
type Counters struct {
	mu        sync.RWMutex
	vals      map[string]int64
	durations map[string][]time.Duration
}

// NewCounters creates an empty Counters instance.
func NewCounters() *Counters {
	return &Counters{
		vals:      make(map[string]int64),
		durations: make(map[string][]time.Duration),
	}
}

// Increment adds one to the named counter.
func (c *Counters) Increment(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[name]++
}

// Add adds val to the named counter.
func (c *Counters) Add(name string, val int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[name] += val
}

// Set overwrites the named counter with val.
func (c *Counters) Set(name string, val int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[name] = val
}

// Get returns the current value of the named counter.
func (c *Counters) Get(name string) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.vals[name]
}

// All returns a snapshot of all counter values.
func (c *Counters) All() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]int64, len(c.vals))
	for k, v := range c.vals {
		out[k] = v
	}
	return out
}

// RecordDuration appends a duration sample to the named metric.
func (c *Counters) RecordDuration(name string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.durations[name] = append(c.durations[name], d)
}

// DurationStats returns computed statistics for the named duration metric.
func (c *Counters) DurationStats(name string) DurationStat {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ds := c.durations[name]
	if len(ds) == 0 {
		return DurationStat{}
	}
	stat := DurationStat{
		Count: len(ds),
		Min:   ds[0],
		Max:   ds[0],
	}
	for _, d := range ds {
		stat.Total += d
		if d < stat.Min {
			stat.Min = d
		}
		if d > stat.Max {
			stat.Max = d
		}
	}
	stat.Avg = stat.Total / time.Duration(stat.Count)
	return stat
}
