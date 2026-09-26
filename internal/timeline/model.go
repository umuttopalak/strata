// Package timeline replays a repository's history into frames of
// per-folder line counts, one frame per commit (or per small group of
// commits in long histories).
package timeline

import (
	"strings"
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
	Added   int
	Deleted int
}

// Frame is the repository's state after a number of commits.
// Slices are indexed by column id (see Timeline.Columns) and hold only the
// columns known at that point; later columns are implicitly zero.
type Frame struct {
	Commit  int      // commits applied so far; 0 is the state before the first
	Totals  []int64  // lines per column, never negative
	Touched []int    // commit number that last changed each column, 0 if none
	Caption *Caption // last commit applied in this frame, nil for frame 0
}

// Total returns the line count of a column, 0 if it did not exist yet.
func (f Frame) Total(col int) int64 {
	if col < len(f.Totals) {
		return f.Totals[col]
	}
	return 0
}

// LastTouched returns the commit number that last changed a column.
func (f Frame) LastTouched(col int) int {
	if col < len(f.Touched) {
		return f.Touched[col]
	}
	return 0
}

// Timeline is a fully scanned history.
type Timeline struct {
	Columns []string // in order of first appearance
	Frames  []Frame  // Frames[0] is the state before the first commit
	Commits int      // commits in the replay
	Peak    []int64  // largest total each column ever reached
	Depth   int      // folder depth the columns were grouped at
}
