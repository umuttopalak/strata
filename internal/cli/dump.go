package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/umuttopalak/summit/internal/gitlog"
)

// dump prints one line per commit plus a summary; a debugging aid for the
// git reader that also shows how fast history can be streamed.
func dump(ctx context.Context, out io.Writer, src gitlog.Source, opts gitlog.LogOptions) error {
	w := bufio.NewWriter(out)
	defer w.Flush()

	start := time.Now()
	base, err := src.Baseline(ctx, opts)
	if err != nil {
		return err
	}
	var baseLines int
	for _, f := range base {
		baseLines += f.Added
	}
	if len(base) > 0 {
		fmt.Fprintf(w, "baseline: %d files, %d lines before %s\n",
			len(base), baseLines, opts.Since.Format("2006-01-02"))
	}

	var commits, files, added, deleted int
	var first time.Duration
	err = src.Walk(ctx, opts, func(c gitlog.Commit) error {
		if commits == 0 {
			first = time.Since(start)
		}
		commits++
		var a, d int
		for _, f := range c.Files {
			a += f.Added
			d += f.Deleted
		}
		files += len(c.Files)
		added += a
		deleted += d
		hash := c.Hash
		if len(hash) > 8 {
			hash = hash[:8]
		}
		_, err := fmt.Fprintf(w, "%s  %s  %-18.18s  +%-6d -%-6d %3d files  %s\n",
			c.AuthorTime.Format("2006-01-02"), hash, c.Author, a, d, len(c.Files), c.Subject)
		return err
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%d commits, %d file changes, +%d -%d lines (net %d)\n",
		commits, files, added, deleted, baseLines+added-deleted)
	fmt.Fprintf(w, "first commit after %s, total %s\n",
		first.Round(time.Millisecond), time.Since(start).Round(time.Millisecond))
	return nil
}
