package gitlog

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func sinceArgs(opts LogOptions) []string {
	if opts.Since.IsZero() {
		return nil
	}
	return []string{"--since=" + opts.Since.Format(time.RFC3339)}
}

// Walk streams `git log --reverse --numstat`, so memory use does not grow
// with the size of the history.
//
// It follows the first-parent (mainline) history and diffs each merge
// against its first parent. Summing those diffs reproduces the tree exactly;
// summing every non-merge commit instead drifts, because conflict
// resolutions and changes applied on both sides live only in merges.
func (r *Repo) Walk(ctx context.Context, opts LogOptions, fn func(Commit) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	args := []string{"-c", "core.quotePath=false", "log",
		"--reverse", "--first-parent", "-m", "--no-renames", "--numstat", logFormat}
	args = append(append(args, sinceArgs(opts)...), "HEAD")

	cmd := r.command(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting git log: %w", err)
	}

	parseErr := Parse(stdout, func(c Commit) error {
		// Once cancelled, git's output may be cut mid-commit; never hand
		// that partial commit to fn.
		if err := ctx.Err(); err != nil {
			return err
		}
		return fn(c)
	})
	if parseErr != nil {
		cancel() // stop git instead of waiting for it to finish writing
	}
	waitErr := cmd.Wait()

	switch {
	case parseErr != nil:
		return parseErr
	case ctx.Err() != nil:
		return ctx.Err()
	case waitErr != nil:
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("git log: %s", msg)
		}
		return fmt.Errorf("git log: %w", waitErr)
	}
	return nil
}

// Bounds reads the first and last commit times without walking file stats.
func (r *Repo) Bounds(ctx context.Context, opts LogOptions) (Bounds, error) {
	last, err := r.output(ctx, "log", "-1", "--format=%ct", "HEAD")
	if err != nil {
		return Bounds{}, err
	}
	roots, err := r.output(ctx, "log", "--max-parents=0", "--format=%ct", "HEAD")
	if err != nil {
		return Bounds{}, err
	}

	var b Bounds
	if b.Last, err = parseUnix(last); err != nil {
		return Bounds{}, err
	}
	for _, line := range strings.Fields(roots) {
		t, err := parseUnix(line)
		if err != nil {
			return Bounds{}, err
		}
		if b.First.IsZero() || t.Before(b.First) {
			b.First = t
		}
	}
	if opts.Since.After(b.First) {
		b.First = opts.Since
	}
	if b.First.After(b.Last) {
		return Bounds{}, fmt.Errorf("since %s: %w", opts.Since.Format("2006-01-02"), ErrNoCommits)
	}
	return b, nil
}

// Baseline diffs the empty tree against the last commit before opts.Since;
// every added line in that diff is a line that already existed.
func (r *Repo) Baseline(ctx context.Context, opts LogOptions) ([]FileChange, error) {
	if opts.Since.IsZero() {
		return nil, nil
	}
	rev, err := r.output(ctx, "rev-list", "-1",
		"--before="+opts.Since.Format(time.RFC3339), "HEAD")
	if err != nil {
		return nil, err
	}
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return nil, nil
	}
	// The empty tree's id depends on the hash algorithm (SHA-1 or SHA-256).
	empty, err := r.run(ctx, "", "hash-object", "-t", "tree", "--stdin")
	if err != nil {
		return nil, err
	}
	out, err := r.output(ctx, "-c", "core.quotePath=false", "diff",
		"--numstat", "--no-renames", strings.TrimSpace(empty), rev)
	if err != nil {
		return nil, err
	}

	var files []FileChange
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		fc, ok, err := parseNumstat(line)
		if err != nil {
			return nil, err
		}
		if ok {
			files = append(files, fc)
		}
	}
	return files, nil
}

func parseUnix(s string) (time.Time, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("unexpected git timestamp %q", s)
	}
	return time.Unix(n, 0), nil
}
