// Package render turns timeline frames into a mountain range: first a
// resolution-independent height profile, then terminal cells (and later
// SVG paths) drawn from it.
package render

import (
	"cmp"
	"slices"

	"github.com/umuttopalak/summit/internal/timeline"
)

// OtherName labels the peak that merges columns which do not fit.
const OtherName = "…other"

// OtherCol is the Peak.Col of the merged peak.
const OtherCol = -1

// Peak is one mountain in the range.
type Peak struct {
	Name  string
	Value int64
	Col   int // timeline column id, stable across frames; OtherCol if merged
}

// Select picks the peaks to draw from a frame: every non-empty column, in
// order of first appearance, so the oldest parts of the project sit on the
// left and new folders rise on the right. When there are more than limit,
// the smallest are merged into one OtherName peak at the right end.
func Select(f timeline.Frame, cols []string, limit int) []Peak {
	limit = max(limit, 1)
	var peaks []Peak
	for i, v := range f.Totals {
		if v > 0 {
			peaks = append(peaks, Peak{Name: cols[i], Value: v, Col: i})
		}
	}
	if len(peaks) <= limit {
		return peaks
	}

	bySize := slices.Clone(peaks)
	slices.SortFunc(bySize, func(a, b Peak) int {
		return cmp.Or(cmp.Compare(b.Value, a.Value), cmp.Compare(a.Col, b.Col))
	})
	keep := make(map[int]bool, limit-1)
	for _, p := range bySize[:limit-1] {
		keep[p.Col] = true
	}

	out := make([]Peak, 0, limit)
	other := Peak{Name: OtherName, Col: OtherCol}
	for _, p := range peaks {
		if keep[p.Col] {
			out = append(out, p)
		} else {
			other.Value += p.Value
		}
	}
	return append(out, other)
}
