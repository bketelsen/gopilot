package domain

import "sync"

// RingBuffer is a thread-safe fixed-size circular buffer of strings.
type RingBuffer struct {
	mu    sync.RWMutex
	buf   []string
	cap   int
	pos   int
	count int
}

// NewRingBuffer creates a RingBuffer that holds at most cap lines.
func NewRingBuffer(cap int) *RingBuffer {
	return &RingBuffer{
		buf: make([]string, cap),
		cap: cap,
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

// Lines returns a copy of all lines in chronological order.
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
	for i := range rb.count {
		out[i] = rb.buf[(start+i)%rb.cap]
	}
	return out
}

// Len returns the number of lines currently stored.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}
