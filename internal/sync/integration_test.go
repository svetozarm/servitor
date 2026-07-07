package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestIntegration_ModifyFile_CommitCreated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workDir, _ := setupTestRepo(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create data/ directory and initial commit for it
	dataDir := filepath.Join(workDir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, ".gitkeep"), []byte(""), 0o644)

	repo, _ := git.PlainOpen(workDir)
	wt, _ := repo.Worktree()
	wt.Add("data/.gitkeep")
	wt.Commit("add data dir", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})

	syncer, err := NewGoGitSyncer(workDir, logger)
	if err != nil {
		t.Fatal(err)
	}

	engine := NewSyncEngineWithPaths(syncer, dataDir, "data", 50*time.Millisecond, 500*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go engine.Start(ctx)
	time.Sleep(20 * time.Millisecond) // let watcher start

	// Modify a file in data/
	os.WriteFile(filepath.Join(dataDir, "notes.txt"), []byte("hello world"), 0o644)

	// Wait for inactivity timer to fire + some buffer
	time.Sleep(150 * time.Millisecond)

	// Verify a new commit exists with auto-sync message
	log, err := repo.Log(&git.LogOptions{})
	if err != nil {
		t.Fatal(err)
	}

	c, err := log.Next()
	if err != nil {
		t.Fatal(err)
	}

	if len(c.Message) < 10 || c.Message[:10] != "auto-sync:" {
		t.Errorf("expected auto-sync commit, got %q", c.Message)
	}
}

func TestIntegration_ForcePushOnDivergedRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	workDir, bareDir := setupTestRepo(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create data/ directory and initial commit
	dataDir := filepath.Join(workDir, "data")
	os.MkdirAll(dataDir, 0o755)
	os.WriteFile(filepath.Join(dataDir, ".gitkeep"), []byte(""), 0o644)

	repo, _ := git.PlainOpen(workDir)
	wt, _ := repo.Worktree()
	wt.Add("data/.gitkeep")
	wt.Commit("add data dir", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})
	repo.Push(&git.PushOptions{})

	// Create divergence: clone bare, commit, push from another clone
	cloneDir := t.TempDir()
	cloneRepo, _ := git.PlainClone(cloneDir, false, &git.CloneOptions{URL: bareDir})
	cloneWt, _ := cloneRepo.Worktree()
	os.WriteFile(filepath.Join(cloneDir, "diverge.txt"), []byte("diverge"), 0o644)
	cloneWt.Add("diverge.txt")
	cloneWt.Commit("divergent commit", &git.CommitOptions{
		Author: &object.Signature{Name: "other", Email: "other@test.com", When: time.Now()},
	})
	cloneRepo.Push(&git.PushOptions{})

	// Start sync engine on workDir
	syncer, err := NewGoGitSyncer(workDir, logger)
	if err != nil {
		t.Fatal(err)
	}

	engine := NewSyncEngineWithPaths(syncer, dataDir, "data", 50*time.Millisecond, 500*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go engine.Start(ctx)
	time.Sleep(20 * time.Millisecond)

	// Write a file to trigger sync — this will conflict with the diverged remote
	os.WriteFile(filepath.Join(dataDir, "local.txt"), []byte("local data"), 0o644)

	// Wait for sync to complete (inactivity + buffer for force push retry)
	time.Sleep(200 * time.Millisecond)

	// Verify the remote HEAD is our auto-sync commit (force push succeeded)
	bare, _ := git.PlainOpen(bareDir)
	log, _ := bare.Log(&git.LogOptions{})
	c, _ := log.Next()

	if !strings.HasPrefix(c.Message, "auto-sync:") {
		t.Errorf("expected auto-sync commit on remote after force push, got %q", c.Message)
	}
}
