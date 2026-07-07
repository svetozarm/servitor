package scaffold_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "servitor")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/svetozarm/servitor/cmd/servitor")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func setupTestTemplateRepo(t *testing.T, variants map[string]map[string]string) string {
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

func TestIntegration_ServitorInit_CreatesStructure(t *testing.T) {
	bin := buildBinary(t)
	repoDir := setupTestTemplateRepo(t, map[string]map[string]string{
		"default": {
			"frontend/index.html": "<h1>hello</h1>",
			"backend/.gitkeep":    "",
			"data/.gitkeep":       "",
		},
	})
	target := t.TempDir()

	cmd := exec.Command(bin, "init", "--repo", repoDir)
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("servitor init failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "project initialised successfully") {
		t.Fatalf("unexpected output: %s", out)
	}

	// Verify structure
	for _, path := range []string{
		"frontend/index.html",
		"backend/.gitkeep",
		"data/.gitkeep",
		".servitor.conf",
		".git",
	} {
		if _, err := os.Stat(filepath.Join(target, path)); err != nil {
			t.Errorf("expected %s to exist", path)
		}
	}
}

func TestIntegration_ServitorInit_NamedTemplate(t *testing.T) {
	bin := buildBinary(t)
	repoDir := setupTestTemplateRepo(t, map[string]map[string]string{
		"default": {"default.txt": "d"},
		"blog":    {"blog.txt": "b"},
	})
	target := t.TempDir()

	cmd := exec.Command(bin, "init", "blog", "--repo", repoDir)
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("servitor init blog failed: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(target, "blog.txt")); err != nil {
		t.Error("blog.txt not found")
	}
	if _, err := os.Stat(filepath.Join(target, "default.txt")); err == nil {
		t.Error("default.txt should not exist")
	}
}

func TestIntegration_ServitorInit_TemplateNotFound(t *testing.T) {
	bin := buildBinary(t)
	repoDir := setupTestTemplateRepo(t, map[string]map[string]string{
		"default": {"a.txt": "x"},
	})
	target := t.TempDir()

	cmd := exec.Command(bin, "init", "nonexistent", "--repo", repoDir)
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for missing template")
	}
	if !strings.Contains(string(out), "template 'nonexistent' not found") {
		t.Fatalf("unexpected error output: %s", out)
	}
}

func TestIntegration_ServitorInit_RepoUnreachable(t *testing.T) {
	bin := buildBinary(t)
	target := t.TempDir()

	cmd := exec.Command(bin, "init", "--repo", "/nonexistent/repo/path")
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for unreachable repo")
	}
	if !strings.Contains(string(out), "error") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestIntegration_ServitorInit_DoesNotOverwriteExisting(t *testing.T) {
	bin := buildBinary(t)
	repoDir := setupTestTemplateRepo(t, map[string]map[string]string{
		"default": {"myfile.txt": "new"},
	})
	target := t.TempDir()
	os.WriteFile(filepath.Join(target, "myfile.txt"), []byte("original"), 0644)

	cmd := exec.Command(bin, "init", "--repo", repoDir)
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("servitor init failed: %v\n%s", err, out)
	}

	data, _ := os.ReadFile(filepath.Join(target, "myfile.txt"))
	if string(data) != "original" {
		t.Fatalf("file was overwritten: got %q", data)
	}
}
