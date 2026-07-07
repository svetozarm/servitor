package sync

import (
	"errors"
	"log/slog"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var ErrNoChanges = errors.New("nothing to commit")

// GoGitSyncer implements GitSyncer using go-git.
type GoGitSyncer struct {
	repo   *git.Repository
	logger *slog.Logger
}

func NewGoGitSyncer(repoPath string, logger *slog.Logger) (*GoGitSyncer, error) {
	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}
	return &GoGitSyncer{repo: repo, logger: logger}, nil
}

func (g *GoGitSyncer) Pull() error {
	wt, err := g.repo.Worktree()
	if err != nil {
		return err
	}
	err = wt.Pull(&git.PullOptions{})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

func (g *GoGitSyncer) Add(paths []string) error {
	wt, err := g.repo.Worktree()
	if err != nil {
		return err
	}
	for _, p := range paths {
		if _, err := wt.Add(p); err != nil {
			g.logger.Debug("add skipped", "path", p, "err", err)
		}
	}
	return nil
}

func (g *GoGitSyncer) Commit(message string) error {
	wt, err := g.repo.Worktree()
	if err != nil {
		return err
	}
	status, err := wt.Status()
	if err != nil {
		return err
	}
	if status.IsClean() {
		return ErrNoChanges
	}
	_, err = wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "servitor",
			Email: "servitor@localhost",
			When:  time.Now(),
		},
	})
	return err
}

func (g *GoGitSyncer) Push(force bool) error {
	opts := &git.PushOptions{Force: force}
	err := g.repo.Push(opts)
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}
