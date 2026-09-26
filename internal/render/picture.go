package render

import (
	"fmt"
	"math"
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
	pos = math.Min(math.Max(pos, 0), float64(len(tl.Frames)-1))
	i := int(math.Ceil(pos))
	f := tl.Frames[i]

	h, owner := Heights(l.Values(f), l.Max, width, float64(rows))
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
	return p
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
	return append(out, strings.Repeat("▔", p.Width), p.footer(p.captionText()))
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
