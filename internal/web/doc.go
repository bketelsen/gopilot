// Package web provides the HTTP dashboard server and real-time event streaming.
//
// It builds a chi router with REST endpoints for health, state, and metrics,
// an SSE hub for live dashboard updates, and WebSocket integration for
// planning sessions. Provider interfaces abstract state access to avoid
// circular imports with the orchestrator.
package web
