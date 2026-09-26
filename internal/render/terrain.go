package render

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/umuttopalak/summit/internal/timeline"
)

// eighths are the block characters for 0/8 … 8/8 of a cell's height.
var eighths = []rune(" ▁▂▃▄▅▆▇█")

// cellAspect is a terminal cell's height divided by its width.
const cellAspect = 2.0

// slotWidth is the narrowest a peak may be, in cells, before columns get
// merged into OtherName. It leaves room for a label, and grows with the
// drawing's height so tall mountains are not squeezed into towers.
func slotWidth(rows int) int {
	return max(10, rows*4/5)
}

// Picture is one frame drawn onto a grid of terminal cells.
type Picture struct {
	Width, Rows int      // Rows is the mountain height, excluding ground and labels
	Cells       [][]rune // Rows × Width
	Owner       [][]int  // peak index per cell, -1 for sky
	Peaks       []Peak
	Terrain     Terrain
	Labels      string // Width cells of peak names under their summits
}

// Draw renders a frame at the given size. rows is the height available for
// the mountains; Lines adds a ground line and a label line under them.
func Draw(f timeline.Frame, cols []string, width, rows int) Picture {
	width, rows = max(width, 1), max(rows, 1)
	peaks := Select(f, cols, max(width/slotWidth(rows), 1))
	t := Shape(peaks, f.Ceiling, width, float64(rows)*cellAspect)
	p := Picture{Width: width, Rows: rows, Peaks: peaks, Terrain: t}
	p.Cells, p.Owner = rasterize(t, rows)
	p.Labels = labels(peaks, t.Centers, width)
	return p
}

// rasterize fills each column of cells up to its height with an eighth-block
// precision cap, so slopes look smooth instead of stair-stepped.
func rasterize(t Terrain, rows int) ([][]rune, [][]int) {
	width := len(t.Heights)
	cells := make([][]rune, rows)
	owner := make([][]int, rows)
	for y := range rows {
		cells[y] = make([]rune, width)
		owner[y] = make([]int, width)
	}
	for x, h := range t.Heights {
		level := int(math.Round(h * float64(rows*8)))
		for y := range rows {
			fill := min(max(level-(rows-1-y)*8, 0), 8)
			cells[y][x] = eighths[fill]
			owner[y][x] = -1
			if fill > 0 {
				owner[y][x] = t.Owner[x]
			}
		}
	}
	return cells, owner
}

// labels centres each peak's name under its summit, trimmed to its slot.
// A label that would collide with the previous one is dropped.
func labels(peaks []Peak, centers []float64, width int) string {
	line := []rune(strings.Repeat(" ", width))
	if len(peaks) == 0 {
		return string(line)
	}
	slot := width / len(peaks)
	next := 0 // first free cell
	for i, p := range peaks {
		name := shorten(p.Name, max(slot-1, 1))
		n := utf8.RuneCountInString(name)
		start := int(math.Round(centers[i] - float64(n)/2))
		start = min(max(start, next), width-n)
		if start < next || start < 0 {
			continue
		}
		copy(line[start:], []rune(name))
		next = start + n + 1
	}
	return string(line)
}

// shorten fits a folder path into n cells. The last component says the
// most, so "vendor/github.com/lib" becomes "…/lib" before being cut.
func shorten(path string, n int) string {
	if utf8.RuneCountInString(path) <= n {
		return path
	}
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		if tail := "…/" + path[i+1:]; utf8.RuneCountInString(tail) <= n {
			return tail
		}
		path = path[i+1:]
	}
	r := []rune(path)
	if len(r) <= n {
		return path
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// Lines returns the picture as plain text: mountains, ground and labels.
func (p Picture) Lines() []string {
	out := make([]string, 0, p.Rows+2)
	for _, row := range p.Cells {
		out = append(out, string(row))
	}
	return append(out, strings.Repeat("─", p.Width), p.Labels)
}
