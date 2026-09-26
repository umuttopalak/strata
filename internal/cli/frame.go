package cli

import (
	"context"
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/umuttopalak/summit/internal/gitlog"
	"github.com/umuttopalak/summit/internal/render"
	"github.com/umuttopalak/summit/internal/timeline"
)

// printFrame builds the whole timeline and prints a single frame sized to
// the terminal: a quick way to look at the renderer without the player.
func printFrame(ctx context.Context, src gitlog.Source, log gitlog.LogOptions, opts timeline.Options, index int) error {
	tl := &timeline.Timeline{}
	if err := timeline.Build(ctx, src, log, opts, tl); err != nil {
		return err
	}
	if index < 0 {
		index += tl.Len()
	}
	if index < 0 || index >= tl.Len() {
		return fmt.Errorf("--frame must be between %d and %d", -tl.Len(), tl.Len()-1)
	}

	width, height := 100, 30
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width, height = w, h
	}
	f := tl.At(index)
	pic := render.Draw(f, tl.Columns(), width, max(height-4, 4))
	for _, line := range pic.Styled() {
		lipgloss.Println(line)
	}
	lipgloss.Printf("frame %d/%d · %s\n", index, tl.Len()-1, f.Time.Format("2006-01-02"))
	return nil
}
