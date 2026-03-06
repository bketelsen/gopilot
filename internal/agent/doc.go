// Package agent launches and manages AI coding agent subprocesses.
//
// It defines the Runner interface for starting and stopping agents, with
// concrete implementations for Claude Code CLI (ClaudeRunner) and GitHub
// Copilot CLI (CopilotRunner). Each running agent is tracked as a Session
// that holds the subprocess PID, cancellation function, and exit status.
package agent
