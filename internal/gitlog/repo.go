// Package gitlog reads commit history from a Git repository.
package gitlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ErrGitNotFound = errors.New("git executable not found in PATH")
	ErrPathMissing = errors.New("path does not exist")
	ErrNotDir      = errors.New("path is not a directory")
	ErrNotRepo     = errors.New("not a Git repository")
	ErrEmptyRepo   = errors.New("repository has no commits yet")
	ErrNoCommits   = errors.New("no commits in the selected time range")
)

// Repo is a validated, non-empty Git working tree.
type Repo struct {
	Root string // absolute path of the top-level directory
	git  string // path to the git executable
}

// OpenRepo checks that git is installed and that path is inside a Git
// repository with at least one commit. Bare repositories (such as those
// made by Clone) work too; their Root is the git directory.
func OpenRepo(ctx context.Context, path string) (*Repo, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, ErrGitNotFound
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	info, err := os.Stat(abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("%s: %w", path, ErrPathMissing)
	case err != nil:
		return nil, fmt.Errorf("%s: %w", path, err)
	case !info.IsDir():
		return nil, fmt.Errorf("%s: %w", path, ErrNotDir)
	}

	r := &Repo{Root: abs, git: gitPath}
	bare, err := r.output(ctx, "rev-parse", "--is-bare-repository")
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return nil, fmt.Errorf("%s: %w", path, ErrNotRepo)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	top := "--show-toplevel"
	if strings.TrimSpace(bare) == "true" {
		top = "--absolute-git-dir"
	}
	out, err := r.output(ctx, "rev-parse", top)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	r.Root = filepath.Clean(strings.TrimSpace(out))

	if _, err := r.output(ctx, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return nil, fmt.Errorf("%s: %w", r.Root, ErrEmptyRepo)
	}
	return r, nil
}

// command builds a git invocation rooted at the repository. LC_ALL=C keeps
// git's messages in English so error matching works on localized systems.
func (r *Repo) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.git, append([]string{"-C", r.Root}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	return cmd
}

// output runs git and returns stdout; on failure the error carries stderr.
func (r *Repo) output(ctx context.Context, args ...string) (string, error) {
	return r.run(ctx, "", args...)
}

func (r *Repo) run(ctx context.Context, stdin string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := r.command(ctx, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		name := args[0]
		if name == "-c" && len(args) > 2 {
			name = args[2]
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %s", name, msg)
		}
		return "", fmt.Errorf("git %s: %w", name, err)
	}
	return stdout.String(), nil
}

// Shallow reports whether the clone has only part of the history, as
// actions/checkout makes by default. Errors count as not shallow.
func (r *Repo) Shallow(ctx context.Context) bool {
	out, err := r.output(ctx, "rev-parse", "--is-shallow-repository")
	return err == nil && strings.TrimSpace(out) == "true"
}
