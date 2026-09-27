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
	"strconv"
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

// CloneProgress is one progress report from git during a clone.
type CloneProgress struct {
	Phase   string // "preparing", "receiving" or "resolving"
	Percent int
	Size    string // amount received so far, e.g. "1.15 MiB", while receiving
}

// Clone fetches the default branch of url, with its full history and tags,
// into dir as a bare repository: no working tree is checked out, which is
// all strata needs. If progress is not nil it is called as git reports
// progress; git's own output is never printed.
func Clone(ctx context.Context, url, dir string, progress func(CloneProgress)) error {
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
		cmd.Stderr = io.MultiWriter(&progressWriter{report: progress}, &stderr)
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

// progressLine matches git's progress lines, with or without "remote: ".
var progressLine = regexp.MustCompile(`(Counting objects|Compressing objects|Receiving objects|Resolving deltas):\s+(\d+)%(?:[^,]*,\s*([0-9.]+ [KMGT]?i?B))?`)

var phases = map[string]string{
	"Counting objects":    "preparing",
	"Compressing objects": "preparing",
	"Receiving objects":   "receiving",
	"Resolving deltas":    "resolving",
}

// progressWriter turns git's progress output, whose updates are separated
// by carriage returns, into CloneProgress reports.
type progressWriter struct {
	report  func(CloneProgress)
	pending []byte
}

func (w *progressWriter) Write(b []byte) (int, error) {
	w.pending = append(w.pending, b...)
	for {
		i := bytes.IndexAny(w.pending, "\r\n")
		if i < 0 {
			return len(b), nil
		}
		line := string(w.pending[:i])
		w.pending = w.pending[i+1:]
		if m := progressLine.FindStringSubmatch(line); m != nil {
			pct, _ := strconv.Atoi(m[2])
			w.report(CloneProgress{Phase: phases[m[1]], Percent: pct, Size: m[3]})
		}
	}
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
