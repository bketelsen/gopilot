package config

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const testYAML = `
github:
  token: test-token
  repos: [owner/repo]
  project: {owner: "@me", number: 1}
  eligible_labels: [gopilot]
agent: {command: copilot}
workspace: {root: /tmp}
polling: {interval_ms: 30000}
`

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(testYAML), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var reloadCount atomic.Int32
	w, err := Watch(ctx, path, func(cfg *Config, err error) {
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

func TestWatcherStopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(testYAML), 0644)

	ctx, cancel := context.WithCancel(context.Background())

	var reloadCount atomic.Int32
	w, err := Watch(ctx, path, func(cfg *Config, err error) {
		reloadCount.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(100 * time.Millisecond)

	// Cancel the context to stop the watcher goroutine.
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Write after cancel — callback should NOT fire.
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

	if reloadCount.Load() != 0 {
		t.Errorf("expected 0 reload callbacks after context cancel, got %d", reloadCount.Load())
	}
}

func TestWatcherCloseStillWorks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopilot.yaml")
	os.WriteFile(path, []byte(testYAML), 0644)

	ctx := context.Background()

	w, err := Watch(ctx, path, func(cfg *Config, err error) {})
	if err != nil {
		t.Fatal(err)
	}

	// Close() should still work as an alternative shutdown mechanism.
	if err := w.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}
