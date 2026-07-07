# Module: Git Sync

## Responsibility

Watch the `data/` directory for filesystem changes. When changes are detected, commit and push to the remote using a debounce + max-interval strategy. Uses go-git for all git operations.

## Requirements Covered

REQ-S-001, REQ-S-002, REQ-E-003, REQ-E-004, REQ-E-005, REQ-E-006, REQ-E-009, REQ-E-010, REQ-X-005, REQ-O-002, REQ-NF-005

## Architecture

```
┌────────────────────────────────────────────────────┐
│                  Sync Engine                         │
├────────────────────────────────────────────────────┤
│                                                      │
│  ┌──────────┐    events    ┌──────────────────┐    │
│  │ Watcher  │─────────────►│  Timer Manager   │    │
│  │(fsnotify)│              │                  │    │
│  └──────────┘              │ inactivity timer │    │
│                            │ max-interval     │    │
│                            └────────┬─────────┘    │
│                                     │ trigger       │
│                                     ▼               │
│                            ┌──────────────────┐    │
│                            │   GitSyncer      │    │
│                            │   (go-git)       │    │
│                            └──────────────────┘    │
└────────────────────────────────────────────────────┘
```

## Interface

```go
package sync

type GitSyncer interface {
    Add(paths []string) error
    Commit(message string) error
    Push(force bool) error
}
```

## GoGitSyncer Implementation

Uses `github.com/go-git/go-git/v5`:

```go
type GoGitSyncer struct {
    repo   *git.Repository
    logger *slog.Logger
}

func NewGoGitSyncer(repoPath string, logger *slog.Logger) (*GoGitSyncer, error)
func (g *GoGitSyncer) Add(paths []string) error    // worktree.Add()
func (g *GoGitSyncer) Commit(message string) error  // worktree.Commit()
func (g *GoGitSyncer) Push(force bool) error         // repo.Push() with force option
```

### Push Strategy

- Always attempt normal push first
- If push fails with "non-fast-forward" error, retry with `force: true`
- Log all push outcomes

## Watcher + Timer Logic

### Components

```go
type SyncEngine struct {
    syncer          GitSyncer
    watchDir        string
    addPath         string
    inactivityDelay time.Duration
    maxInterval     time.Duration
    logger          *slog.Logger
    pending         bool
}
```

### Constructors

```go
// NewSyncEngine creates a SyncEngine where the watch path and git add path are the same.
// This is the common case when running from the repo root (e.g., dataDir = "data/").
func NewSyncEngine(syncer GitSyncer, dataDir string, inactivityDelay, maxInterval time.Duration, logger *slog.Logger) *SyncEngine

// NewSyncEngineWithPaths allows specifying separate watch and add paths.
// watchDir is the absolute path for fsnotify; addPath is the relative path for git add.
// Needed in integration tests where fsnotify requires an absolute path but git add
// needs a path relative to the worktree root.
func NewSyncEngineWithPaths(syncer GitSyncer, watchDir, addPath string, inactivityDelay, maxInterval time.Duration, logger *slog.Logger) *SyncEngine
```

### State Machine

```
IDLE ──(fs event)──► WAITING
                       │
            ┌──────────┤
            │          │
     (fs event)   (inactivity timer expires)
            │          │
            ▼          ▼
        WAITING     SYNCING ──► IDLE
            │
     (max-interval expires)
            │
            ▼
         SYNCING ──► IDLE
```

### Timer Behaviour

1. **First change detected**: start inactivity timer (30s) AND max-interval timer (5m)
2. **Subsequent changes**: reset inactivity timer only; max-interval continues
3. **Inactivity timer expires**: trigger sync, reset both timers
4. **Max-interval expires**: trigger sync, stop inactivity timer

### Sync Operation (triggered)

1. `Add(["data/"])` — stage all changes in data directory
2. `Commit("auto-sync: 2026-06-04T16:00:00Z")` — timestamp in RFC3339
3. `Push(false)` — attempt normal push
4. If push fails → `Push(true)` — force push
5. Log result

### Shutdown

- On context cancellation: check if `pending` is true
- If yes: perform one final sync (add + commit + push)
- Then return nil

## Public API

```go
func NewSyncEngine(syncer GitSyncer, dataDir string, inactivityDelay, maxInterval time.Duration, logger *slog.Logger) *SyncEngine
func NewSyncEngineWithPaths(syncer GitSyncer, watchDir, addPath string, inactivityDelay, maxInterval time.Duration, logger *slog.Logger) *SyncEngine
func (e *SyncEngine) Start(ctx context.Context) error  // blocking, runs until ctx cancelled
func (e *SyncEngine) HasPending() bool
func (e *SyncEngine) FlushSync()  // force immediate sync (no return value)
```

## Testing Strategy

- Unit test timer logic: simulate fs events, verify sync triggers at correct times
- Unit test: single change → wait inactivity_delay → sync triggered
- Unit test: rapid changes → only one sync after inactivity
- Unit test: continuous changes for > max_interval → sync triggered at max_interval
- Unit test: push conflict → verify force push retry
- Unit test: shutdown with pending changes → verify flush
- Integration test: actual go-git operations against a local bare repo
- Unit test: sync disabled → no watcher started, no operations
