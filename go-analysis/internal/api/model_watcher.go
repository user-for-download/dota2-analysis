package api

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/scoring"
)

// ModelWatcher listens for SIGHUP and triggers a model reload.
type ModelWatcher struct {
	reloader scoring.ModelReloader
	log      *slog.Logger
}

// NewModelWatcher creates a new ModelWatcher.
// Pass nil for reloader to create a no-op watcher (e.g. when using linear scorer).
func NewModelWatcher(reloader scoring.ModelReloader, log *slog.Logger) *ModelWatcher {
	return &ModelWatcher{reloader: reloader, log: log}
}

// Watch listens for SIGHUP and calls reloader.Reload() on each signal.
// Blocks until ctx is cancelled. Safe to call as a goroutine.
func (w *ModelWatcher) Watch(ctx context.Context) {
	if w.reloader == nil {
		return
	}
	sigHUP := make(chan os.Signal, 1)
	signal.Notify(sigHUP, syscall.SIGHUP)
	defer signal.Stop(sigHUP)

	for {
		select {
		case <-sigHUP:
			if err := w.reloader.Reload(); err != nil {
				w.log.Error("model reload failed", "err", err)
			} else {
				w.log.Info("model reloaded successfully", "version", w.reloader.Version())
			}
		case <-ctx.Done():
			return
		}
	}
}
