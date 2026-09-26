package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	rockStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	snowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	groundStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dimStyle    = lipgloss.NewStyle().Faint(true)
	repoStyle   = lipgloss.NewStyle().Bold(true)
	addedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	deletedSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func cellStyle(c Cell) (lipgloss.Style, bool) {
	switch c {
	case Rock, RockHalf:
		return rockStyle, true
	case Snow, SnowHalf:
		return snowStyle, true
	}
	return lipgloss.Style{}, false
}

// Styled returns the picture with ANSI colors, one string per screen row.
// Colors are written in full; send the output through a color profile
// writer to downsample them, or drop them for NO_COLOR and pipes.
func (p Picture) Styled() []string {
	name, gap, date := p.header()
	out := []string{dimStyle.Render(ansi.Truncate(prompt, p.Width, "")) + repoStyle.Render(name) +
		strings.Repeat(" ", gap) + dimStyle.Render(date)}

	var b strings.Builder
	for _, row := range p.Cells {
		b.Reset()
		for x := 0; x < len(row); {
			end := x + 1
			for end < len(row) && sameStyle(row[end], row[x]) {
				end++
			}
			var run strings.Builder
			for _, c := range row[x:end] {
				run.WriteString(cellRunes[c])
			}
			if st, ok := cellStyle(row[x]); ok {
				b.WriteString(st.Render(run.String()))
			} else {
				b.WriteString(run.String())
			}
			x = end
		}
		out = append(out, b.String())
	}

	return append(out,
		groundStyle.Render(strings.Repeat("▔", p.Width)),
		p.footer(p.styledCaption()))
}

func sameStyle(a, b Cell) bool {
	class := func(c Cell) int {
		switch c {
		case Rock, RockHalf:
			return 1
		case Snow, SnowHalf:
			return 2
		}
		return 0
	}
	return class(a) == class(b)
}

// styledCaption colors the +added and -deleted counts of the fitted caption.
func (p Picture) styledCaption() string {
	text := p.captionText()
	stats := fmt.Sprintf("+%d -%d", p.Added, p.Deleted)
	before, after, found := strings.Cut(text, stats)
	if !found || p.Caption == "" {
		return dimStyle.Render(text)
	}
	return dimStyle.Render(before) +
		addedStyle.Render(fmt.Sprintf("+%d", p.Added)) + " " +
		deletedSty.Render(fmt.Sprintf("-%d", p.Deleted)) +
		after
}
