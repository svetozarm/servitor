# Lessons Learned

## SyncEngine: watch path vs git add path

The `SyncEngine` uses `dataDir` for both fsnotify watching and passing to `syncer.Add()`. In production (running from repo root with `"data/"`), this works for both. In integration tests, fsnotify needs an absolute path (temp dir) while `wt.Add()` needs a path relative to the worktree root.

**Fix:** Added `NewSyncEngineWithPaths(syncer, watchDir, addPath, ...)` to separate the absolute watch path from the relative git add path. The original `NewSyncEngine` still works for the common case where both are the same.

## Multi-App Mode: path resolution

In multi-app mode, all paths (backend binary, frontend dir, app dir) must be resolved to **absolute paths** at config load time. The backend binary is spawned with `cmd.Dir` set to the app directory so it can find its `data/` locally — if the binary path were relative, `exec.Command` would resolve it relative to `cmd.Dir`, causing a double-nested path.

**Fix:** `LoadApps()` resolves `appDir`, `backend.Path`, and `frontend.Path` to absolute paths using `filepath.Abs()`.

## Multi-App Mode: git sync addPath

The git repo lives at the repository root, but each app's data is in a subdirectory (e.g. `apps/projects/data/`). The `addPath` passed to the sync engine must be relative to the repo root, not relative to the app dir.

**Fix:** Use `filepath.Rel(cwd, absoluteDataDir)` to compute the path relative to the repo root (where the git repo is opened). Note: `filepath.Rel(".", absolutePath)` does NOT work because `"."` is not resolved to an absolute path for the comparison.

## Multi-App Mode: frontend relative paths

In multi-app mode, apps are served under `/<name>/`. The app prefix is stripped before forwarding to the backend. Frontends must use **relative** API paths (e.g. `fetch("api/types")` not `fetch("/api/types")`), so the browser resolves them relative to the page's base URL (`/<name>/`).
