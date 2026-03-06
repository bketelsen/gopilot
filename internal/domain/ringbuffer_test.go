package domain

import (
	"fmt"
	"sync"
	"testing"
)

func TestRingBufferAdd(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Add("line1")
	rb.Add("line2")

	lines := rb.Lines()
	if len(lines) != 2 {
		t.Fatalf("len = %d, want 2", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" {
		t.Errorf("lines = %v, want [line1 line2]", lines)
	}
}

func TestRingBufferOverflow(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Add("a")
	rb.Add("b")
	rb.Add("c")
	rb.Add("d") // overflows, "a" evicted

	lines := rb.Lines()
	if len(lines) != 3 {
		t.Fatalf("len = %d, want 3", len(lines))
	}
	want := []string{"b", "c", "d"}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("lines[%d] = %q, want %q", i, lines[i], w)
		}
	}
}

func TestRingBufferOverflowMultipleWraps(t *testing.T) {
	rb := NewRingBuffer(2)
	for i := 0; i < 10; i++ {
		rb.Add(fmt.Sprintf("line%d", i))
	}

	lines := rb.Lines()
	if len(lines) != 2 {
		t.Fatalf("len = %d, want 2", len(lines))
	}
	if lines[0] != "line8" || lines[1] != "line9" {
		t.Errorf("lines = %v, want [line8 line9]", lines)
	}
}

func TestRingBufferEmpty(t *testing.T) {
	rb := NewRingBuffer(5)
	lines := rb.Lines()
	if len(lines) != 0 {
		t.Errorf("len = %d, want 0", len(lines))
	}
}

func TestRingBufferLast(t *testing.T) {
	rb := NewRingBuffer(5)
	if rb.Last() != "" {
		t.Errorf("Last() on empty = %q, want empty", rb.Last())
	}

	rb.Add("first")
	rb.Add("second")
	if got := rb.Last(); got != "second" {
		t.Errorf("Last() = %q, want %q", got, "second")
	}
}

func TestRingBufferConcurrent(t *testing.T) {
	rb := NewRingBuffer(50)
	var wg sync.WaitGroup

	// 10 goroutines each adding 100 lines
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				rb.Add(fmt.Sprintf("g%d-line%d", id, i))
			}
		}(g)
	}

	// Concurrent reads
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = rb.Lines()
				_ = rb.Last()
			}
		}()
	}

	wg.Wait()

	lines := rb.Lines()
	if len(lines) != 50 {
		t.Errorf("len = %d, want 50", len(lines))
	}
}
