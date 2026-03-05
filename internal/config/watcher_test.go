package config

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")

	yaml := `
github:
  token: test-token
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
polling: {interval_ms: 30000}
`
	os.WriteFile(path, []byte(yaml), 0644)

	var reloadCount atomic.Int32
	w, err := Watch(path, func(cfg *Config, err error) {
		reloadCount.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(100 * time.Millisecond)

	newYaml := `
github:
  token: test-token
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
polling: {interval_ms: 15000}
`
	os.WriteFile(path, []byte(newYaml), 0644)

	time.Sleep(500 * time.Millisecond)

	if reloadCount.Load() < 1 {
		t.Error("expected at least 1 reload callback")
	}
}
