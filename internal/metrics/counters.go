package metrics

import "sync"

type Counters struct {
	mu   sync.RWMutex
	vals map[string]int64
}

func NewCounters() *Counters {
	return &Counters{vals: make(map[string]int64)}
}

func (c *Counters) Increment(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[name]++
}

func (c *Counters) Add(name string, val int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[name] += val
}

func (c *Counters) Get(name string) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.vals[name]
}

func (c *Counters) All() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]int64, len(c.vals))
	for k, v := range c.vals {
		out[k] = v
	}
	return out
}
