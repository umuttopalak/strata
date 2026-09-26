package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/umuttopalak/strata/internal/gitlog"
	"github.com/umuttopalak/strata/internal/render"
	"github.com/umuttopalak/strata/internal/timeline"
)

// scan reads the whole history before playback (the scale depends on it),
// showing a commit counter on stderr when that is a terminal.
func scan(ctx context.Context, src gitlog.Source, log gitlog.LogOptions, opts timeline.Options) (*timeline.Timeline, error) {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		var last time.Time
		opts.Progress = func(n int) {
			if now := time.Now(); now.Sub(last) > 100*time.Millisecond {
				last = now
				fmt.Fprintf(os.Stderr, "\r\x1b[Kreading history… %d commits", n)
			}
		}
		defer fmt.Fprint(os.Stderr, "\r\x1b[K")
	}
	return timeline.Build(ctx, src, log, opts)
}

// printFrame prints one frame sized to the terminal: a quick way to look
// at the renderer without animating.
func printFrame(out io.Writer, tl *timeline.Timeline, repo string, index int) error {
	if index < 0 {
		index += len(tl.Frames)
	}
	if index < 0 || index >= len(tl.Frames) {
		return fmt.Errorf("--frame must be between %d and %d", -len(tl.Frames), len(tl.Frames)-1)
	}
	width, height := 100, 30
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width, height = w, h
	}
	pic := render.Draw(tl, render.NewLayout(tl), index, repo, width, height)
	for _, line := range pic.Styled() {
		if _, err := lipgloss.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}
