package timeline

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/umuttopalak/strata/internal/gitlog"
)

// DefaultMaxFrames caps the frame count: longer histories advance several
// commits per frame.
const DefaultMaxFrames = 300

// Options controls how commits become frames.
type Options struct {
	Depth     int      // folder depth that defines a column; 0 picks one (see Build)
	MaxFrames int      // values < 1 mean DefaultMaxFrames
	Exclude   []string // path globs or presets to leave out of the replay

	// Progress, if set, is called after each commit is read with the
	// number read so far.
	Progress func(commits int)
}

// record is what the replay keeps of each commit: its net line change per
// column, plus the caption. File lists are dropped as soon as they are read.
type record struct {
	deltas  []delta
	caption Caption
	at      time.Time // committer date, for quiet periods
	tags    []string
}

type delta struct {
	col int
	n   int64
}

// Build scans the whole history first, because both the frame grouping and
// the drawing scale depend on its full length and peak sizes.
//
// With Depth 0 it picks the shallowest level that yields at least MinPeaks
// folders, so a project with a single src/ folder, or one whose files all
// sit in the root, still becomes a range rather than one lone mountain.
func Build(ctx context.Context, src gitlog.Source, log gitlog.LogOptions, opts Options) (*Timeline, error) {
	if opts.MaxFrames < 1 {
		opts.MaxFrames = DefaultMaxFrames
	}

	base, err := src.Baseline(ctx, log)
	if err != nil {
		return nil, err
	}

	c := columns{depth: opts.Depth, ids: map[string]int{}}
	if opts.Depth < 1 {
		// Collect at the finest level once; pickLevel merges afterwards.
		c.depth, c.splitRoot = maxAutoDepth, true
	}
	var seed []delta
	for _, f := range base {
		if excluded(f.Path, opts.Exclude) {
			continue
		}
		seed = append(seed, delta{c.id(f.Path), int64(f.Added - f.Deleted)})
	}

	var records []record
	err = src.Walk(ctx, log, func(gc gitlog.Commit) error {
		cm := record{at: gc.Time, tags: gc.Tags, caption: Caption{
			Hash: gc.Hash, Author: gc.Author, Subject: gc.Subject, Time: gc.AuthorTime,
		}}
		for _, f := range gc.Files {
			if excluded(f.Path, opts.Exclude) {
				continue
			}
			cm.caption.Files++
			cm.caption.Added += f.Added
			cm.caption.Deleted += f.Deleted
			cm.deltas = addDelta(cm.deltas, c.id(f.Path), int64(f.Added-f.Deleted))
		}
		records = append(records, cm)
		if opts.Progress != nil {
			opts.Progress(len(records))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, gitlog.ErrNoCommits
	}

	level, names, remap := opts.Depth, c.names, identity(len(c.names))
	if opts.Depth < 1 {
		level = c.pickLevel(netTotals(len(c.names), seed, records))
		names, remap = c.coarsen(level)
	}
	tl := replay(names, remap, seed, records, opts.MaxFrames)
	tl.Depth = level
	var seedLines int64
	for _, d := range seed {
		seedLines += d.n
	}
	tl.Events = detectEvents(records, seedLines, len(seed) > 0, framesPer(len(records), opts.MaxFrames))
	return tl, nil
}

// excluded reports whether a path matches an exclusion glob or built-in
// preset. Patterns match the complete slash-separated path; a pattern with
// no slash also matches any directory or file component of that name.
func excluded(name string, patterns []string) bool {
	name = strings.Trim(name, "/")
	for _, raw := range patterns {
		p := strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
		if p == "" {
			continue
		}
		if preset, ok := excludePresets[p]; ok {
			for _, presetPattern := range preset {
				if matchExcludePattern(name, presetPattern) {
					return true
				}
			}
			continue
		}
		if matchExcludePattern(name, p) {
			return true
		}
	}
	return false
}

func matchExcludePattern(name, pattern string) bool {
	if strings.Contains(pattern, "/") {
		return globMatch(pattern, name)
	}
	for _, part := range strings.Split(name, "/") {
		if globMatch(pattern, part) {
			return true
		}
	}
	return false
}

// Presets intentionally use directory components, so they work at any
// nesting level without requiring a non-standard ** glob implementation.
var excludePresets = map[string][]string{
	"vendor":    {"vendor"},
	"generated": {"generated", "gen"},
	"test":      {"test/**", "tests/**", "*_test.go", "*.test.*"},
}

func globMatch(pattern, name string) bool {
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		return name == base || strings.HasPrefix(name, base+"/")
	}
	if strings.HasPrefix(pattern, "**/") {
		pattern = strings.TrimPrefix(pattern, "**/")
		if globMatch(pattern, name) {
			return true
		}
		for i := strings.IndexByte(name, '/'); i >= 0; {
			if globMatch(pattern, name[i+1:]) {
				return true
			}
			rest := name[i+1:]
			n := strings.IndexByte(rest, '/')
			if n < 0 {
				break
			}
			i += n + 1
		}
		return false
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// framesPer is how many commits each frame advances.
func framesPer(commits, maxFrames int) int {
	return max((commits+maxFrames-1)/maxFrames, 1)
}

// addDelta merges a file's change into its column's entry; a commit
// touches few columns, so a linear scan beats a map.
func addDelta(ds []delta, col int, n int64) []delta {
	for i := range ds {
		if ds[i].col == col {
			ds[i].n += n
			return ds
		}
	}
	return append(ds, delta{col, n})
}

// replay applies the records in order and snapshots every per-th one, so
// the result has at most maxFrames frames after the initial one.
// Deltas refer to collected columns; remap turns them into output columns.
func replay(names []string, remap []int, seed []delta, records []record, maxFrames int) *Timeline {
	n := len(records)
	per := framesPer(n, maxFrames)

	totals := make([]int64, len(names))
	touched := make([]int, len(names))
	tl := &Timeline{Columns: names, Commits: n, Peak: make([]int64, len(names))}
	for _, d := range seed {
		totals[remap[d.col]] += d.n
	}

	snapshot := func(i int, caption *Caption) {
		f := Frame{Commit: i, Totals: make([]int64, len(totals)),
			Touched: append([]int(nil), touched...), Caption: caption}
		for c, v := range totals {
			f.Totals[c] = max(v, 0)
		}
		tl.Frames = append(tl.Frames, f)
	}
	snapshot(0, nil)
	for c, v := range totals {
		tl.Peak[c] = max(v, 0)
	}

	for i, cm := range records {
		for _, d := range cm.deltas {
			col := remap[d.col]
			totals[col] += d.n
			touched[col] = i + 1
			tl.Peak[col] = max(tl.Peak[col], totals[col])
		}
		if (i+1)%per == 0 || i == n-1 {
			snapshot(i+1, &cm.caption)
		}
	}
	return tl
}

// columns assigns ids to folder keys in order of first appearance.
type columns struct {
	depth     int  // directory levels kept in a key
	splitRoot bool // give each root-level file its own key instead of RootKey
	ids       map[string]int
	names     []string
	rootFile  []bool // the key is a root-level file, not a folder
}

func (c *columns) id(path string) int {
	key, file := RootKey, false
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		key = KeyFor(path, c.depth)
	} else if c.splitRoot {
		key, file = path, true
	}
	id, ok := c.ids[key]
	if !ok {
		id = len(c.names)
		c.ids[key] = id
		c.names = append(c.names, key)
		c.rootFile = append(c.rootFile, file)
	}
	return id
}
