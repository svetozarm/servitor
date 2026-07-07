package sync

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func setupTestRepo(t *testing.T) (workDir, bareDir string) {
	t.Helper()

	bareDir = t.TempDir()
	_, err := git.PlainInit(bareDir, true)
	if err != nil {
		t.Fatal(err)
	}

	workDir = t.TempDir()
	repo, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create initial commit so we have a HEAD
	wt, _ := repo.Worktree()
	f, _ := os.Create(filepath.Join(workDir, ".gitkeep"))
	f.Close()
	wt.Add(".gitkeep")
	wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})

	// Add bare as remote
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Push initial commit
	repo.Push(&git.PushOptions{})

	return workDir, bareDir
}

func TestGoGitSyncer_AddAndCommit(t *testing.T) {
	workDir, _ := setupTestRepo(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s, err := NewGoGitSyncer(workDir, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Create file in data/
	os.MkdirAll(filepath.Join(workDir, "data"), 0o755)
	os.WriteFile(filepath.Join(workDir, "data", "test.json"), []byte(`{"a":1}`), 0o644)

	if err := s.Add([]string{"data/"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit("test commit"); err != nil {
		t.Fatal(err)
	}

	// Verify commit exists
	repo, _ := git.PlainOpen(workDir)
	log, _ := repo.Log(&git.LogOptions{})
	c, _ := log.Next()
	if c.Message != "test commit" {
		t.Errorf("expected 'test commit', got %q", c.Message)
	}
}

func TestGoGitSyncer_Push(t *testing.T) {
	workDir, bareDir := setupTestRepo(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s, err := NewGoGitSyncer(workDir, logger)
	if err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(filepath.Join(workDir, "data"), 0o755)
	os.WriteFile(filepath.Join(workDir, "data", "file.txt"), []byte("hello"), 0o644)
	s.Add([]string{"data/"})
	s.Commit("push test")

	if err := s.Push(false); err != nil {
		t.Fatal(err)
	}

	// Verify remote has the commit
	bare, _ := git.PlainOpen(bareDir)
	log, _ := bare.Log(&git.LogOptions{})
	c, _ := log.Next()
	if c.Message != "push test" {
		t.Errorf("expected 'push test', got %q", c.Message)
	}
}

func TestGoGitSyncer_ForcePush(t *testing.T) {
	workDir, bareDir := setupTestRepo(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s, err := NewGoGitSyncer(workDir, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Create divergence: clone bare to another dir, commit, push
	cloneDir := t.TempDir()
	cloneRepo, _ := git.PlainClone(cloneDir, false, &git.CloneOptions{URL: bareDir})
	cloneWt, _ := cloneRepo.Worktree()
	os.WriteFile(filepath.Join(cloneDir, "diverge.txt"), []byte("diverge"), 0o644)
	cloneWt.Add("diverge.txt")
	cloneWt.Commit("divergent commit", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})
	cloneRepo.Push(&git.PushOptions{})

	// Now commit in workDir and force push
	os.MkdirAll(filepath.Join(workDir, "data"), 0o755)
	os.WriteFile(filepath.Join(workDir, "data", "local.txt"), []byte("local"), 0o644)
	s.Add([]string{"data/"})
	s.Commit("local commit")

	if err := s.Push(true); err != nil {
		t.Fatal(err)
	}

	// Verify remote HEAD is our commit
	bare, _ := git.PlainOpen(bareDir)
	log, _ := bare.Log(&git.LogOptions{})
	c, _ := log.Next()
	if c.Message != "local commit" {
		t.Errorf("expected 'local commit', got %q", c.Message)
	}
}

func TestGoGitSyncer_NoChanges(t *testing.T) {
	workDir, _ := setupTestRepo(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s, err := NewGoGitSyncer(workDir, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Add with no changes should not error
	if err := s.Add([]string{"data/"}); err != nil {
		t.Fatal(err)
	}
	// Commit with nothing staged should return ErrNoChanges or similar
	err = s.Commit("empty")
	if err == nil {
		t.Error("expected error on empty commit")
	}
}
