package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// slogLogFormatter implements chi's middleware.LogFormatter using slog.
type slogLogFormatter struct{}

func (f *slogLogFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	return &slogLogEntry{
		method:     r.Method,
		path:       r.URL.Path,
		remoteAddr: r.RemoteAddr,
		userAgent:  r.UserAgent(),
		isStatic:   strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/favicon.ico",
	}
}

// slogLogEntry implements chi's middleware.LogEntry using slog.
type slogLogEntry struct {
	method, path, remoteAddr, userAgent string
	isStatic                            bool
}

func (e *slogLogEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	var level slog.Level
	switch {
	case e.isStatic:
		level = slog.LevelDebug
	case status >= 500:
		level = slog.LevelError
	case status >= 400:
		level = slog.LevelWarn
	default:
		level = slog.LevelInfo
	}

	durationMS := float64(elapsed.Microseconds()) / 1000.0

	slog.Log(context.Background(), level, "http request",
		"method", e.method,
		"path", e.path,
		"status", status,
		"duration_ms", durationMS,
		"remote_addr", e.remoteAddr,
		"bytes", bytes,
		"user_agent", e.userAgent,
	)
}

func (e *slogLogEntry) Panic(v interface{}, stack []byte) {
	slog.Error("http request panic",
		"method", e.method,
		"path", e.path,
		"panic", fmt.Sprintf("%v", v),
		"stack", string(stack),
	)
}
