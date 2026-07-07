package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fsnotify/fsnotify"
)

// SyncEngine watches a directory and triggers git sync on changes.
type SyncEngine struct {
	syncer          GitSyncer
	watchDir        string
	addPath         string
	inactivityDelay time.Duration
	maxInterval     time.Duration
	logger          *slog.Logger
	pending         bool
}

func NewSyncEngine(syncer GitSyncer, dataDir string, inactivityDelay, maxInterval time.Duration, logger *slog.Logger) *SyncEngine {
	if logger == nil {
		logger = slog.Default()
	}
	return &SyncEngine{
		syncer:          syncer,
		watchDir:        dataDir,
		addPath:         dataDir,
		inactivityDelay: inactivityDelay,
		maxInterval:     maxInterval,
		logger:          logger,
	}
}

// NewSyncEngineWithPaths allows specifying separate watch and add paths.
// watchDir is the absolute path for fsnotify; addPath is the relative path for git add.
func NewSyncEngineWithPaths(syncer GitSyncer, watchDir, addPath string, inactivityDelay, maxInterval time.Duration, logger *slog.Logger) *SyncEngine {
	if logger == nil {
		logger = slog.Default()
	}
	return &SyncEngine{
		syncer:          syncer,
		watchDir:        watchDir,
		addPath:         addPath,
		inactivityDelay: inactivityDelay,
		maxInterval:     maxInterval,
		logger:          logger,
	}
}

func (e *SyncEngine) Start(ctx context.Context) error {
	if err := e.syncer.Pull(); err != nil {
		e.logger.Warn("pull on startup failed", "err", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(e.watchDir); err != nil {
		return err
	}

	inactivityTimer := time.NewTimer(0)
	if !inactivityTimer.Stop() {
		<-inactivityTimer.C
	}

	maxTimer := time.NewTimer(0)
	if !maxTimer.Stop() {
		<-maxTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			if e.pending {
				e.doSync("shutdown")
			}
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				if !e.pending {
					maxTimer.Reset(e.maxInterval)
				}
				e.pending = true
				inactivityTimer.Reset(e.inactivityDelay)
			}

		case <-inactivityTimer.C:
			e.doSync("inactivity")
			maxTimer.Stop()

		case <-maxTimer.C:
			e.doSync("max_interval")
			inactivityTimer.Stop()

		case _, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
		}
	}
}

func (e *SyncEngine) doSync(reason string) {
	e.logger.Info("sync triggered", "reason", reason)

	if err := e.syncer.Add([]string{e.addPath}); err != nil {
		e.logger.Error("sync add failed", "err", err)
		return
	}

	msg := fmt.Sprintf("auto-sync: %s", time.Now().Format(time.RFC3339))
	if err := e.syncer.Commit(msg); err != nil {
		e.logger.Error("sync commit failed", "err", err)
		return
	}

	if err := e.syncer.Push(false); err != nil {
		e.logger.Warn("push failed, retrying with force", "err", err)
		if err := e.syncer.Push(true); err != nil {
			e.logger.Error("force push failed", "err", err)
			return
		}
	}

	e.pending = false
	e.logger.Info("sync completed", "reason", reason)
}

func (e *SyncEngine) FlushSync() {
	if e.pending {
		e.doSync("shutdown")
	}
}

func (e *SyncEngine) HasPending() bool {
	return e.pending
}
