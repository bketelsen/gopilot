package web

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/a-h/templ"
)

type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

type SSEEvent struct {
	Type string
	Data string
}

func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan SSEEvent]struct{}),
	}
}

func (h *SSEHub) Broadcast(eventType string, data string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	event := SSEEvent{Type: eventType, Data: data}
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
		}
	}
}

func (h *SSEHub) BroadcastComponent(eventType string, component templ.Component) {
	var buf bytes.Buffer
	component.Render(context.Background(), &buf)
	h.Broadcast(eventType, buf.String())
}

func (h *SSEHub) Subscribe() (chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}
}

func (h *SSEHub) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cleanup := h.Subscribe()
	defer cleanup()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			flusher.Flush()
		}
	}
}
