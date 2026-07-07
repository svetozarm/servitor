package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func setupTemplateRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := repo.Worktree()

	defDir := filepath.Join(dir, "default")
	os.MkdirAll(defDir, 0755)
	os.WriteFile(filepath.Join(defDir, "index.html"), []byte("<h1>servitor</h1>"), 0644)

	blogDir := filepath.Join(dir, "blog")
	os.MkdirAll(blogDir, 0755)
	os.WriteFile(filepath.Join(blogDir, "blog.html"), []byte("<h1>blog</h1>"), 0644)

	wt.Add(".")
	wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t", When: time.Now()},
	})
	return dir
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "servitor")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(".", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s\n%s", err, out)
	}
	return bin
}

func TestCLI_InitDefault(t *testing.T) {
	bin := buildBinary(t)
	repoDir := setupTemplateRepo(t)
	target := t.TempDir()

	cmd := exec.Command(bin, "init", "--repo", repoDir)
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("servitor init failed: %s\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(target, "index.html")); err != nil {
		t.Fatal("index.html not created")
	}
	if _, err := os.Stat(filepath.Join(target, ".servitor.conf")); err != nil {
		t.Fatal(".servitor.conf not generated")
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatal(".git not initialised")
	}
}

func TestCLI_InitNamedTemplate(t *testing.T) {
	bin := buildBinary(t)
	repoDir := setupTemplateRepo(t)
	target := t.TempDir()

	cmd := exec.Command(bin, "init", "blog", "--repo", repoDir)
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("servitor init blog failed: %s\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(target, "blog.html")); err != nil {
		t.Fatal("blog.html not created")
	}
}

func TestCLI_InitInvalidTemplate(t *testing.T) {
	bin := buildBinary(t)
	repoDir := setupTemplateRepo(t)
	target := t.TempDir()

	cmd := exec.Command(bin, "init", "nonexistent", "--repo", repoDir)
	cmd.Dir = target
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}
