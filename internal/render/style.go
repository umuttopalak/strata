package render

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// palette holds earthy mid-tones that read on both dark and light
// backgrounds. Colors follow the column, not the position, so a mountain
// keeps its color when neighbours appear or disappear. Language-based
// colors replace this later.
var palette = []color.Color{
	lipgloss.Color("#7FA66B"), // moss
	lipgloss.Color("#C08A5B"), // clay
	lipgloss.Color("#6C8EBF"), // slate blue
	lipgloss.Color("#B5A55A"), // ochre
	lipgloss.Color("#9B7FB0"), // heather
	lipgloss.Color("#5FA3A0"), // lichen
	lipgloss.Color("#B0736F"), // red rock
	lipgloss.Color("#8C9A6E"), // sage
}

var (
	otherColor  = lipgloss.Color("#8A8A8A")
	groundStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B5E4E"))
	labelStyle  = lipgloss.NewStyle().Faint(true)
)

func peakStyle(p Peak) lipgloss.Style {
	c := otherColor
	if p.Col != OtherCol {
		c = palette[p.Col%len(palette)]
	}
	return lipgloss.NewStyle().Foreground(c)
}

// Styled returns the picture with colors. Runs of cells that belong to the
// same peak are styled together to keep the escape sequences short. Colors
// are written as truecolor; print through a lipgloss writer (or Bubble Tea)
// to downsample them or drop them for NO_COLOR and pipes.
func (p Picture) Styled() []string {
	styles := make([]lipgloss.Style, len(p.Peaks))
	for i, pk := range p.Peaks {
		styles[i] = peakStyle(pk)
	}

	out := make([]string, 0, p.Rows+2)
	var b strings.Builder
	for y, row := range p.Cells {
		b.Reset()
		for x := 0; x < len(row); {
			owner := p.Owner[y][x]
			end := x + 1
			for end < len(row) && p.Owner[y][end] == owner {
				end++
			}
			run := string(row[x:end])
			if owner >= 0 {
				run = styles[owner].Render(run)
			}
			b.WriteString(run)
			x = end
		}
		out = append(out, b.String())
	}
	return append(out,
		groundStyle.Render(strings.Repeat("─", p.Width)),
		labelStyle.Render(p.Labels))
}
