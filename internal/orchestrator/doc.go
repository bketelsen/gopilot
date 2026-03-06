// Package orchestrator implements the poll-dispatch-reconcile loop that drives Gopilot.
//
// It polls GitHub for eligible issues, claims and dispatches them to agent
// sessions in isolated workspaces, monitors running agents for completion or
// staleness, retries failed runs with exponential backoff, and reconciles
// state when issues become terminal or ineligible.
package orchestrator
