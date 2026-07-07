# Module: Scaffolding

## Responsibility

Implement `servitor init [template]` — clone a template repository and generate the standard project directory structure with a default `.servitor.conf`.

## Requirements Covered

REQ-E-007, REQ-E-008, REQ-X-008

## Design

### Template Repository

- Default URL: `https://github.com/svetozarm/servitor-templates.git`
- Override via `--repo <url>` flag
- Repository contains one directory per template variant (e.g., `default/`, `notes-app/`)

### Behaviour

```
servitor init              → clone repo, copy "default" template
servitor init notes-app    → clone repo, copy "notes-app" template
servitor init --repo URL   → use custom repo URL
```

The user is prompted for an upstream git repo URL (stdin). If provided, it is configured as the `origin` remote.

### Steps

1. Parse CLI arguments (template name, `--repo` flag)
2. Prompt user for upstream git URL (can be left empty)
3. Clone template repository into a temp directory (using go-git, depth 1)
4. Locate the template subdirectory (default: `default/`)
5. Copy template files to current directory, skipping files that already exist
6. Generate `.servitor.conf` with default values (if not already present)
7. If `.git/` doesn't exist:
   a. `git init` with `main` as default branch
   b. Add `origin` remote if upstream URL was provided
   c. `git add .` + commit ("Initial commit")
8. Clean up temp directory

### Generated Structure

```
./
├── .servitor.conf
├── frontend/
│   ├── index.html
│   ├── css/
│   └── js/
├── backend/
└── data/
```

### Error Cases

| Condition | Behaviour |
|---|---|
| Template repo unreachable | Exit with error: "clone template: ..." |
| Named template not found in repo | Exit with error: "template '<name>' not found in repository" |
| File already exists | Skip it, log warning |
| No network | Exit with error (same as unreachable) |

## Public API

```go
package scaffold

type Options struct {
    TemplateName string
    RepoURL      string
    TargetDir    string
    UpstreamURL  string
}

func Init(opts Options, logger *slog.Logger) error
```

## Configuration Defaults Template

Generated `.servitor.conf`:

```yaml
server:
  port: 8080
  host: 127.0.0.1

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

## Testing Strategy

- Integration test: run Init in temp dir, verify directory structure created
- Integration test: run with named template, verify correct files copied
- Unit test: existing files are not overwritten
- Unit test: unreachable repo returns expected error
- Unit test: missing template name in repo returns expected error
