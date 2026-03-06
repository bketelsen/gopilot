package domain

import (
	"testing"
)

func TestRingBufferAdd(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Add("line1")
	rb.Add("line2")
	rb.Add("line3")

	lines := rb.Lines()
	if len(lines) != 3 {
		t.Fatalf("Lines() len = %d, want 3", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Errorf("Lines() = %v, want [line1 line2 line3]", lines)
	}
}

func TestRingBufferOverflow(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Add("a")
	rb.Add("b")
	rb.Add("c")
	rb.Add("d") // pushes out "a"
	rb.Add("e") // pushes out "b"

	lines := rb.Lines()
	if len(lines) != 3 {
		t.Fatalf("Lines() len = %d, want 3", len(lines))
	}
	if lines[0] != "c" || lines[1] != "d" || lines[2] != "e" {
		t.Errorf("Lines() = %v, want [c d e]", lines)
	}
}

func TestRingBufferEmpty(t *testing.T) {
	rb := NewRingBuffer(5)
	lines := rb.Lines()
	if len(lines) != 0 {
		t.Errorf("Lines() len = %d, want 0", len(lines))
	}
}

func TestRingBufferPartial(t *testing.T) {
	rb := NewRingBuffer(5)
	rb.Add("only")

	lines := rb.Lines()
	if len(lines) != 1 {
		t.Fatalf("Lines() len = %d, want 1", len(lines))
	}
	if lines[0] != "only" {
		t.Errorf("Lines()[0] = %q, want %q", lines[0], "only")
	}
}

func TestRingBufferLen(t *testing.T) {
	rb := NewRingBuffer(3)
	if rb.Len() != 0 {
		t.Errorf("Len() = %d, want 0", rb.Len())
	}
	rb.Add("a")
	rb.Add("b")
	if rb.Len() != 2 {
		t.Errorf("Len() = %d, want 2", rb.Len())
	}
	rb.Add("c")
	rb.Add("d")
	if rb.Len() != 3 {
		t.Errorf("Len() = %d, want 3", rb.Len())
	}
}

func TestRingBufferConcurrent(t *testing.T) {
	rb := NewRingBuffer(50)
	done := make(chan struct{})

	go func() {
		for i := 0; i < 100; i++ {
			rb.Add("line")
		}
		close(done)
	}()

	// Read concurrently
	for i := 0; i < 50; i++ {
		_ = rb.Lines()
		_ = rb.Len()
	}
	<-done

	lines := rb.Lines()
	if len(lines) != 50 {
		t.Errorf("Lines() len = %d, want 50", len(lines))
	}
}
