// Package config loads, validates, and watches the gopilot.yaml configuration file.
//
// It parses YAML into typed structs covering GitHub credentials, polling intervals,
// workspace hooks, agent settings, skills directories, dashboard options, and prompt
// templates. A filesystem watcher supports hot-reload of select settings without
// restarting the process.
package config
