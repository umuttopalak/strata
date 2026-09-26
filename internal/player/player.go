// Package player animates a timeline in the terminal.
package player

import (
	"context"
	"errors"
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

// DefaultInterval is the time one frame takes at speed 1.
const DefaultInterval = 75 * time.Millisecond

// drawEvery is how often the screen is redrawn. It is shorter than a frame
// so mountains move smoothly between frames instead of jumping.
const drawEvery = 25 * time.Millisecond

// seekFrames is how far ← and → jump.
const seekFrames = 10

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
	Interval time.Duration // one frame at speed 1; 0 means DefaultInterval
	Speed    float64       // starting speed; 0 means 1
}

// fallbackSize is used when the output is not a terminal.
const fallbackWidth, fallbackHeight = 100, 30

// Play animates tl on out. When out is not a terminal (a pipe or file) it
// prints only the final frame. When stdin is a terminal it also listens
// for keys: space pauses, ←/→ seek, +/- change speed, q quits.
// Ctrl+C or cancelling ctx stops the replay, leaves the current frame on
// screen, restores the terminal and returns context.Canceled.
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
	io.WriteString(out, hideCursor) // the first draw clears the screen
	// Deferred so the cursor comes back on every exit path.
	defer io.WriteString(out, showCursor)

	keys, restore := listen(os.Stdin)
	defer restore()

	interval := opts.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	s := state{last: float64(len(tl.Frames) - 1), speed: opts.Speed}
	if s.speed <= 0 {
		s.speed = 1
	}
	ticker := time.NewTicker(drawEvery)
	defer ticker.Stop()

	var lastW, lastH int
	drawn := -1.0
	prev := time.Now()
	for {
		width, height, err := term.GetSize(fd)
		if err != nil {
			width, height = fallbackWidth, fallbackHeight
		}
		resized := width != lastW || height != lastH
		if resized || s.pos != drawn || s.dirty {
			if err := draw(w, tl, layout, s, opts.Repo, width, height, resized); err != nil {
				return err
			}
			lastW, lastH, drawn, s.dirty = width, height, s.pos, false
		}
		if s.pos >= s.last && !s.paused {
			break
		}

		select {
		case <-ctx.Done():
			restore()
			io.WriteString(out, "\r\n")
			return ctx.Err()
		case k := <-keys:
			if err := s.handle(k); err != nil {
				restore()
				io.WriteString(out, "\r\n")
				if errors.Is(err, errQuit) {
					return nil // q leaves the current frame on screen
				}
				return err
			}
		case now := <-ticker.C:
			if !s.paused {
				s.pos = min(s.pos+float64(now.Sub(prev))/float64(interval)*s.speed, s.last)
			}
			prev = now
		}
	}
	restore()
	_, err := io.WriteString(w, "\r\n"+finished(tl)+"\n")
	return err
}

// state is the playback position and controls.
type state struct {
	pos    float64 // frame position, fractional between frames
	last   float64
	speed  float64
	paused bool
	dirty  bool // status changed without the position moving
}

// handle applies a key. It returns an error when playback should stop.
func (s *state) handle(k key) error {
	switch k {
	case keyPause:
		s.paused = !s.paused
		s.dirty = true
	case keyBack:
		s.pos = max(s.pos-seekFrames, 0)
	case keyForward:
		s.pos = min(s.pos+seekFrames, s.last)
	case keyFaster:
		s.speed = nextSpeed(s.speed, 1)
		s.dirty = true
	case keySlower:
		s.speed = nextSpeed(s.speed, -1)
		s.dirty = true
	case keyQuit:
		return errQuit
	case keyInterrupt:
		return context.Canceled
	}
	return nil
}

// errQuit ends playback early at the user's request; it is not a failure.
var errQuit = errors.New("quit")

func (s state) status() string {
	var parts []string
	if s.paused {
		parts = append(parts, "paused")
	}
	if s.speed != 1 {
		parts = append(parts, fmt.Sprintf("%g×", s.speed))
	}
	return strings.Join(parts, " ")
}

func draw(w io.Writer, tl *timeline.Timeline, l render.Layout, s state, repo string, width, height int, clear bool) error {
	var b strings.Builder
	if clear {
		b.WriteString(clearScreen) // first draw or resized: wipe leftovers
	}
	b.WriteString(cursorHome)
	pic := render.DrawAt(tl, l, s.pos, repo, width, height)
	pic.Status = s.status()
	lines := pic.Styled()
	for j, line := range lines {
		b.WriteString(line + clearLine)
		if j < len(lines)-1 {
			b.WriteString("\r\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func finished(tl *timeline.Timeline) string {
	return fmt.Sprintf("✓ belgesel bitti · %d commit", tl.Commits)
}

// listen puts stdin in raw mode and streams key presses. If stdin is not a
// terminal it returns a channel that never delivers. restore is safe to
// call more than once.
func listen(in *os.File) (<-chan key, func()) {
	keys := make(chan key, 16)
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		return keys, func() {}
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return keys, func() {}
	}
	restored := false
	restore := func() {
		if !restored {
			restored = true
			_ = term.Restore(fd, old)
		}
	}
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := in.Read(buf)
			if err != nil {
				return
			}
			for _, k := range parseKeys(buf[:n]) {
				keys <- k
			}
		}
	}()
	return keys, restore
}
