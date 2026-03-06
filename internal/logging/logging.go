package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// Setup configures the global slog logger to write JSON to stderr and optionally a file.
func Setup(level slog.Level, logFile string) {
	var w io.Writer = os.Stderr
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			w = io.MultiWriter(os.Stderr, f)
		}
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

// IssueLogger returns a logger with issue context fields.
func IssueLogger(repo string, id int, sessionID string) *slog.Logger {
	return slog.With(
		"issue_id", id,
		"issue", fmt.Sprintf("%s#%d", repo, id),
		"session_id", sessionID,
	)
}
