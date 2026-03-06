package config

import (
	"context"
	"log/slog"

	"github.com/fsnotify/fsnotify"
)

// ReloadCallback is the function signature for config reload notifications.
type ReloadCallback func(cfg *Config, err error)

// Watcher monitors a configuration file for changes and triggers reloads.
type Watcher struct {
	fsw *fsnotify.Watcher
}

// Watch starts watching the config file at path and calls cb on each change.
func Watch(ctx context.Context, path string, cb ReloadCallback) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := fsw.Add(path); err != nil {
		fsw.Close()
		return nil, err
	}

	go func() {
		defer fsw.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-fsw.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					slog.Info("config file changed, reloading", "path", path)
					cfg, err := Load(path)
					cb(cfg, err)
				}
			case err, ok := <-fsw.Errors:
				if !ok {
					return
				}
				slog.Error("config watcher error", "error", err)
			}
		}
	}()

	return &Watcher{fsw: fsw}, nil
}

// Close stops watching the configuration file.
func (w *Watcher) Close() error {
	return w.fsw.Close()
}
