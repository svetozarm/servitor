# Module: CGI Proxy

## Responsibility

For requests matching the API prefix, serialize the raw HTTP request, invoke the backend binary, parse the raw HTTP response, and return it to the client. Handles timeout and crash scenarios.

## Requirements Covered

REQ-U-004, REQ-U-005, REQ-U-006, REQ-E-001, REQ-S-003, REQ-X-001, REQ-X-002, REQ-X-004, REQ-O-001

## Interface

```go
package proxy

type BackendInvoker interface {
    Invoke(req *http.Request) (*http.Response, error)
}
```

This interface enables swapping the subprocess implementation for an in-process call on Android later.

## CGI Invoker (Desktop Implementation)

### Request Flow

1. Serialize incoming `*http.Request` to raw HTTP bytes (request line + headers + body)
2. Spawn `os/exec.Command(cfg.Backend.Path)`
3. Write serialized request to subprocess stdin
4. Close stdin to signal end-of-input
5. Read stdout until EOF
6. Parse response bytes via `http.ReadResponse`
7. Return parsed response

### Serialization Format

Request to stdin:
```
GET /api/items HTTP/1.1\r\n
Host: localhost:8080\r\n
Content-Type: application/json\r\n
Content-Length: 0\r\n
\r\n
```

Response from stdout:
```
HTTP/1.1 200 OK\r\n
Content-Type: application/json\r\n
Content-Length: 15\r\n
\r\n
{"items":[]}
```

### Timeout Enforcement

- Create a `context.WithTimeout` using `cfg.Backend.Timeout` (default 3s)
- Pass context to `exec.CommandContext`
- If context expires: process is killed, return HTTP 504

### Error Handling

| Condition | Response | Log |
|---|---|---|
| Binary not found at path | 502 Bad Gateway | error with path |
| Process exits non-zero | 502 Bad Gateway | error with exit code + stderr |
| Process exceeds timeout | 504 Gateway Timeout | warning with path + duration |
| Stdout not valid HTTP | 502 Bad Gateway | error with parse failure |

### Hot-Reload

Since the backend is spawned fresh per request, recompiling the binary is picked up immediately on the next request. No restart of servitor is needed.

## Public API

```go
type CGIInvoker struct {
    BinaryPath string
    Timeout    time.Duration
    Logger     *slog.Logger
}

func (c *CGIInvoker) Invoke(req *http.Request) (*http.Response, error)
```

## HTTP Handler Wrapper

```go
type ProxyHandler struct {
    Invoker BackendInvoker
    Logger  *slog.Logger
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

The handler calls `Invoker.Invoke(r)`, copies the response status/headers/body to `w`, and handles errors by writing the appropriate error status code.

## Testing Strategy

- Unit test: mock a backend binary (shell script echoing fixed HTTP response), verify round-trip
- Unit test: backend exits with code 1 → verify 502
- Unit test: backend sleeps longer than timeout → verify 504 + process killed
- Unit test: binary path doesn't exist → verify 502 + error logged
- Unit test: verify all HTTP methods (GET, POST, PUT, DELETE) are forwarded correctly
- Unit test: verify request body is piped to stdin accurately
