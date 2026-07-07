# Module: Self-Update

## Responsibility

Check for new releases on GitHub, download the platform-appropriate binary, verify its checksum, and replace the running binary in-place. Invoked via `servitor update`.

## Design

### Update Flow

```
servitor update
       │
       ▼
1. Check embedded version (fail if dev build)
2. Query GitHub API for latest release
3. Compare semver (exit if up-to-date)
4. Derive platform asset name (e.g., servitor_1.2.0_linux_amd64.tar.gz)
5. Download asset archive + checksums.txt
6. Verify SHA256 checksum
7. Extract binary from archive
8. Replace running binary via atomic rename
9. Clean up temp files
```

### Platform Asset Naming

```
servitor_<version>_<GOOS>_<GOARCH>.tar.gz   (linux, darwin)
servitor_<version>_<GOOS>_<GOARCH>.zip      (windows)
```

### Security

- Downloads restricted to HTTPS from allowlisted hosts (`github.com`, `objects.githubusercontent.com`)
- SHA256 checksum verification against `checksums.txt` from the release
- Max download size enforced (50 MB)
- Atomic binary replacement via temp file + rename
- Writability check before attempting download

### Error Cases

| Condition | Behaviour |
|---|---|
| No version embedded (dev build) | Exit: "update unavailable for development builds" |
| GitHub API unreachable | Exit with error |
| Already up-to-date | Print "already up to date: <version>" |
| Platform asset missing from release | `ErrAssetNotFound` |
| Binary directory not writable | `ErrPermission` |
| Checksum mismatch | `ErrChecksumMismatch` |
| Binary replacement fails | `ErrReplaceFailed` |

## Interface

```go
type ReleaseChecker interface {
    LatestRelease(ctx context.Context) (*Release, error)
    DownloadAsset(ctx context.Context, url string, dest string) error
}

type Release struct {
    Tag    string
    Assets []Asset
}

type Asset struct {
    Name string
    URL  string
}
```

## Public API

```go
type Result struct {
    OldVersion string
    NewVersion string
    UpToDate   bool
}

func Run(ctx context.Context, currentVersion string, checker ReleaseChecker) (*Result, error)
func NewGitHubClient(repo string) *GitHubClient
```

## Binary Replacement

- **Unix**: extract from `.tar.gz`, write to temp file in same directory, `chmod 0755`, `os.Rename` over existing binary
- **Windows**: extract from `.zip`, write to temp file, rename (handles locked-file semantics separately in `replace_windows.go`)

## Testing Strategy

- Unit test: semver comparison (isNewer)
- Unit test: asset name derivation per platform
- Unit test: checksum verification (valid, mismatch, missing entry)
- Integration test: mock GitHub API, verify full update flow
- Unit test: dev build (empty version) returns ErrDevBuild
- Unit test: already up-to-date returns Result.UpToDate = true
