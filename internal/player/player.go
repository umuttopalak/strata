// Package player animates a timeline in the terminal.
package player

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"

	"github.com/umuttopalak/strata/internal/render"
	"github.com/umuttopalak/strata/internal/timeline"
)

// DefaultInterval is the time between frames at speed 1.
const DefaultInterval = 75 * time.Millisecond

const (
	hideCursor  = "\x1b[?25l"
	showCursor  = "\x1b[?25h"
	clearScreen = "\x1b[2J"
	cursorHome  = "\x1b[H"
	clearLine   = "\x1b[K" // to end of line
)

// Options configures a replay.
type Options struct {
	Repo     string        // name shown in the header
	Interval time.Duration // between frames
}

// fallbackSize is used when the output is not a terminal.
const fallbackWidth, fallbackHeight = 100, 30

// Play animates tl on out. When out is not a terminal (a pipe or file) it
// prints only the final frame. Cancelling ctx (Ctrl+C) stops the replay,
// leaves the current frame on screen and restores the cursor.
func Play(ctx context.Context, out *os.File, tl *timeline.Timeline, opts Options) error {
	layout := render.NewLayout(tl)
	w := colorprofile.NewWriter(out, os.Environ())
	fd := int(out.Fd())

	if !term.IsTerminal(fd) {
		pic := render.Draw(tl, layout, len(tl.Frames)-1, opts.Repo, fallbackWidth, fallbackHeight)
		_, err := io.WriteString(w, strings.Join(pic.Styled(), "\n")+"\n"+finished(tl)+"\n")
		return err
	}

	enableVT(out)
	io.WriteString(out, hideCursor) // the first frame clears the screen
	// Deferred so the cursor comes back on every exit path, Ctrl+C included.
	defer io.WriteString(out, showCursor)

	interval := max(opts.Interval, time.Millisecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastW, lastH int
	for i := range tl.Frames {
		width, height, err := term.GetSize(fd)
		if err != nil {
			width, height = fallbackWidth, fallbackHeight
		}
		var b strings.Builder
		if width != lastW || height != lastH {
			b.WriteString(clearScreen) // first frame or resized: clear leftovers
			lastW, lastH = width, height
		}
		b.WriteString(cursorHome)
		pic := render.Draw(tl, layout, i, opts.Repo, width, height)
		lines := pic.Styled()
		for j, line := range lines {
			b.WriteString(line + clearLine)
			if j < len(lines)-1 {
				b.WriteString("\r\n")
			}
		}
		if _, err := io.WriteString(w, b.String()); err != nil {
			return err
		}

		if i == len(tl.Frames)-1 {
			break
		}
		select {
		case <-ctx.Done():
			io.WriteString(out, "\r\n")
			return ctx.Err()
		case <-ticker.C:
		}
	}
	_, err := io.WriteString(w, "\r\n"+finished(tl)+"\n")
	return err
}

func finished(tl *timeline.Timeline) string {
	return fmt.Sprintf("✓ belgesel bitti · %d commit", tl.Commits)
}
