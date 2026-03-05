package logging

import (
	"fmt"
	"log/slog"
	"os"
)

// Setup configures the global slog logger to write JSON to stderr.
func Setup(level slog.Level) {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
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
