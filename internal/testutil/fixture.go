// Package testutil builds small throwaway Git repositories for tests.
package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Repo is a Git repository in a temporary directory whose commits get
// fixed timestamps, so tests are deterministic.
type Repo struct {
	t   testing.TB
	Dir string
}

// NewRepo creates an empty repository isolated from the user's Git config.
func NewRepo(t testing.TB) *Repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	r := &Repo{t: t, Dir: t.TempDir()}
	r.Git("init", "-q", "-b", "main")
	return r
}

// Git runs a git command in the repository and returns its stdout.
func (r *Repo) Git(args ...string) string {
	return r.gitEnv(nil, args...)
}

func (r *Repo) gitEnv(env []string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.Dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test Author", "GIT_AUTHOR_EMAIL=author@example.com",
		"GIT_COMMITTER_NAME=Test Author", "GIT_COMMITTER_EMAIL=author@example.com",
		"LC_ALL=C")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// Write creates or overwrites a file (creating parent directories).
func (r *Repo) Write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.Dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// Remove deletes a file from the working tree.
func (r *Repo) Remove(path string) {
	r.t.Helper()
	if err := os.Remove(filepath.Join(r.Dir, filepath.FromSlash(path))); err != nil {
		r.t.Fatal(err)
	}
}

// Commit stages everything and commits it with both dates set to at.
// It returns the new commit's hash.
func (r *Repo) Commit(msg string, at time.Time) string {
	r.t.Helper()
	date := fmt.Sprintf("%d +0000", at.Unix())
	r.Git("add", "-A")
	r.gitEnv([]string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date},
		"commit", "-q", "--allow-empty", "-m", msg)
	return strings.TrimSpace(r.Git("rev-parse", "HEAD"))
}

// Merge merges branch into the current branch with a merge commit dated at.
// If resolve is non-nil it runs before committing, so tests can resolve
// conflicts or make an "evil merge" that changes extra lines.
func (r *Repo) Merge(branch, msg string, at time.Time, resolve func()) {
	r.t.Helper()
	cmd := exec.Command("git", "-C", r.Dir, "merge", "-q", "--no-ff", "--no-commit", branch)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	_ = cmd.Run() // a conflict exits non-zero; Commit fails if it is left unresolved
	if resolve != nil {
		resolve()
	}
	r.Commit(msg, at)
}

// Lines returns n newline-terminated lines.
func Lines(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// Day returns midnight UTC of the given date, for readable test timelines.
func Day(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
