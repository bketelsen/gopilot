package config

import (
	"log/slog"
	"os"
)

// SetupLogging configures the global slog logger to write JSON to stderr.
func SetupLogging(level slog.Level) {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}
