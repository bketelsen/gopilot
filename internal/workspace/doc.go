// Package workspace manages per-issue workspace directories and lifecycle hooks.
//
// The Manager interface defines operations for creating isolated directories,
// executing shell hooks (after_create, before_run, after_run, before_remove)
// with variable interpolation, and cleaning up after agent completion.
package workspace
