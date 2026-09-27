package gitlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrCloneFailed means a remote repository could not be fetched.
var ErrCloneFailed = errors.New("could not clone the repository")

// knownHost matches "github.com/owner/repo" style arguments without a scheme.
var knownHost = regexp.MustCompile(`^(github\.com|gitlab\.com|codeberg\.org|bitbucket\.org)/[^/]+/[^/]+`)

// IsRemote reports whether arg names a remote repository rather than a
// local path: a URL, an scp-style "git@host:owner/repo", or a well-known
// host followed by owner/repo.
func IsRemote(arg string) bool {
	return strings.Contains(arg, "://") || strings.HasPrefix(arg, "git@") || knownHost.MatchString(arg)
}

// RemoteURL turns "github.com/owner/repo" into an https URL and leaves
// full URLs alone.
func RemoteURL(arg string) string {
	if strings.Contains(arg, "://") || strings.HasPrefix(arg, "git@") {
		return arg
	}
	return "https://" + arg
}

// RepoName is the last path element of a remote, without ".git".
func RepoName(url string) string {
	url = strings.TrimRight(url, "/")
	if i := strings.LastIndexAny(url, "/:"); i >= 0 {
		url = url[i+1:]
	}
	return strings.TrimSuffix(url, ".git")
}

// Clone fetches the default branch of url, with its full history and tags,
// into dir as a bare repository: no working tree is checked out, which is
// all strata needs. Git's progress goes to progress when it is not nil.
func Clone(ctx context.Context, url, dir string, progress io.Writer) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return ErrGitNotFound
	}
	args := []string{"clone", "--bare", "--single-branch", "--quiet"}
	if progress != nil {
		args = append(args[:3], "--progress")
	}
	cmd := exec.CommandContext(ctx, gitPath, append(args, "--", url, dir)...)
	// Never wait for a password prompt: private repositories just fail.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if progress != nil {
		cmd.Stderr = io.MultiWriter(progress, &stderr)
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "fatal: "); i >= 0 {
			msg = strings.TrimSpace(msg[i+len("fatal: "):])
		}
		return fmt.Errorf("%s: %w: %s", url, ErrCloneFailed, msg)
	}
	return nil
}

// RemoveClone deletes a clone made by Clone. Git writes its object files
// read-only, which Windows refuses to delete, so they are made writable
// first.
func RemoveClone(dir string) error {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			_ = os.Chmod(path, 0o644)
		}
		return nil
	})
	return os.RemoveAll(dir)
}
