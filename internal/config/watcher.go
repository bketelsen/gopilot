package config

import (
	"context"
	"log/slog"

	"github.com/fsnotify/fsnotify"
)

type ReloadCallback func(cfg *Config, err error)

type Watcher struct {
	fsw *fsnotify.Watcher
}

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

func (w *Watcher) Close() error {
	return w.fsw.Close()
}
