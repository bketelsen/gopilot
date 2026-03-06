package web

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/a-h/templ"
)

// SSEHub manages Server-Sent Events clients and broadcasts.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

// SSEEvent is a typed event sent to SSE clients.
type SSEEvent struct {
	Type string
	Data string
}

// NewSSEHub creates a hub with no connected clients.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan SSEEvent]struct{}),
	}
}

// Broadcast sends a text event to all connected clients.
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

// BroadcastComponent renders a templ component and broadcasts its HTML.
func (h *SSEHub) BroadcastComponent(eventType string, component templ.Component) {
	var buf bytes.Buffer
	component.Render(context.Background(), &buf) //nolint:errcheck // best-effort template render
	h.Broadcast(eventType, buf.String())
}

// Subscribe registers a new client and returns a channel and cleanup function.
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

// HandleSSE is an HTTP handler that streams SSE events to the client.
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
