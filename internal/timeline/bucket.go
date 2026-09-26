package timeline

import (
	"math"
	"time"

	"github.com/umuttopalak/summit/internal/gitlog"
)

// DefaultFrames is how many time slices a whole history is divided into.
const DefaultFrames = 600

// ceilingDecay is how much of the drawing scale is kept per frame after the
// tallest column shrinks: about 70 frames to halve. Without it one huge
// folder that is later deleted (a vendored node_modules) would keep every
// other peak small for the rest of the replay.
const ceilingDecay = 0.99

// Options controls how commits are grouped.
type Options struct {
	Depth  int // folder depth that defines a column; values < 1 mean 1
	Frames int // number of time slices; values < 1 mean DefaultFrames
}

// Builder groups commits into equal time slices between the history's
// bounds. Slices without commits still produce frames, so quiet periods
// stay visible. Commits must be added in history order.
type Builder struct {
	tl    *Timeline
	depth int
	start time.Time
	end   time.Time
	step  time.Duration
	n     int

	ids     map[string]int
	names   []string
	totals  []int64 // may dip below zero transiently; clamped when published
	touched []int64

	cur     int // index of the slice being filled
	commits int
	caption *Caption
	max     int64
	ceiling float64
}

// NewBuilder prepares a builder that publishes frames to tl.
func NewBuilder(b gitlog.Bounds, opts Options, tl *Timeline) *Builder {
	if opts.Depth < 1 {
		opts.Depth = 1
	}
	n := opts.Frames
	if n < 1 {
		n = DefaultFrames
	}
	span := b.Last.Sub(b.First)
	step := span / time.Duration(n)
	if step <= 0 {
		n, step = 1, max(span, time.Second)
	}
	tl.setExpected(n)
	return &Builder{
		tl: tl, depth: opts.Depth, start: b.First, end: b.Last, step: step, n: n,
		ids: map[string]int{},
	}
}

// Seed adds files that existed before the first slice (see gitlog.Baseline).
// Their last-touched time is unknown and stays 0.
func (b *Builder) Seed(files []gitlog.FileChange) {
	for _, f := range files {
		b.totals[b.column(f.Path)] += int64(f.Added - f.Deleted)
	}
}

// Add applies one commit, first publishing any slices that ended before it.
func (b *Builder) Add(c gitlog.Commit) {
	b.advanceTo(b.slice(c.Time))

	at := c.Time.Unix()
	for _, f := range c.Files {
		id := b.column(f.Path)
		b.totals[id] += int64(f.Added - f.Deleted)
		b.touched[id] = at
	}
	b.commits++
	b.caption = &Caption{Hash: c.Hash, Author: c.Author, Subject: c.Subject, Time: c.AuthorTime}
}

// Finish publishes the remaining slices; the last one holds the final state.
func (b *Builder) Finish() {
	b.advanceTo(b.n - 1)
	b.publish()
}

// slice returns the slice a commit time falls in. Commit dates are not
// strictly ordered (clock skew, rebases), so time never moves backwards:
// an early-dated commit joins the slice currently being filled.
func (b *Builder) slice(t time.Time) int {
	i := int(t.Sub(b.start) / b.step)
	return min(max(i, b.cur), b.n-1)
}

func (b *Builder) advanceTo(i int) {
	for b.cur < i {
		b.publish()
		b.cur++
		b.commits, b.caption = 0, nil
	}
}

func (b *Builder) publish() {
	totals := make([]int64, len(b.totals))
	var tallest int64
	for i, v := range b.totals {
		totals[i] = max(v, 0)
		tallest = max(tallest, totals[i])
	}
	b.max = max(b.max, tallest)
	b.ceiling = max(float64(tallest), b.ceiling*ceilingDecay)
	end := b.start.Add(b.step * time.Duration(b.cur+1))
	if b.cur == b.n-1 {
		end = b.end // the step is rounded down; land exactly on the last commit
	}
	b.tl.push(Frame{
		Index:   b.cur,
		Time:    end,
		Totals:  totals,
		Touched: append([]int64(nil), b.touched...),
		Max:     b.max,
		Ceiling: int64(math.Ceil(b.ceiling)),
		Commits: b.commits,
		Caption: b.caption,
	}, b.names[:len(b.names):len(b.names)])
}

func (b *Builder) column(path string) int {
	key := KeyFor(path, b.depth)
	id, ok := b.ids[key]
	if !ok {
		id = len(b.names)
		b.ids[key] = id
		b.names = append(b.names, key)
		b.totals = append(b.totals, 0)
		b.touched = append(b.touched, 0)
	}
	return id
}
