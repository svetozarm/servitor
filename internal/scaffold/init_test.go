package scaffold

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"time"
)

func setupTemplateRepo(t *testing.T, variants map[string]map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := repo.Worktree()

	for variant, files := range variants {
		varDir := filepath.Join(dir, variant)
		os.MkdirAll(varDir, 0755)
		for name, content := range files {
			fpath := filepath.Join(varDir, name)
			os.MkdirAll(filepath.Dir(fpath), 0755)
			os.WriteFile(fpath, []byte(content), 0644)
		}
	}

	wt.Add(".")
	wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t", When: time.Now()},
	})
	return dir
}

func TestInit_DefaultTemplate(t *testing.T) {
	repoDir := setupTemplateRepo(t, map[string]map[string]string{
		"default": {"index.html": "<h1>hello</h1>", "sub/app.js": "console.log('hi')"},
	})
	target := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	err := Init(Options{RepoURL: repoDir, TargetDir: target}, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Verify files copied
	data, err := os.ReadFile(filepath.Join(target, "index.html"))
	if err != nil {
		t.Fatal("index.html not created")
	}
	if string(data) != "<h1>hello</h1>" {
		t.Fatalf("unexpected content: %s", data)
	}
	data, err = os.ReadFile(filepath.Join(target, "sub/app.js"))
	if err != nil {
		t.Fatal("sub/app.js not created")
	}
	if string(data) != "console.log('hi')" {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestInit_NamedTemplate(t *testing.T) {
	repoDir := setupTemplateRepo(t, map[string]map[string]string{
		"default": {"a.txt": "default"},
		"blog":    {"b.txt": "blog"},
	})
	target := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	err := Init(Options{TemplateName: "blog", RepoURL: repoDir, TargetDir: target}, logger)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(target, "b.txt")); err != nil {
		t.Fatal("blog template file not found")
	}
	if _, err := os.Stat(filepath.Join(target, "a.txt")); err == nil {
		t.Fatal("default template file should not exist")
	}
}

func TestInit_TemplateNotFound(t *testing.T) {
	repoDir := setupTemplateRepo(t, map[string]map[string]string{
		"default": {"a.txt": "x"},
	})
	target := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	err := Init(Options{TemplateName: "nonexistent", RepoURL: repoDir, TargetDir: target}, logger)
	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

func TestInit_RepoUnreachable(t *testing.T) {
	target := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	err := Init(Options{RepoURL: "/nonexistent/repo", TargetDir: target}, logger)
	if err == nil {
		t.Fatal("expected error for unreachable repo")
	}
}

func TestInit_SkipsExistingFiles(t *testing.T) {
	repoDir := setupTemplateRepo(t, map[string]map[string]string{
		"default": {"existing.txt": "new content"},
	})
	target := t.TempDir()
	os.WriteFile(filepath.Join(target, "existing.txt"), []byte("original"), 0644)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	err := Init(Options{RepoURL: repoDir, TargetDir: target}, logger)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(target, "existing.txt"))
	if string(data) != "original" {
		t.Fatalf("existing file was overwritten: %s", data)
	}
}

func TestInit_GeneratesDefaultConfig(t *testing.T) {
	repoDir := setupTemplateRepo(t, map[string]map[string]string{
		"default": {"index.html": "hi"},
	})
	target := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	err := Init(Options{RepoURL: repoDir, TargetDir: target}, logger)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(target, ".servitor.conf")); err != nil {
		t.Fatal(".servitor.conf not generated")
	}
}

func TestInit_InitsGitRepo(t *testing.T) {
	repoDir := setupTemplateRepo(t, map[string]map[string]string{
		"default": {"index.html": "hi"},
	})
	target := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	err := Init(Options{RepoURL: repoDir, TargetDir: target}, logger)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatal(".git not initialised")
	}

	repo, err := git.PlainOpen(target)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err == nil && head.Name().Short() != "main" {
		t.Fatalf("expected default branch 'main', got %q", head.Name().Short())
	}
}

func TestInit_SkipsGitInitInsideExistingRepo(t *testing.T) {
	repoDir := setupTemplateRepo(t, map[string]map[string]string{
		"default": {"index.html": "hi"},
	})

	// Create a parent git repo, then init inside a subdirectory
	parentDir := t.TempDir()
	parentRepo, err := git.PlainInit(parentDir, false)
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(parentDir, "subproject")
	os.MkdirAll(target, 0755)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	err = Init(Options{RepoURL: repoDir, TargetDir: target, UpstreamURL: "https://example.com/repo.git"}, logger)
	if err != nil {
		t.Fatal(err)
	}

	// .git should NOT be created in the subdirectory
	if _, err := os.Stat(filepath.Join(target, ".git")); err == nil {
		t.Fatal(".git should not be created inside an existing repo")
	}

	// upstream remote should NOT be added to the parent repo
	remotes, _ := parentRepo.Remotes()
	if len(remotes) != 0 {
		t.Fatalf("expected no remotes on parent repo, got %d", len(remotes))
	}
}
