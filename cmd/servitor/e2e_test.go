package main

import (
	"fmt"
	"io"
	"net/http"
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

func TestE2E_FullSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in short mode")
	}

	// Build servitor binary
	binary := filepath.Join(t.TempDir(), "servitor")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(findModuleRoot(t), "cmd", "servitor")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Setup work directory
	workDir := t.TempDir()

	// Create bare remote
	bareDir := t.TempDir()
	if _, err := git.PlainInit(bareDir, true); err != nil {
		t.Fatal(err)
	}

	// Init local repo
	repo, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatal(err)
	}
	repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	})

	// Create directories
	dataDir := filepath.Join(workDir, "data")
	os.MkdirAll(dataDir, 0o755)
	frontendDir := filepath.Join(workDir, "frontend")
	os.MkdirAll(frontendDir, 0o755)

	// Create frontend file
	os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<h1>servitor e2e</h1>"), 0o644)

	// Create backend script
	backendPath := filepath.Join(workDir, "backend.sh")
	os.WriteFile(backendPath, []byte(`#!/bin/sh
cat > /dev/null
printf "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 15\r\n\r\n{\"status\":\"ok\"}"
`), 0o755)

	// Initial commit
	os.WriteFile(filepath.Join(dataDir, ".gitkeep"), []byte(""), 0o644)
	wt, _ := repo.Worktree()
	wt.Add(".")
	wt.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	})
	repo.Push(&git.PushOptions{})

	// Write config
	port := 19832
	conf := fmt.Sprintf(`server:
  port: %d
  host: 127.0.0.1
frontend:
  path: ./frontend
backend:
  path: %s
  api_prefix: /api
  timeout: 3s
sync:
  enabled: true
  inactivity_delay: 100ms
  max_interval: 5s
`, port, backendPath)
	os.WriteFile(filepath.Join(workDir, ".servitor.conf"), []byte(conf), 0o644)

	// Start servitor
	cmd := exec.Command(binary)
	cmd.Dir = workDir
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for server to be ready
	ready := false
	for i := 0; i < 30; i++ {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(baseURL + "/")
		if err == nil {
			resp.Body.Close()
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("server did not start in time")
	}

	// 1. GET / → verify frontend
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "servitor e2e") {
		t.Fatalf("GET / body = %q, want to contain 'servitor e2e'", body)
	}

	// 2. POST /api/data → verify backend response
	resp, err = http.Post(baseURL+"/api/data", "application/json", strings.NewReader(`{"test":true}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("POST /api/data status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("POST /api/data body = %q, want JSON with status ok", body)
	}

	// 3. Write file to data/ and wait for sync
	os.WriteFile(filepath.Join(dataDir, "e2e-test.txt"), []byte("end-to-end data"), 0o644)

	// Wait for inactivity sync (100ms delay + buffer)
	time.Sleep(400 * time.Millisecond)

	// 4. Verify git log has auto-sync commit
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
		t.Fatalf("expected auto-sync commit, got %q", c.Message)
	}

	// 5. SIGTERM → verify clean exit
	cmd.Process.Signal(syscall.SIGTERM)

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
}
