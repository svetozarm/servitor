package scaffold

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const DefaultTemplateRepo = "https://github.com/svetozarm/servitor-templates.git"

const defaultConfig = `server:
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
`

type Options struct {
	TemplateName string
	RepoURL      string
	TargetDir    string
	UpstreamURL  string
}

func Init(opts Options, logger *slog.Logger) error {
	if opts.TemplateName == "" {
		opts.TemplateName = "default"
	}
	if opts.RepoURL == "" {
		opts.RepoURL = DefaultTemplateRepo
	}
	if opts.TargetDir == "" {
		opts.TargetDir = "."
	}

	tmpDir, err := cloneTemplateRepo(opts.RepoURL)
	if err != nil {
		return fmt.Errorf("clone template: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	templateDir := filepath.Join(tmpDir, opts.TemplateName)
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		return fmt.Errorf("template '%s' not found in repository", opts.TemplateName)
	}

	if err := copyTemplateFiles(templateDir, opts.TargetDir, logger); err != nil {
		return err
	}

	confPath := filepath.Join(opts.TargetDir, ".servitor.conf")
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		if err := os.WriteFile(confPath, []byte(defaultConfig), 0644); err != nil {
			return err
		}
	}

	_, openErr := git.PlainOpenWithOptions(opts.TargetDir, &git.PlainOpenOptions{DetectDotGit: true})
	if openErr != nil {
		repo, err := git.PlainInitWithOptions(opts.TargetDir, &git.PlainInitOptions{
			InitOptions: git.InitOptions{
				DefaultBranch: plumbing.NewBranchReferenceName("main"),
			},
		})
		if err != nil {
			return fmt.Errorf("git init: %w", err)
		}

		if opts.UpstreamURL != "" {
			_, err := repo.CreateRemote(&gitconfig.RemoteConfig{
				Name: "origin",
				URLs: []string{opts.UpstreamURL},
			})
			if err != nil {
				return fmt.Errorf("add remote: %w", err)
			}
		}

		wt, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("worktree: %w", err)
		}
		if _, err := wt.Add("."); err != nil {
			return fmt.Errorf("git add: %w", err)
		}
		if _, err := wt.Commit("Initial commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "servitor",
				Email: "servitor@localhost",
				When:  time.Now(),
			},
		}); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
	}

	return nil
}

func cloneTemplateRepo(url string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "servitor-template-*")
	if err != nil {
		return "", err
	}
	_, err = git.PlainClone(tmpDir, false, &git.CloneOptions{
		URL:   url,
		Depth: 1,
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}
	return tmpDir, nil
}

func copyTemplateFiles(src, dst string, logger *slog.Logger) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		if relPath == "." {
			return nil
		}
		targetPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		if _, err := os.Stat(targetPath); err == nil {
			logger.Warn("skipping existing file", "path", relPath)
			return nil
		}

		return copyFile(path, targetPath)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
