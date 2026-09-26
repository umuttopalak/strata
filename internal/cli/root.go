// Package cli wires command-line flags to the summit pipeline.
package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/umuttopalak/summit/internal/gitlog"
	"github.com/umuttopalak/summit/internal/timeline"
)

// Version is overridden at build time via -ldflags "-X ...cli.Version=v1.2.3".
var Version = "dev"

// Config is the validated set of options for one run.
type Config struct {
	Path  string
	Speed float64
	Depth int
	Since time.Time // zero means the whole history
}

type flags struct {
	speed float64
	depth int
	since string
	dump  bool
	dumpF bool
	frame int
}

func (f flags) config(path string) (Config, error) {
	cfg := Config{Path: path, Speed: f.speed, Depth: f.depth}
	if f.speed <= 0 {
		return cfg, fmt.Errorf("--speed must be greater than 0 (got %g)", f.speed)
	}
	if f.depth < 1 {
		return cfg, fmt.Errorf("--depth must be at least 1 (got %d)", f.depth)
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
		Use:   "summit [path]",
		Short: "Watch a Git repository's history grow into a mountain range",
		Long: `summit replays a Git repository's history in the terminal as a mountain
range: each peak is a top-level folder and its height is the folder's size
in lines of code.`,
		Example: `  summit                      # the repository in the current directory
  summit ./path/to/repo       # a specific repository
  summit --speed 2 --depth 2  # faster playback, folders two levels deep
  summit --since 2020-01-01   # only history from 2020 onwards`,
		Args:          cobra.MaximumNArgs(1),
		Version:       Version,
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
			repo, err := gitlog.OpenRepo(cmd.Context(), cfg.Path)
			if err != nil {
				return friendly(err)
			}
			opts := gitlog.LogOptions{Since: cfg.Since}
			if _, err := repo.Bounds(cmd.Context(), opts); err != nil {
				return friendly(err)
			}
			if f.dump {
				return dump(cmd.Context(), cmd.OutOrStdout(), repo, opts)
			}
			if cmd.Flags().Changed("frame") {
				return printFrame(cmd.Context(), repo, opts,
					timeline.Options{Depth: cfg.Depth}, f.frame)
			}
			if f.dumpF {
				return dumpFrames(cmd.Context(), cmd.OutOrStdout(), repo, opts,
					timeline.Options{Depth: cfg.Depth})
			}
			// Placeholder until the player lands.
			fmt.Fprintf(cmd.OutOrStdout(), "repository: %s\nspeed: %gx  depth: %d\n",
				repo.Root, cfg.Speed, cfg.Depth)
			return nil
		},
	}
	cmd.Flags().Float64Var(&f.speed, "speed", 1, "playback speed multiplier")
	cmd.Flags().IntVar(&f.depth, "depth", 1, "folder depth that defines a peak")
	cmd.Flags().StringVar(&f.since, "since", "", "start from this date (YYYY-MM-DD)")
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
		hint = "run summit inside a Git repository or pass the path to one"
	case errors.Is(err, gitlog.ErrEmptyRepo):
		hint = "make at least one commit, then try again"
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
