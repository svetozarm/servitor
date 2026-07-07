# SERVITOR — High-Level Architecture

## Overview

SERVITOR is a single Go binary that turns a git repository into a self-hosted web application. It has seven internal modules that collaborate at runtime:

```
┌───────────────────────────────────────────────────────────────────┐
│                          servitor binary                           │
├──────────┬───────────┬───────────┬──────────┬──────────┬──────────┤
│  Config  │HTTP Server│ CGI Proxy │ Git Sync │Self-Update│Lifecycle │
│  Module  │  Module   │  Module   │  Module  │  Module  │  Module  │
└──────────┴───────────┴───────────┴──────────┴──────────┴──────────┘
                                                              │
                          ┌───────────────────────────────────┘
                          ▼
                   ┌─────────────┐
                   │ Scaffolding │  (servitor init — CLI only)
                   │   Module    │
                   └─────────────┘
```

## Modules

| Module | Responsibility | Key Interfaces |
|--------|---------------|----------------|
| **Configuration** | Parse `.servitor.conf`, validate, provide typed config to all modules | `Config` struct |
| **HTTP Server** | Listen on localhost, route requests to static files or CGI proxy | `net/http` handlers |
| **CGI Proxy** | Spawn backend binary, pipe raw HTTP, return response | `BackendInvoker` interface |
| **Git Sync** | Watch `data/`, debounce changes, commit + push | `GitSyncer` interface |
| **Self-Update** | Check GitHub releases, download + verify + replace binary | `ReleaseChecker` interface |
| **Lifecycle** | Startup orchestration, signal handling, graceful shutdown | `context.Context` propagation |
| **Scaffolding** | `servitor init` — clone templates, generate project structure | CLI subcommand |

## Data Flow

```
Browser Request
       │
       ▼
┌─────────────────┐
│   HTTP Server   │
│ (host:port)     │
├─────────────────┤
│ path matches    │──── /api/* ────► CGI Proxy ──► servitor-backend (subprocess)
│ api_prefix?     │                                      │
│                 │                                      ▼
│ otherwise       │──── /* ────► Static File Server    data/
│                 │              (frontend/)              │
└─────────────────┘                                      ▼
                                                   Git Sync Engine
                                                   (fsnotify → commit → push)
```

## Multi-App Mode

When the top-level `.servitor.conf` contains an `apps` list, servitor runs in multi-app mode:

```
Browser Request
       │
       ▼
┌─────────────────────────────────────────────────────┐
│   HTTP Server (host:port)                            │
├─────────────────────────────────────────────────────┤
│ /              → index page (links to all apps)      │
│ /<name>/api/* → CGI Proxy (app's backend binary)     │
│ /<name>/*     → Static File Server (app's frontend/) │
└─────────────────────────────────────────────────────┘
       │                              │
       ▼ (per app)                    ▼ (per app)
   backend binary                  data/
       │                              │
       ▼                              ▼
   WorkDir = app dir           Git Sync Engine
                               (one per app, shared repo)
```

### Key multi-app behaviours:

- The app name prefix (`/<name>/`) is stripped before forwarding to backend and static handler
- Backend binary runs with `WorkDir` set to the app's directory (so it finds `data/` locally)
- Backend binary path is resolved to absolute at config load time
- Git sync opens the repo at the root; `addPath` is relative to repo root (e.g. `apps/notes/data/`)
- Root URL (`/`) serves an HTML index page with links to all registered apps
- Frontends must use **relative** API paths (e.g. `api/types` not `/api/types`)

## Key Abstractions (Android-Ready)

Two interfaces are defined from the start to enable Android support later without refactoring:

### BackendInvoker

```go
type BackendInvoker interface {
    Invoke(req *http.Request) (*http.Response, error)
}
```

- **CGIInvoker** (desktop): spawns subprocess, serializes raw HTTP to stdin, parses response from stdout
- **InProcessInvoker** (future/Android): calls `HandleRequest()` directly

### GitSyncer

```go
type GitSyncer interface {
    Add(paths []string) error
    Commit(message string) error
    Push(force bool) error
}
```

- **GoGitSyncer** (all platforms): uses `github.com/go-git/go-git/v5`

### ReleaseChecker

```go
type ReleaseChecker interface {
    LatestRelease(ctx context.Context) (*Release, error)
    DownloadAsset(ctx context.Context, url string, dest string) error
}
```

- **GitHubClient**: queries GitHub API for latest release, downloads assets with host allowlist

## Package Layout

```
servitor/
├── cmd/
│   └── servitor/
│       └── main.go              # Entry point, CLI parsing, subcommands
├── internal/
│   ├── config/
│   │   ├── config.go            # Struct definitions
│   │   ├── duration.go          # Custom YAML duration type
│   │   ├── load.go              # Parsing + validation
│   │   └── config_test.go
│   ├── server/
│   │   ├── server.go            # HTTP server + routing
│   │   └── server_test.go
│   ├── proxy/
│   │   ├── invoker.go           # BackendInvoker interface
│   │   ├── handler.go           # ProxyHandler (http.Handler)
│   │   ├── cgi.go               # CGIInvoker implementation
│   │   └── cgi_test.go
│   ├── sync/
│   │   ├── syncer.go            # GitSyncer interface
│   │   ├── gogit.go             # GoGitSyncer implementation
│   │   ├── watcher.go           # SyncEngine (fsnotify + timer logic)
│   │   └── watcher_test.go
│   ├── update/
│   │   ├── update.go            # Run() orchestrator
│   │   ├── github.go            # GitHubClient (ReleaseChecker)
│   │   ├── version.go           # Semver comparison
│   │   ├── asset_name.go        # Platform-specific asset name derivation
│   │   ├── checksum.go          # SHA256 verification
│   │   ├── replace.go           # Binary replacement (Unix)
│   │   ├── replace_windows.go   # Binary replacement (Windows)
│   │   ├── writable.go          # Directory writability check
│   │   └── errors.go            # Sentinel errors
│   └── scaffold/
│       ├── init.go              # servitor init logic
│       └── init_test.go
├── go.mod
├── go.sum
├── .goreleaser.yaml
└── .servitor.conf.example
```

Note: there is no `internal/lifecycle/` package. Lifecycle orchestration lives directly in `cmd/servitor/main.go`.

## Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| `gopkg.in/yaml.v3` | v3.0.1 | YAML config parsing |
| `github.com/go-git/go-git/v5` | v5.19.1 | Git operations (clone, add, commit, push) |
| `github.com/fsnotify/fsnotify` | v1.10.1 | Filesystem change notifications |

Standard library only for everything else (`net/http`, `log/slog`, `os/exec`, `context`, `os/signal`, `archive/tar`, `crypto/sha256`).

## Configuration Defaults

| Field | Default | Notes |
|---|---|---|
| `server.host` | `127.0.0.1` | Always localhost, not configurable to other values |
| `server.port` | `8080` | |
| `frontend.path` | `./frontend` | |
| `backend.path` | `./backend/servitor-backend` | |
| `backend.api_prefix` | `/api` | |
| `backend.timeout` | `3s` | Kill subprocess after this duration |
| `sync.enabled` | `true` | |
| `sync.inactivity_delay` | `30s` | |
| `sync.max_interval` | `5m` | |

## Cross-Cutting Concerns

- **Logging**: `log/slog` with text handler writing to stderr, all entries timestamped
- **Error handling**: Errors bubble up to HTTP layer as 4xx/5xx; internal errors logged
- **Concurrency**: HTTP server handles requests concurrently via goroutines; git sync runs in its own goroutine, serializes operations via channel
- **Shutdown**: `context.Context` cancelled on SIGINT/SIGTERM; all modules respect cancellation; git sync flushes pending work before exit
- **Versioning**: `version` variable set via `-ldflags` at build time; used by `servitor version` and `servitor update`
