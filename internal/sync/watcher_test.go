package sync

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type mockSyncer struct {
	syncCount atomic.Int32
}

func (m *mockSyncer) Pull() error               { return nil }
func (m *mockSyncer) Add(paths []string) error  { return nil }
func (m *mockSyncer) Commit(msg string) error   { m.syncCount.Add(1); return nil }
func (m *mockSyncer) Push(force bool) error      { return nil }

func setupWatcherTest(t *testing.T, inactivity, maxInterval time.Duration) (*SyncEngine, string, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	mock := &mockSyncer{}
	engine := NewSyncEngine(mock, dir, inactivity, maxInterval, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go engine.Start(ctx)
	// Give watcher time to start
	time.Sleep(20 * time.Millisecond)
	return engine, dir, cancel
}

func TestWatcher_SingleChange_InactivityFires(t *testing.T) {
	engine, dir, cancel := setupWatcherTest(t, 50*time.Millisecond, 500*time.Millisecond)
	defer cancel()

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	time.Sleep(120 * time.Millisecond)

	mock := engine.syncer.(*mockSyncer)
	if got := mock.syncCount.Load(); got != 1 {
		t.Fatalf("expected 1 sync, got %d", got)
	}
}

func TestWatcher_RapidChanges_OnlyOneSync(t *testing.T) {
	engine, dir, cancel := setupWatcherTest(t, 50*time.Millisecond, 500*time.Millisecond)
	defer cancel()

	for i := range 10 {
		os.WriteFile(filepath.Join(dir, "test.txt"), []byte{byte(i)}, 0644)
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(120 * time.Millisecond)

	mock := engine.syncer.(*mockSyncer)
	if got := mock.syncCount.Load(); got != 1 {
		t.Fatalf("expected 1 sync, got %d", got)
	}
}

func TestWatcher_MaxInterval_Fires(t *testing.T) {
	engine, dir, cancel := setupWatcherTest(t, 200*time.Millisecond, 100*time.Millisecond)
	defer cancel()

	// Write events continuously, max-interval should fire before inactivity
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("a"), 0644)
	time.Sleep(150 * time.Millisecond)

	mock := engine.syncer.(*mockSyncer)
	if got := mock.syncCount.Load(); got < 1 {
		t.Fatalf("expected at least 1 sync from max-interval, got %d", got)
	}
}

func TestWatcher_TimerResets_OnNewEvent(t *testing.T) {
	engine, dir, cancel := setupWatcherTest(t, 80*time.Millisecond, 500*time.Millisecond)
	defer cancel()

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("a"), 0644)
	time.Sleep(50 * time.Millisecond)
	// Should not have synced yet
	mock := engine.syncer.(*mockSyncer)
	if got := mock.syncCount.Load(); got != 0 {
		t.Fatalf("expected 0 syncs at 50ms, got %d", got)
	}
	// New event resets timer
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("b"), 0644)
	time.Sleep(50 * time.Millisecond)
	if got := mock.syncCount.Load(); got != 0 {
		t.Fatalf("expected 0 syncs at 100ms (timer reset), got %d", got)
	}
	// Now wait for inactivity
	time.Sleep(80 * time.Millisecond)
	if got := mock.syncCount.Load(); got != 1 {
		t.Fatalf("expected 1 sync after full inactivity, got %d", got)
	}
}

func TestWatcher_ShutdownFlushes_Pending(t *testing.T) {
	engine, dir, cancel := setupWatcherTest(t, 500*time.Millisecond, 5*time.Second)

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("a"), 0644)
	time.Sleep(30 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	mock := engine.syncer.(*mockSyncer)
	if got := mock.syncCount.Load(); got != 1 {
		t.Fatalf("expected 1 sync on shutdown flush, got %d", got)
	}
	_ = engine
}

func TestWatcher_NoPending_ShutdownClean(t *testing.T) {
	engine, _, cancel := setupWatcherTest(t, 50*time.Millisecond, 500*time.Millisecond)

	cancel()
	time.Sleep(50 * time.Millisecond)

	mock := engine.syncer.(*mockSyncer)
	if got := mock.syncCount.Load(); got != 0 {
		t.Fatalf("expected 0 syncs on clean shutdown, got %d", got)
	}
	_ = engine
}
