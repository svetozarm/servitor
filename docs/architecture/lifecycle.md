# Module: Lifecycle

## Responsibility

Orchestrate startup of all modules, propagate `context.Context` for cancellation, handle OS signals, and ensure graceful shutdown with pending work flushed. Dispatch CLI subcommands (`init`, `update`, `version`).

## Requirements Covered

REQ-E-009, REQ-NF-001, REQ-NF-005

## CLI Dispatch

```go
func main() {
    if len(os.Args) > 1 {
        switch os.Args[1] {
        case "init":    runInit()
        case "update":  runUpdate()
        case "version": // print version and exit
        }
    }
    runServer()
}
```

| Subcommand | Behaviour |
|---|---|
| (none) | Start the server |
| `init` | Scaffold a new project (see scaffolding module) |
| `update` | Self-update binary from GitHub releases |
| `version` | Print `servitor <version>` (or `dev` if unset) and exit |

## Startup Sequence (Server)

```
1. Load configuration (config.Load)
2. Initialise logger (slog to stderr)
3. Create context via signal.NotifyContext(SIGINT, SIGTERM)
4. Open git repository (go-git) + create SyncEngine (if sync enabled)
5. Start SyncEngine goroutine (if enabled)
6. Create CGIInvoker + ProxyHandler
7. Create HTTP Server
8. Start HTTP Server in background goroutine
9. Block waiting for context cancellation
```

Note: lifecycle orchestration lives in `cmd/servitor/main.go` — there is no separate `internal/lifecycle` package.

## Shutdown Sequence

```
Signal received (SIGINT/SIGTERM)
       │
       ▼
1. Root context cancelled
2. HTTP Server.Shutdown() — stops accepting, drains in-flight requests (10s grace)
3. Wait for server goroutine to return
4. Wait for SyncEngine goroutine to finish (it flushes pending on ctx.Done)
5. Log shutdown complete
6. Exit 0
```

## Signal Handling

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
```

- Single signal → graceful shutdown
- Second signal → force exit (standard Go behaviour with NotifyContext)

## Logging Setup

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
```

All modules receive `*slog.Logger` via constructor injection.

### Log Events

| Event | Level | Fields |
|---|---|---|
| Server started | Info | addr |
| Backend error | Error | path, exit_code, stderr_snippet |
| Sync triggered | Info | reason (inactivity/max_interval/shutdown) |
| Sync completed | Info | reason |
| Push failed | Warn | error |
| Force push | Warn | reason |
| Shutdown initiated | Info | — |
| Shutdown complete | Info | — |
| Sync engine error | Error | err |
| Server shutdown error | Error | err |

## Versioning

The `version` variable is set at build time via ldflags:

```
go build -ldflags "-X main.version=v1.0.0" ./cmd/servitor/
```

Used by `servitor version` and `servitor update` to determine current version.

## Testing Strategy

- Integration test: start servitor, send SIGTERM, verify it exits cleanly
- Integration test: start servitor with pending data changes, SIGTERM, verify sync flushed
- Unit test: verify startup fails fast on bad config (exits before server starts)
