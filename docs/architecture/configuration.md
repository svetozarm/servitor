# Module: Configuration

## Responsibility

Parse, validate, and expose the `.servitor.conf` YAML file as a typed Go struct. This module is the first thing invoked at startup and feeds all other modules.

## Requirements Covered

REQ-U-003, REQ-X-006, REQ-X-007, REQ-S-002

## Struct Definition

```go
package config

import "time"

type Config struct {
    Server   ServerConfig   `yaml:"server"`
    Frontend FrontendConfig `yaml:"frontend"`
    Backend  BackendConfig  `yaml:"backend"`
    Sync     SyncConfig     `yaml:"sync"`
}

type ServerConfig struct {
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
}

type FrontendConfig struct {
    Path string `yaml:"path"`
}

type BackendConfig struct {
    Path      string        `yaml:"path"`
    APIPrefix string        `yaml:"api_prefix"`
    Timeout   time.Duration `yaml:"timeout"`
}

type SyncConfig struct {
    Enabled         bool          `yaml:"enabled"`
    InactivityDelay time.Duration `yaml:"inactivity_delay"`
    MaxInterval     time.Duration `yaml:"max_interval"`
}
```

## Behaviour

### Loading

1. Look for `.servitor.conf` in the current working directory
2. Read file contents
3. Unmarshal YAML via `gopkg.in/yaml.v3`
4. Apply defaults for missing optional fields
5. Validate required fields are present

### Defaults

| Field | Default |
|---|---|
| `server.host` | `127.0.0.1` |
| `server.port` | `8080` |
| `backend.timeout` | `3s` |
| `sync.enabled` | `true` |
| `sync.inactivity_delay` | `30s` |
| `sync.max_interval` | `5m` |

### Validation Rules

- `frontend.path` — required, must be a non-empty string
- `backend.path` — required, must be a non-empty string
- `backend.api_prefix` — required, must start with `/`
- `server.port` — must be 1–65535
- `sync.inactivity_delay` — must be > 0 if sync enabled
- `sync.max_interval` — must be > inactivity_delay if sync enabled

### Error Cases

- File not found → exit with "configuration file .servitor.conf not found in current directory"
- YAML parse error → exit with "failed to parse .servitor.conf: <yaml error>"
- Missing required field → exit with "missing required configuration: <field path>"
- Invalid value → exit with "invalid configuration: <field path>: <reason>"

## Public API

```go
func Load(path string) (*Config, error)
func LoadApps(cfg *Config, baseDir string) ([]AppConfig, error)
func (c *Config) Addr() string      // returns "host:port"
func (c *Config) IsMultiApp() bool   // true when apps list is non-empty
```

## Multi-App Mode (Cascading Configs)

When the top-level `.servitor.conf` contains an `apps` list, servitor runs in multi-app mode. Each app is hosted under its own URL prefix (`/<name>/`).

### Top-Level Config

```yaml
server:
  port: 8080
  host: 127.0.0.1

apps:
  - name: notes
    path: ./apps/notes
  - name: wiki
    path: ./apps/wiki
```

In multi-app mode, `frontend`, `backend`, and `sync` fields at the top level are ignored. Only `server` and `apps` are used.

### Per-App Config

Each app directory must contain its own `.servitor.conf`:

```yaml
frontend:
  path: ./frontend

backend:
  path: ./backend/servitor-backend
  api_prefix: /api
  timeout: 3s

sync:
  enabled: true
  inactivity_delay: 30s
  max_interval: 5m
```

All paths in per-app configs are resolved relative to the app's directory and converted to absolute paths at load time.

### Routing

- Root `/` → HTML index page with links to all apps
- App `notes` with `api_prefix: /api` → static at `/notes/`, API at `/notes/api/`
- App `wiki` with `api_prefix: /api` → static at `/wiki/`, API at `/wiki/api/`
- The `/<name>/` prefix is stripped before forwarding to backend/static handlers

### Frontend Requirement

Frontends must use **relative** API paths (e.g. `fetch("api/types")` not `fetch("/api/types")`). The browser resolves relative URLs against the page's base URL (`/<name>/`), producing the correct prefixed path.

### Path Resolution

`LoadApps()` resolves all paths to absolute at load time:
- `appDir` → absolute (used as `WorkDir` for backend subprocess)
- `backend.Path` → absolute (so `exec.Command` finds it regardless of `WorkDir`)
- `frontend.Path` → absolute (for `http.FileServer`)

### Git Sync in Multi-App

Each app gets its own sync engine. The git repo is opened at the repository root. The `addPath` for each app is computed relative to the repo root (e.g. `apps/notes/data/`), while the fsnotify `watchDir` uses the absolute path.

### Project Structure (Multi-App)

```
my-repo/
├── .servitor.conf              # Top-level: server + apps list
├── apps/
│   ├── notes/
│   │   ├── .servitor.conf      # App config
│   │   ├── frontend/
│   │   ├── backend/
│   │   └── data/
│   └── wiki/
│       ├── .servitor.conf
│       ├── frontend/
│       ├── backend/
│       └── data/
└── .git/
```

Each app gets its own sync engine watching its own `data/` directory.

## Testing Strategy

- Unit tests with valid YAML → verify all fields populated
- Unit tests with missing required fields → verify specific error messages
- Unit tests with invalid YAML → verify parse error
- Unit tests with partial config → verify defaults applied
- Unit tests with edge values (port 0, negative durations) → verify validation
