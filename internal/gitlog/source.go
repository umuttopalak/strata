package gitlog

import (
	"context"
	"time"
)

// Source produces a repository's history. The git CLI is the only
// implementation for now; a go-git fallback can satisfy the same interface.
type Source interface {
	// Bounds returns the time span of the history that Walk will visit.
	Bounds(ctx context.Context, opts LogOptions) (Bounds, error)
	// Baseline returns the line count of every file as it was just before
	// opts.Since, so a partial replay starts from the real sizes. It is empty
	// when Since is zero or predates the first commit.
	Baseline(ctx context.Context, opts LogOptions) ([]FileChange, error)
	// Walk calls fn for each mainline (first-parent) commit, oldest first.
	// Returning an error from fn stops the walk and Walk returns that error.
	Walk(ctx context.Context, opts LogOptions, fn func(Commit) error) error
}

// LogOptions narrows the history that is read.
type LogOptions struct {
	Since time.Time // zero means the whole history
}

// Bounds describes the history span in committer time.
type Bounds struct {
	First, Last time.Time
}

var _ Source = (*Repo)(nil)
