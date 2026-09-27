package cli

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/umuttopalak/strata/internal/gitlog"
	"github.com/umuttopalak/strata/internal/render"
	"github.com/umuttopalak/strata/internal/timeline"
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

// dumpFrames prints every frame with its largest columns.
func dumpFrames(out io.Writer, tl *timeline.Timeline) error {
	w := bufio.NewWriter(out)
	defer w.Flush()
	for _, f := range tl.Frames {
		date := "          "
		if f.Caption != nil {
			date = f.Caption.Time.Format("2006-01-02")
		}
		fmt.Fprintf(w, "%6d  %s  %s\n", f.Commit, date, topColumns(f, tl.Columns, 5))
	}
	fmt.Fprintf(w, "\n%d commits in %d frames, %d columns\n", tl.Commits, len(tl.Frames), len(tl.Columns))
	if len(tl.Events) > 0 {
		fmt.Fprintf(w, "\n%d events:\n", len(tl.Events))
	}
	for _, e := range tl.Events {
		fmt.Fprintf(w, "  frame %4d  commit %6d  %s\n", e.Frame, e.Commit, render.EventText(e))
	}
	return nil
}

func topColumns(f timeline.Frame, cols []string, n int) string {
	idx := make([]int, len(f.Totals))
	for i := range idx {
		idx[i] = i
	}
	slices.SortFunc(idx, func(a, b int) int { return cmp.Compare(f.Totals[b], f.Totals[a]) })
	var parts []string
	for _, i := range idx[:min(n, len(idx))] {
		if f.Totals[i] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", cols[i], f.Totals[i]))
		}
	}
	return strings.Join(parts, " ")
}
