package render

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/umuttopalak/strata/internal/timeline"
)

// Cell is what one terminal cell of the mountain area shows.
type Cell uint8

const (
	Sky Cell = iota
	Rock
	RockHalf
	Snow
	SnowHalf
)

var cellRunes = [...]string{Sky: " ", Rock: "█", RockHalf: "▄", Snow: "▓", SnowHalf: "▄"}

// Snow rules: a summit is snow-capped when it is very tall, or when it is
// fairly tall and its folder has not been touched for a long stretch.
const (
	snowHeight     = 0.70 // of the mountain area, always snowy above this
	oldSnowHeight  = 0.45 // of the mountain area, snowy above this when old
	oldCommitShare = 6    // untouched for 1/oldCommitShare of all commits
)

// chromeRows are the rows around the mountains: the header above, the
// ground line and caption below, and one spare row at the bottom so a
// closing message fits without scrolling the picture.
const chromeRows = 4

// Picture is one full screen: header, mountains, ground and caption.
type Picture struct {
	Width    int
	Repo     string
	Date     string   // YYYY-MM shown in the header
	Cells    [][]Cell // mountain rows, top first
	Caption  string   // date · author · +added -deleted · subject
	Progress string   // commit n/total
	Status   string   // shown before Progress, e.g. "paused" or "2×"
	Labels   *string  // folder names under the ground line, nil when off
	Added    int
	Deleted  int
}

// Draw renders frame i of tl for a terminal of width × height cells.
func Draw(tl *timeline.Timeline, l Layout, i int, repo string, width, height int) Picture {
	return DrawAt(tl, l, float64(i), repo, width, height)
}

// DrawAt renders a moment between two frames: pos 4.25 is a quarter of the
// way from frame 4 to frame 5. Heights ease between the two, so mountains
// grow and erode smoothly; the caption and snow follow frame 5, the commit
// being applied.
func DrawAt(tl *timeline.Timeline, l Layout, pos float64, repo string, width, height int) Picture {
	width = max(width, 1)
	rows := max(height-chromeRows, 1)
	if l.Labels {
		rows = max(rows-1, 1)
	}
	pos = math.Min(math.Max(pos, 0), float64(len(tl.Frames)-1))
	i := int(math.Ceil(pos))
	f := tl.Frames[i]

	values := l.Values(f)
	h, owner := Heights(values, l.Max, width, float64(rows))
	if prev := int(math.Floor(pos)); prev != i {
		from, _ := Heights(l.Values(tl.Frames[prev]), l.Max, width, float64(rows))
		t := smoothstep(pos - float64(prev))
		for x := range h {
			h[x] = from[x] + (h[x]-from[x])*t
		}
	}
	touched := l.Touched(f)
	cells := make([][]Cell, rows)
	for r := range cells {
		cells[r] = make([]Cell, width)
	}
	for x, hx := range h {
		for r := range rows {
			cells[r][x] = cellAt(hx, float64(rows-r))
		}
		if owner[x] >= 0 && snowy(hx, float64(rows), f.Commit-touched[owner[x]], tl.Commits) {
			if top := rows - int(math.Floor(hx+0.5)); top >= 0 && top < rows {
				cells[top][x] += Snow - Rock // Rock→Snow, RockHalf→SnowHalf
			}
		}
	}

	p := Picture{
		Width:    width,
		Repo:     repo,
		Cells:    cells,
		Progress: fmt.Sprintf("commit %d/%d", f.Commit, tl.Commits),
	}
	if c := captionFor(tl, i); c != nil {
		p.Date = c.Time.Format("2006-01")
	}
	if c := f.Caption; c != nil {
		p.Caption = fmt.Sprintf("%s · %s · +%d -%d · %s",
			c.Time.Format("2006-01-02"), c.Author, c.Added, c.Deleted, c.Subject)
		p.Added, p.Deleted = c.Added, c.Deleted
	}
	if l.Labels {
		line := labelLine(l, values, width)
		p.Labels = &line
	}
	return p
}

// labelLine writes each standing mountain's folder name under its summit.
// Larger folders are placed first and keep their full name when there is
// room; a name may slide up to half a slot off centre to find space, and is
// shortened (down to minLabel cells) before being left out.
func labelLine(l Layout, values []int64, width int) string {
	line := []rune(strings.Repeat(" ", width))
	n := len(l.Slots)
	if n == 0 {
		return string(line)
	}
	centres := slotCentres(n, width)
	slide := int(slotGap(n, width) / 2)
	used := make([]bool, width)

	order := make([]int, 0, n)
	for i, v := range values {
		if v > 0 {
			order = append(order, i)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(values[b], values[a]) })

	for _, i := range order {
		full := []rune(l.Slots[i].Name)
		for size := len(full); size >= min(minLabel, len(full)); size-- {
			name := []rune(shorten(l.Slots[i].Name, size))
			if at, ok := freeSpan(used, len(name), int(math.Round(centres[i]-float64(len(name))/2)), slide); ok {
				copy(line[at:], name)
				// Reserve one blank cell on each side so names never touch.
				for x := max(at-1, 0); x < min(at+len(name)+1, width); x++ {
					used[x] = true
				}
				break
			}
		}
	}
	return string(line)
}

// minLabel is the shortest a label is cut to before it is dropped.
const minLabel = 3

// freeSpan finds a start for n cells near want (within slide) where none is
// used and the text stays on screen, preferring positions closest to want.
func freeSpan(used []bool, n, want, slide int) (int, bool) {
	fits := func(at int) bool {
		if at < 0 || at+n > len(used) {
			return false
		}
		return !slices.Contains(used[at:at+n], true)
	}
	for d := 0; d <= slide; d++ {
		if fits(want - d) {
			return want - d, true
		}
		if fits(want + d) {
			return want + d, true
		}
	}
	return 0, false
}

// shorten fits a folder path into n cells. The last component says the
// most, so "vendor/github.com/lib" becomes "…/lib" before being cut.
func shorten(path string, n int) string {
	r := []rune(path)
	if len(r) <= n {
		return path
	}
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		tail := []rune("…/" + path[i+1:])
		if len(tail) <= n {
			return string(tail)
		}
		r = []rune(path[i+1:])
		if len(r) <= n {
			return string(r)
		}
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// smoothstep eases t in 0..1 so motion starts and stops gently.
func smoothstep(t float64) float64 {
	return t * t * (3 - 2*t)
}

// cellAt draws a column of height h at the given level (1 is the bottom
// row): full below the height, a half block where the top ends in the
// lower half of the cell, which rounds off the summits.
func cellAt(h, level float64) Cell {
	switch {
	case h >= level:
		return Rock
	case h >= level-0.5:
		return RockHalf
	}
	return Sky
}

func snowy(h, rows float64, age, commits int) bool {
	if h > snowHeight*rows {
		return true
	}
	old := float64(age) >= float64(commits)/oldCommitShare
	return old && h > oldSnowHeight*rows
}

// captionFor returns the caption dating frame i. The first frame has none
// of its own, so it borrows the date of the commit about to land.
func captionFor(tl *timeline.Timeline, i int) *timeline.Caption {
	if c := tl.Frames[i].Caption; c != nil {
		return c
	}
	if i+1 < len(tl.Frames) {
		return tl.Frames[i+1].Caption
	}
	return nil
}

// Lines returns the picture as plain text, one string per screen row.
func (p Picture) Lines() []string {
	name, gap, date := p.header()
	out := []string{ansi.Truncate(prompt+name, p.Width, "") + strings.Repeat(" ", gap) + date}
	for _, row := range p.Cells {
		var b strings.Builder
		for _, c := range row {
			b.WriteString(cellRunes[c])
		}
		out = append(out, b.String())
	}
	out = append(out, strings.Repeat("▔", p.Width))
	if p.Labels != nil {
		out = append(out, *p.Labels)
	}
	return append(out, p.footer(p.captionText()))
}

const prompt = "$ strata "

// header lays out "$ strata <repo>" on the left and the date on the right:
// it returns the (possibly shortened) repo name, the gap and the date.
func (p Picture) header() (name string, gap int, date string) {
	date = p.Date
	if len(prompt)+1+len(date) > p.Width {
		date = ""
	}
	room := max(p.Width-len(prompt)-len(date)-1, 0)
	name = ansi.Truncate(p.Repo, room, "…")
	gap = max(p.Width-len(prompt)-ansi.StringWidth(name)-len(date), 0)
	return name, gap, date
}

// right is the text pinned to the right edge of the caption line.
func (p Picture) right() string {
	if p.Status == "" {
		return p.Progress
	}
	return p.Status + "  " + p.Progress
}

// captionText fits the caption into the space left of the progress counter.
func (p Picture) captionText() string {
	room := p.Width - ansi.StringWidth(p.right()) - 2
	if room < 1 {
		return ""
	}
	return ansi.Truncate(p.Caption, room, "…")
}

// footer puts a (possibly styled) caption on the left and the progress
// counter on the right edge.
func (p Picture) footer(caption string) string {
	right := p.right()
	gap := p.Width - ansi.StringWidth(caption) - ansi.StringWidth(right)
	if gap < 1 {
		return ansi.Truncate(right, p.Width, "")
	}
	return caption + strings.Repeat(" ", gap) + right
}
