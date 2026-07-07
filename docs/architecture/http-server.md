# Module: HTTP Server

## Responsibility

Listen on localhost, route incoming requests to either the static file server or the CGI proxy based on the configured `api_prefix`.

## Requirements Covered

REQ-U-001, REQ-U-002, REQ-U-003, REQ-E-002, REQ-E-011, REQ-X-003, REQ-NF-001, REQ-NF-002

## Design

The server is a thin routing layer built on `net/http`. It does NOT use a third-party router — path prefix matching is sufficient.

### Routing Logic

```go
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if strings.HasPrefix(r.URL.Path, s.cfg.Backend.APIPrefix) {
        s.proxy.ServeHTTP(w, r)
    } else {
        s.static.ServeHTTP(w, r)
    }
}
```

### Static File Serving

- Uses `http.FileServer` wrapping `http.Dir(cfg.Frontend.Path)`
- No in-memory caching — reads from disk on every request (inherent hot-reload)
- `/` serves `index.html` (standard `http.FileServer` behaviour)
- Correct `Content-Type` via `http.DetectContentType` / extension mapping

### Binding

- Always binds to `127.0.0.1:<port>` — never `0.0.0.0`
- Server starts with a `context.Context` for graceful shutdown

### Concurrency

- `net/http` handles each request in its own goroutine
- Static file serving and CGI proxy requests run concurrently
- No shared mutable state between requests

## Public API

```go
type Server struct { ... }

func New(cfg *config.Config, invoker proxy.BackendInvoker, logger *slog.Logger) *Server
func (s *Server) Start(ctx context.Context) error
func (s *Server) Shutdown(ctx context.Context) error
```

## Testing Strategy

- Integration test: start server, GET a static file, verify 200 + correct body
- Integration test: start server, GET `/`, verify index.html served
- Integration test: GET a non-existent file, verify 404
- Integration test: GET `/api/...`, verify request reaches proxy handler
- Test that server binds only to 127.0.0.1 (attempt connection from loopback only)
- Test concurrent requests: static + API simultaneously both complete
