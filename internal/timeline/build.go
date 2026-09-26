package timeline

import (
	"context"

	"github.com/umuttopalak/summit/internal/gitlog"
)

// Build reads the whole history from src into tl. It is meant to run in its
// own goroutine: tl fills up frame by frame and is marked done at the end,
// with the error (if any) available from tl.Done.
func Build(ctx context.Context, src gitlog.Source, log gitlog.LogOptions, opts Options, tl *Timeline) (err error) {
	defer func() { tl.finish(err) }()

	bounds, err := src.Bounds(ctx, log)
	if err != nil {
		return err
	}
	base, err := src.Baseline(ctx, log)
	if err != nil {
		return err
	}

	b := NewBuilder(bounds, opts, tl)
	b.Seed(base)
	err = src.Walk(ctx, log, func(c gitlog.Commit) error {
		b.Add(c)
		return nil
	})
	if err != nil {
		return err
	}
	b.Finish()
	return nil
}
