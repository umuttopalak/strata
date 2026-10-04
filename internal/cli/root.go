// Package cli wires command-line flags to the strata pipeline.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/spf13/cobra"

	"github.com/umuttopalak/strata/internal/export"
	"github.com/umuttopalak/strata/internal/gitlog"
	"github.com/umuttopalak/strata/internal/player"
	"github.com/umuttopalak/strata/internal/timeline"
)

// Version is overridden at build time via -ldflags "-X ...cli.Version=v1.2.3".
var Version = "dev"

// version falls back to the module version Go records for
// `go install …@v1.2.3` builds, which have no ldflags.
func version() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}

// Config is the validated set of options for one run.
type Config struct {
	Path    string
	Speed   float64
	Depth   int
	Since   time.Time // zero means the whole history
	Cols    int       // --size, 0 when not given
	Rows    int
	Labels  bool
	Exclude []string
}

type flags struct {
	speed   float64
	depth   int
	since   string
	size    string
	svg     string
	json    string
	csv     string
	labels  bool
	exclude []string
	dump    bool
	dumpF   bool
	frame   int
}

func (f flags) config(path string) (Config, error) {
	cfg := Config{Path: path, Speed: f.speed, Depth: f.depth, Labels: f.labels, Exclude: append([]string(nil), f.exclude...)}
	if f.speed <= 0 {
		return cfg, fmt.Errorf("--speed must be greater than 0 (got %g)", f.speed)
	}
	if f.depth < 0 {
		return cfg, fmt.Errorf("--depth must be 0 (automatic) or more (got %d)", f.depth)
	}
	if f.size != "" {
		if _, err := fmt.Sscanf(f.size, "%dx%d", &cfg.Cols, &cfg.Rows); err != nil ||
			cfg.Cols < 20 || cfg.Rows < 8 || fmt.Sprintf("%dx%d", cfg.Cols, cfg.Rows) != f.size {
			return cfg, fmt.Errorf("--size must look like 100x30, at least 20x8 (got %q)", f.size)
		}
	}
	if f.since != "" {
		t, err := time.ParseInLocation("2006-01-02", f.since, time.Local)
		if err != nil {
			return cfg, fmt.Errorf("--since must be a date like 2020-01-31 (got %q)", f.since)
		}
		cfg.Since = t
	}
	return cfg, nil
}

func newRootCmd() *cobra.Command {
	var f flags
	cmd := &cobra.Command{
		Use:   "strata [path or URL]",
		Short: "Watch a Git repository's history grow into a mountain range",
		Long: `strata replays a Git repository's history in the terminal as a mountain
range: each peak is a folder and its height is the folder's size in lines
of code. By default strata picks the folder depth that gives at least four
peaks; a repository with all its files in the root gets a peak per file.`,
		Example: `  strata                      # the repository in the current directory
  strata ./path/to/repo       # a specific repository
  strata github.com/owner/repo  # any public repository, cloned to a temporary folder
  strata --speed 2 --depth 2  # faster playback, folders two levels deep
  strata --since 2020-01-01   # only history from 2020 onwards
  strata --labels             # name each mountain under the ground line
  strata --svg strata.svg     # animated SVG for a README (loops, no scripts)

Keys while playing: space pause · ←/→ seek · +/- speed · q quit`,
		Args:          cobra.MaximumNArgs(1),
		Version:       version(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			cfg, err := f.config(path)
			if err != nil {
				return err
			}
			name := ""
			if gitlog.IsRemote(cfg.Path) {
				dir, cleanup, err := cloneRemote(cmd.Context(), cmd.ErrOrStderr(), cfg.Path)
				defer cleanup()
				if err != nil {
					return friendly(err)
				}
				name = gitlog.RepoName(gitlog.RemoteURL(cfg.Path))
				cfg.Path = dir
			}
			repo, err := gitlog.OpenRepo(cmd.Context(), cfg.Path)
			if err != nil {
				return friendly(err)
			}
			if name == "" {
				name = filepath.Base(repo.Root)
			}
			if repo.Shallow(cmd.Context()) {
				fmt.Fprintln(cmd.ErrOrStderr(), "strata: warning: this is a shallow clone, so only part of the history is available\n"+
					"  hint: run `git fetch --unshallow`, or set `fetch-depth: 0` on actions/checkout")
			}
			opts := gitlog.LogOptions{Since: cfg.Since}
			bounds, err := repo.Bounds(cmd.Context(), opts)
			if err != nil {
				return friendly(err)
			}
			if f.dump {
				return dump(cmd.Context(), cmd.OutOrStdout(), repo, opts)
			}

			tl, err := scan(cmd.Context(), repo, opts, timeline.Options{Depth: cfg.Depth, Exclude: cfg.Exclude})
			if err != nil {
				return friendly(err)
			}
			if f.json != "" {
				if err := export.WriteJSON(f.json, name, bounds, tl); err != nil {
					return err
				}
			}
			if f.csv != "" {
				if err := export.WriteCSV(f.csv, name, bounds, tl); err != nil {
					return err
				}
			}
			if f.svg != "" {
				if err := writeSVG(cmd.ErrOrStderr(), f.svg, tl, name, cfg); err != nil {
					return err
				}
			}
			if f.json != "" || f.csv != "" || f.svg != "" {
				return nil
			}
			switch {
			case cmd.Flags().Changed("frame"):
				return printFrame(cmd.OutOrStdout(), tl, name, f.frame, cfg)
			case f.dumpF:
				return dumpFrames(cmd.OutOrStdout(), tl)
			}
			return player.Play(cmd.Context(), os.Stdout, tl, player.Options{
				Repo:   name,
				Speed:  cfg.Speed,
				Labels: cfg.Labels,
			})
		},
	}
	cmd.Flags().Float64Var(&f.speed, "speed", 1, "playback speed multiplier")
	cmd.Flags().IntVar(&f.depth, "depth", 0, "folder depth that defines a peak (0 = pick automatically)")
	cmd.Flags().StringVar(&f.since, "since", "", "start from this date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&f.svg, "svg", "", "write an animated SVG to this file instead of playing")
	cmd.Flags().StringVar(&f.json, "json", "", "write a versioned JSON export to this file instead of playing")
	cmd.Flags().StringVar(&f.csv, "csv", "", "write a CSV export to this file instead of playing")
	cmd.Flags().BoolVar(&f.labels, "labels", false, "write folder names under the mountains")
	cmd.Flags().StringArrayVar(&f.exclude, "exclude", nil, "exclude paths matching a glob or preset (vendor, generated, test); repeatable")
	cmd.Flags().StringVar(&f.size, "size", "", "size in terminal cells for --svg and --frame, e.g. 100x30")
	cmd.Flags().BoolVar(&f.dump, "dump", false, "print the parsed history instead of playing it")
	cmd.Flags().BoolVar(&f.dumpF, "dump-frames", false, "print the timeline frames instead of playing them")
	cmd.Flags().IntVar(&f.frame, "frame", -1, "print one frame (negative counts from the end) and exit")
	_ = cmd.Flags().MarkHidden("dump")
	_ = cmd.Flags().MarkHidden("frame")
	_ = cmd.Flags().MarkHidden("dump-frames")
	return cmd
}

// friendly appends a hint on how to fix the most common problems.
func friendly(err error) error {
	var hint string
	switch {
	case errors.Is(err, gitlog.ErrGitNotFound):
		hint = "install Git from https://git-scm.com/downloads and make sure it is on your PATH"
	case errors.Is(err, gitlog.ErrNotRepo):
		hint = "run strata inside a Git repository or pass the path to one"
	case errors.Is(err, gitlog.ErrEmptyRepo):
		hint = "make at least one commit, then try again"
	case errors.Is(err, gitlog.ErrCloneFailed):
		hint = "check the URL; for a private repository, clone it yourself and pass the local path"
	case errors.Is(err, gitlog.ErrNoCommits):
		hint = "pick an earlier --since date"
	default:
		return err
	}
	return fmt.Errorf("%w\n  hint: %s", err, hint)
}

// Execute runs the root command.
func Execute(ctx context.Context) error {
	return newRootCmd().ExecuteContext(ctx)
}
