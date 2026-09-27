// Package render draws timeline frames as a mountain range: a height
// profile first, then terminal cells.
package render

import (
	"cmp"
	"slices"

	"github.com/umuttopalak/strata/internal/timeline"
)

// MaxPeaks is the most mountains drawn; beyond it the smallest folders are
// merged into one.
const MaxPeaks = 12

// OtherName names the merged mountain.
const OtherName = "other"

// Slot is one mountain: a folder, or several merged into OtherName.
type Slot struct {
	Name string
	Cols []int // timeline column ids
}

// Layout fixes where each mountain stands and the height scale for the
// whole replay, so mountains neither move nor rescale while they grow.
type Layout struct {
	Slots  []Slot // left to right
	Max    int64  // largest slot total in any frame
	Labels bool   // draw folder names under the ground line
}

// NewLayout ranks folders by their size at the end of the history (then by
// the largest size they ever reached), keeps the top MaxPeaks-1 (merging
// the rest when there are more than MaxPeaks) and arranges them with the
// largest in the middle and the others alternating right and left. The
// replay thus grows into a range that rises towards its centre, while
// folders that were deleted along the way rise and erode near the edges.
func NewLayout(tl *timeline.Timeline) Layout {
	final := tl.Frames[len(tl.Frames)-1]
	var ranked []int
	for col, peak := range tl.Peak {
		if peak > 0 {
			ranked = append(ranked, col)
		}
	}
	slices.SortStableFunc(ranked, func(a, b int) int {
		return cmp.Or(cmp.Compare(final.Total(b), final.Total(a)), cmp.Compare(tl.Peak[b], tl.Peak[a]))
	})

	var slots []Slot
	if len(ranked) > MaxPeaks {
		for _, col := range ranked[:MaxPeaks-1] {
			slots = append(slots, Slot{Name: tl.Columns[col], Cols: []int{col}})
		}
		slots = append(slots, Slot{Name: OtherName, Cols: slices.Clone(ranked[MaxPeaks-1:])})
	} else {
		for _, col := range ranked {
			slots = append(slots, Slot{Name: tl.Columns[col], Cols: []int{col}})
		}
	}

	l := Layout{Slots: centreOut(slots)}
	for _, f := range tl.Frames {
		for _, v := range l.Values(f) {
			l.Max = max(l.Max, v)
		}
	}
	return l
}

// centreOut places ranked[0] in the middle, ranked[1] to its right,
// ranked[2] to its left, and so on outwards.
func centreOut(ranked []Slot) []Slot {
	var left, right []Slot
	for i, s := range ranked {
		if i%2 == 1 {
			right = append(right, s)
		} else {
			left = append(left, s)
		}
	}
	slices.Reverse(left)
	return append(left, right...)
}

// Values returns each slot's line count in a frame.
func (l Layout) Values(f timeline.Frame) []int64 {
	out := make([]int64, len(l.Slots))
	for i, s := range l.Slots {
		for _, col := range s.Cols {
			out[i] += f.Total(col)
		}
	}
	return out
}

// Touched returns, per slot, the commit number of its most recent change.
func (l Layout) Touched(f timeline.Frame) []int {
	out := make([]int, len(l.Slots))
	for i, s := range l.Slots {
		for _, col := range s.Cols {
			out[i] = max(out[i], f.LastTouched(col))
		}
	}
	return out
}
