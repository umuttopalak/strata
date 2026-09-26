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

	"github.com/umuttopalak/summit/internal/gitlog"
	"github.com/umuttopalak/summit/internal/timeline"
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

// dumpFrames builds the timeline and prints every frame that has commits,
// with its largest columns.
func dumpFrames(ctx context.Context, out io.Writer, src gitlog.Source, log gitlog.LogOptions, opts timeline.Options) error {
	w := bufio.NewWriter(out)
	defer w.Flush()

	start := time.Now()
	tl := &timeline.Timeline{}
	errc := make(chan error, 1)
	go func() { errc <- timeline.Build(ctx, src, log, opts, tl) }()

	// Poll only to measure how soon a player could show something.
	var first time.Duration
	for tl.Len() == 0 {
		if done, _ := tl.Done(); done {
			break
		}
		time.Sleep(time.Millisecond)
	}
	first = time.Since(start)
	if err := <-errc; err != nil {
		return err
	}

	cols := tl.Columns()
	quiet := 0
	for i := range tl.Len() {
		f := tl.At(i)
		if f.Commits == 0 {
			quiet++
			continue
		}
		fmt.Fprintf(w, "%4d  %s  %4d commits  %s\n", f.Index, f.Time.Format("2006-01-02"),
			f.Commits, topColumns(f, cols, 5))
	}
	last := tl.At(tl.Len() - 1)
	fmt.Fprintf(w, "\n%d frames (%d quiet), %d columns, tallest %d lines\n",
		tl.Len(), quiet, len(cols), last.Max)
	fmt.Fprintf(w, "final: %s\n", topColumns(last, cols, 10))
	fmt.Fprintf(w, "first frame after %s, total %s\n",
		first.Round(time.Millisecond), time.Since(start).Round(time.Millisecond))
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
