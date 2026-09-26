// Package timeline turns a stream of commits into fixed-interval frames of
// per-folder line counts, ready to be drawn.
package timeline

import (
	"strings"
	"sync"
	"time"
)

// RootKey is the column for files that sit directly in the repository root.
const RootKey = "(root)"

// KeyFor maps a file path to its column: the first depth directories of the
// path. A file shallower than depth uses the directories it has.
func KeyFor(path string, depth int) string {
	i := strings.LastIndexByte(path, '/')
	if i <= 0 {
		return RootKey
	}
	dir := path[:i]
	n := 0
	for i := range len(dir) {
		if dir[i] == '/' {
			n++
			if n == depth {
				return dir[:i]
			}
		}
	}
	return dir
}

// Caption describes the commit shown under a frame.
type Caption struct {
	Hash    string
	Author  string
	Subject string
	Time    time.Time // author date
}

// Frame is the state of the repository at the end of one time slice.
// Slices are indexed by column id (see Timeline.Columns) and hold only the
// columns known at that point; later columns are implicitly zero.
type Frame struct {
	Index   int
	Time    time.Time // end of the slice
	Totals  []int64   // lines per column, never negative
	Touched []int64   // unix time each column last changed, 0 if unknown
	Max     int64     // largest column total in this or any earlier frame
	Ceiling int64     // scale for drawing: follows Max up at once, eases down slowly
	Commits int       // commits that landed in this slice
	Caption *Caption  // last commit of the slice, nil for a quiet slice
}

// Total returns the line count of a column, 0 if it did not exist yet.
func (f Frame) Total(col int) int64 {
	if col < len(f.Totals) {
		return f.Totals[col]
	}
	return 0
}

// Timeline collects frames as they are built, so a reader (the player) can
// show early frames while later ones are still being computed. It is safe
// for one writer and any number of readers.
type Timeline struct {
	mu       sync.RWMutex
	columns  []string
	frames   []Frame
	expected int
	done     bool
	err      error
}

// Len returns the number of frames built so far.
func (t *Timeline) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.frames)
}

// At returns frame i; it panics if i >= Len().
func (t *Timeline) At(i int) Frame {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.frames[i]
}

// Columns returns the column names in order of first appearance. The
// returned slice must not be modified.
func (t *Timeline) Columns() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.columns
}

// Expected returns how many frames the finished timeline will have, or 0
// before the history's bounds are known.
func (t *Timeline) Expected() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.expected
}

// Done reports whether building has finished, and with which error.
func (t *Timeline) Done() (bool, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.done, t.err
}

func (t *Timeline) setExpected(n int) {
	t.mu.Lock()
	t.expected = n
	t.mu.Unlock()
}

// push publishes a frame. Old frames and column names are never mutated,
// which is what makes handing out slices to readers safe.
func (t *Timeline) push(f Frame, columns []string) {
	t.mu.Lock()
	t.frames = append(t.frames, f)
	t.columns = columns
	t.mu.Unlock()
}

func (t *Timeline) finish(err error) {
	t.mu.Lock()
	t.done, t.err = true, err
	t.mu.Unlock()
}
