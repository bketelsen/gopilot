// Package logging configures structured JSON logging for Gopilot.
//
// It sets up slog with a JSON handler writing to stderr and an optional log
// file, and provides helper functions to create loggers pre-filled with
// issue context fields.
package logging
