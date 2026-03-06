package domain

import "sync"

// RingBuffer is a thread-safe circular buffer that stores the last N strings.
type RingBuffer struct {
	mu    sync.RWMutex
	buf   []string
	pos   int
	count int
	cap   int
}

// NewRingBuffer creates a RingBuffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([]string, capacity),
		cap: capacity,
	}
}

// Add appends a line to the buffer, evicting the oldest if full.
func (rb *RingBuffer) Add(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.buf[rb.pos] = line
	rb.pos = (rb.pos + 1) % rb.cap
	if rb.count < rb.cap {
		rb.count++
	}
}

// Lines returns a copy of all stored lines in insertion order (oldest first).
func (rb *RingBuffer) Lines() []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	if rb.count == 0 {
		return nil
	}
	out := make([]string, rb.count)
	start := 0
	if rb.count == rb.cap {
		start = rb.pos // oldest element is at pos when full
	}
	for i := 0; i < rb.count; i++ {
		out[i] = rb.buf[(start+i)%rb.cap]
	}
	return out
}

// Last returns the most recently added line, or "" if empty.
func (rb *RingBuffer) Last() string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	if rb.count == 0 {
		return ""
	}
	idx := (rb.pos - 1 + rb.cap) % rb.cap
	return rb.buf[idx]
}
