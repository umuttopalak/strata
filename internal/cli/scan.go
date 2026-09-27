package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/umuttopalak/strata/internal/gitlog"
	"github.com/umuttopalak/strata/internal/player"
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
func printFrame(out io.Writer, tl *timeline.Timeline, repo string, index int, cfg Config) error {
	if index < 0 {
		index += len(tl.Frames)
	}
	if index < 0 || index >= len(tl.Frames) {
		return fmt.Errorf("--frame must be between %d and %d", -len(tl.Frames), len(tl.Frames)-1)
	}
	width, height := 100, 30
	if cfg.Cols > 0 {
		width, height = cfg.Cols, cfg.Rows
	} else if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width, height = w, h
	}
	layout := render.NewLayout(tl)
	layout.Labels = cfg.Labels
	pic := render.Draw(tl, layout, index, repo, width, height)
	for _, line := range pic.Styled() {
		if _, err := lipgloss.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

// writeSVG exports the replay as an animated SVG and reports its size.
func writeSVG(log io.Writer, path string, tl *timeline.Timeline, repo string, cfg Config) error {
	o := render.DefaultSVGOptions
	o.Repo = repo
	o.FrameDuration = time.Duration(float64(player.DefaultInterval) / cfg.Speed)
	if cfg.Cols > 0 {
		o.Cols, o.Height = cfg.Cols, cfg.Rows
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	layout := render.NewLayout(tl)
	layout.Labels = cfg.Labels
	if err := render.WriteSVG(f, tl, layout, o); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	play := o.PlayTime(len(tl.Frames))
	fmt.Fprintf(log, "wrote %s · %d KB · %d commits in %s, then holds %s\n",
		path, (info.Size()+1023)/1024, tl.Commits, play.Round(100*time.Millisecond), o.Hold)
	return nil
}

// cloneRemote clones a remote repository into a temporary folder for the
// duration of the run. cleanup removes it and is always safe to call.
func cloneRemote(ctx context.Context, log io.Writer, arg string) (dir string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "strata-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { _ = gitlog.RemoveClone(tmp) }

	url := gitlog.RemoteURL(arg)
	fmt.Fprintf(log, "cloning %s…\n", url)
	var progress io.Writer
	if term.IsTerminal(int(os.Stderr.Fd())) {
		progress = os.Stderr
	}
	dir = filepath.Join(tmp, "repo.git")
	return dir, cleanup, gitlog.Clone(ctx, url, dir, progress)
}
