package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestIntegration_SIGTERM_FlushesSync_CleanExit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build the binary
	binary := filepath.Join(t.TempDir(), "servitor")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "servitor")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Setup work directory with git repo, data dir, frontend dir
	workDir := t.TempDir()

	// Create bare remote
	bareDir := t.TempDir()
	_, err := git.PlainInit(bareDir, true)
	if err != nil {
		t.Fatal(err)
	}

	// Init local repo
	repo, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Add remote
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create directories
	dataDir := filepath.Join(workDir, "data")
	os.MkdirAll(dataDir, 0o755)
	frontendDir := filepath.Join(workDir, "frontend")
	os.MkdirAll(frontendDir, 0o755)
	os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<h1>test</h1>"), 0o644)

	// Initial commit so HEAD exists
	os.WriteFile(filepath.Join(dataDir, ".gitkeep"), []byte(""), 0o644)
	wt, _ := repo.Worktree()
	wt.Add("data/.gitkeep")
	wt.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	})
	repo.Push(&git.PushOptions{})

	// Write .servitor.conf with sync enabled and long timers (flush must happen on shutdown, not timer)
	conf := `server:
  port: 18937
  host: 127.0.0.1
frontend:
  path: ./frontend
backend:
  path: /bin/true
  api_prefix: /api
  timeout: 1s
sync:
  enabled: true
  inactivity_delay: 10s
  max_interval: 60s
`
	os.WriteFile(filepath.Join(workDir, ".servitor.conf"), []byte(conf), 0o644)

	// Start servitor
	cmd := exec.Command(binary)
	cmd.Dir = workDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Wait for process to be ready
	time.Sleep(200 * time.Millisecond)

	// Modify a file in data/ to make sync engine have pending changes
	os.WriteFile(filepath.Join(dataDir, "signal-test.txt"), []byte("written before SIGTERM"), 0o644)

	// Give the watcher time to detect the file change
	time.Sleep(100 * time.Millisecond)

	// Send SIGTERM — should trigger FlushSync before exit
	cmd.Process.Signal(syscall.SIGTERM)

	// Wait for process to exit
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean exit, got: %v", err)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process did not exit within timeout")
	}

	// Verify sync was flushed: check git log for auto-sync commit
	repo, _ = git.PlainOpen(workDir)
	log, err := repo.Log(&git.LogOptions{})
	if err != nil {
		t.Fatal(err)
	}

	c, err := log.Next()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(c.Message, "auto-sync:") {
		t.Errorf("expected auto-sync commit from flush, got %q", c.Message)
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root")
		}
		dir = parent
	}
}
