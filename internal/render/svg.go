package render

import (
	"bufio"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/umuttopalak/strata/internal/timeline"
)

// SVGOptions configures an animated SVG export.
type SVGOptions struct {
	Repo          string
	Cols, Height  int           // emulated terminal size in cells; Height includes header and caption rows
	FrameDuration time.Duration // time per timeline frame
	Hold          time.Duration // how long the last frame stays before the loop restarts
	MinPlay       time.Duration // short histories are slowed down to last at least this long
	MaxKeyframes  int           // frames sampled into the animation; fewer means a smaller file
}

// DefaultSVGOptions are sized for a README: about 800×520 pixels.
var DefaultSVGOptions = SVGOptions{
	Cols: 100, Height: 30,
	FrameDuration: 75 * time.Millisecond,
	Hold:          4 * time.Second,
	MinPlay:       6 * time.Second,
	MaxKeyframes:  120,
}

// Cell size in pixels: terminal cells are about twice as tall as wide.
const (
	cellW    = 8
	cellH    = 16
	svgPad   = 16
	titleBar = 28
	fontSize = 13 // monospace fonts advance about 0.6em, close to cellW
)

// WriteSVG writes the replay as a self-contained animated SVG. It uses SMIL
// animation only (no scripts), so it plays inside a GitHub README image.
// Renderers without SMIL show the final frame.
//
// Each keyframe is drawn with Draw, so the SVG matches the terminal cell
// for cell: every column is a rock rectangle whose top follows the
// silhouette, with a snow rectangle on top when its summit is snowy.
// Between keyframes the heights move linearly, like the player's easing.
func WriteSVG(w io.Writer, tl *timeline.Timeline, l Layout, o SVGOptions) error {
	if o.Cols < 1 || o.Height < chromeRows+1 {
		return fmt.Errorf("svg size must be at least 1x%d cells", chromeRows+1)
	}
	if o.MaxKeyframes < 2 {
		o.MaxKeyframes = 2
	}

	frames := sampleFrames(len(tl.Frames), o.MaxKeyframes)
	pics := make([]Picture, len(frames))
	for k, i := range frames {
		pics[k] = Draw(tl, l, i, o.Repo, o.Cols, o.Height)
	}
	a := newAnimation(frames, o)

	rows := len(pics[0].Cells)
	captionRow := 1 // rows below the ground line
	if pics[0].Labels != nil {
		captionRow = 2
	}
	top := svgPad + titleBar           // y of the header row
	areaTop := top + cellH             // first mountain row
	areaBottom := areaTop + rows*cellH // ground line
	width := 2*svgPad + o.Cols*cellW   // whole image
	height := top + o.Height*cellH + svgPad

	b := bufio.NewWriter(w)
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" xml:space="preserve" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`+"\n",
		width, height, width, height)
	fmt.Fprintf(b, "<title>strata · %s</title>\n", html.EscapeString(o.Repo))
	b.WriteString(`<style>
text{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono",monospace;font-size:` + strconv.Itoa(fontSize) + `px;white-space:pre;fill:#e6edf3}
.d{fill:#8b949e}.b{font-weight:bold}.e{fill:#d29922;font-weight:bold}.g{fill:#3fb950}.r{fill:#f85149}
.k{fill:#8a8a8a}.s{fill:#f5f5f5}
</style>
`)
	// Terminal window.
	fmt.Fprintf(b, `<rect x="0.5" y="0.5" width="%d" height="%d" rx="8" fill="#0d1117" stroke="#30363d"/>`+"\n",
		width-1, height-1)
	for i, c := range []string{"#ff5f57", "#febc2e", "#28c840"} {
		fmt.Fprintf(b, `<circle cx="%d" cy="%d" r="6" fill="%s"/>`+"\n", svgPad+6+i*20, svgPad+6, c)
	}

	// Header: the prompt and repository are fixed; the date changes.
	name := pics[0].header().name
	fmt.Fprintf(b, `<text x="%d" y="%d"><tspan class="d">%s</tspan><tspan class="b">%s</tspan></text>`+"\n",
		svgPad, baseline(top), html.EscapeString(prompt), html.EscapeString(name))

	// Mountains, clipped to the area above the ground.
	fmt.Fprintf(b, `<clipPath id="a"><rect x="%d" y="%d" width="%d" height="%d"/></clipPath>`+"\n",
		svgPad, areaTop, o.Cols*cellW, rows*cellH)
	b.WriteString(`<g clip-path="url(#a)">` + "\n")
	for x := range o.Cols {
		writeColumn(b, a, pics, x, svgPad+x*cellW, areaBottom, rows)
	}
	b.WriteString("</g>\n")
	fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" fill="#585858"/>`+"\n",
		svgPad, areaBottom, o.Cols*cellW, cellH/8)

	// Date, caption and counter for each keyframe, shown in turn.
	right := svgPad + o.Cols*cellW
	for k, p := range pics {
		fmt.Fprintf(b, `<g visibility="%s">%s`, a.baseVisibility(k), a.visibility(k))
		fmt.Fprintf(b, `<text x="%d" y="%d" text-anchor="end" class="d">%s</text>`,
			right, baseline(top), html.EscapeString(p.Date))
		if h := p.header(); h.event != "" {
			fmt.Fprintf(b, `<text x="%d" y="%d" class="e">%s</text>`,
				svgPad+h.eventAt*cellW, baseline(top), html.EscapeString(h.event))
		}
		if p.Labels != nil {
			fmt.Fprintf(b, `<text x="%d" y="%d" class="d">%s</text>`,
				svgPad, baseline(areaBottom+cellH), html.EscapeString(*p.Labels))
		}
		fmt.Fprintf(b, `<text x="%d" y="%d">%s</text>`, svgPad, baseline(areaBottom+captionRow*cellH), captionSpans(p))
		fmt.Fprintf(b, `<text x="%d" y="%d" text-anchor="end" class="d">%s</text>`,
			right, baseline(areaBottom+captionRow*cellH), html.EscapeString(p.right()))
		b.WriteString("</g>\n")
	}
	// The closing line appears while the last frame holds.
	fmt.Fprintf(b, `<text x="%d" y="%d" class="g">%s%s</text>`+"\n",
		svgPad, baseline(areaBottom+(captionRow+1)*cellH), a.holdVisibility(), html.EscapeString(FinishedText(tl)))

	b.WriteString("</svg>\n")
	return b.Flush()
}

// FinishedText is the line shown when a replay ends.
func FinishedText(tl *timeline.Timeline) string {
	if tl.Commits == 1 {
		return "✓ done · 1 commit"
	}
	return fmt.Sprintf("✓ done · %d commits", tl.Commits)
}

func baseline(rowTop int) int { return rowTop + cellH*3/4 }

// captionSpans colors the +added -deleted part of the fitted caption.
func captionSpans(p Picture) string {
	text := p.captionText()
	stats := fmt.Sprintf("+%d -%d", p.Added, p.Deleted)
	before, after, found := strings.Cut(text, stats)
	if !found || p.Caption == "" {
		return `<tspan class="d">` + html.EscapeString(text) + `</tspan>`
	}
	return fmt.Sprintf(`<tspan class="d">%s</tspan><tspan class="g">+%d</tspan> <tspan class="r">-%d</tspan>%s`,
		html.EscapeString(before), p.Added, p.Deleted, html.EscapeString(after))
}

// column reads one column of a picture: its height and snow depth, both in
// half rows, from the cells Draw produced.
func column(p Picture, x int) (height, snow int) {
	rows := len(p.Cells)
	for r, row := range p.Cells {
		level := rows - r
		switch row[x] {
		case Rock:
			return 2 * level, 0
		case Snow:
			return 2 * level, 2
		case RockHalf:
			return 2*level - 1, 0
		case SnowHalf:
			return 2*level - 1, 1
		}
	}
	return 0, 0
}

// writeColumn emits one column as a group that slides up and down: a rock
// rectangle reaching below the ground (clipped) and, if the column is ever
// snowy, a snow rectangle whose height animates. Columns that stay empty
// are left out.
func writeColumn(b *bufio.Writer, a animation, pics []Picture, x, px, bottom, rows int) {
	ys := make([]int, len(pics))
	snows := make([]int, len(pics))
	anyRock, anySnow := false, false
	for k, p := range pics {
		h, s := column(p, x)
		ys[k] = bottom - h*cellH/2
		snows[k] = s * cellH / 2
		anyRock = anyRock || h > 0
		anySnow = anySnow || s > 0
	}
	if !anyRock {
		return
	}
	last := len(pics) - 1
	fmt.Fprintf(b, `<g transform="translate(0 %d)">`, ys[last])
	if !a.static() {
		fmt.Fprintf(b, `<animateTransform attributeName="transform" type="translate" values="%s" dur="%s" repeatCount="indefinite"/>`,
			a.values(ys, func(v int) string { return "0 " + strconv.Itoa(v) }), a.dur())
	}
	// Slightly wider than a cell so neighbours overlap without hairline gaps.
	fmt.Fprintf(b, `<rect x="%d" width="%g" height="%d" class="k"/>`, px, cellW+0.6, rows*cellH)
	if anySnow {
		fmt.Fprintf(b, `<rect x="%d" width="%g" height="%d" class="s">`, px, cellW+0.6, snows[last])
		if !a.static() {
			fmt.Fprintf(b, `<animate attributeName="height" values="%s" dur="%s" repeatCount="indefinite"/>`,
				a.values(snows, strconv.Itoa), a.dur())
		}
		b.WriteString("</rect>")
	}
	b.WriteString("</g>\n")
}

// sampleFrames picks up to max frame indices spread evenly over n frames,
// always including the first and the last.
func sampleFrames(n, max int) []int {
	if n <= max {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	out := make([]int, max)
	for k := range out {
		out[k] = int(float64(k)*float64(n-1)/float64(max-1) + 0.5)
	}
	return out
}

// animation holds the shared timing: keyframes evenly spaced in time,
// followed by hold steps that repeat the last one. Evenly spaced values
// need no keyTimes, which keeps every per-column list short.
type animation struct {
	keyframes int
	hold      int     // extra steps repeating the last keyframe
	step      float64 // seconds between values
}

// PlayTime is how long a replay of n timeline frames runs before the hold.
func (o SVGOptions) PlayTime(n int) time.Duration {
	if n < 2 {
		return 0
	}
	return max(time.Duration(n-1)*o.FrameDuration, o.MinPlay)
}

func newAnimation(frames []int, o SVGOptions) animation {
	a := animation{keyframes: len(frames)}
	if len(frames) < 2 {
		return a
	}
	play := o.PlayTime(frames[len(frames)-1] + 1).Seconds()
	a.step = play / float64(len(frames)-1)
	if a.step > 0 {
		a.hold = int(o.Hold.Seconds()/a.step + 0.5)
	}
	return a
}

func (a animation) static() bool { return a.keyframes < 2 || a.step <= 0 }

func (a animation) steps() int { return a.keyframes - 1 + a.hold }

func (a animation) dur() string {
	return strconv.FormatFloat(float64(a.steps())*a.step, 'f', 2, 64) + "s"
}

// values joins per-keyframe values, padded with the hold.
func (a animation) values(vs []int, format func(int) string) string {
	var sb strings.Builder
	for k, v := range vs {
		if k > 0 {
			sb.WriteByte(';')
		}
		sb.WriteString(format(v))
	}
	for range a.hold {
		sb.WriteByte(';')
		sb.WriteString(format(vs[len(vs)-1]))
	}
	return sb.String()
}

// keyTime returns the fraction of the loop at which keyframe k starts.
func (a animation) keyTime(k int) string {
	return strconv.FormatFloat(float64(k)/float64(a.steps()), 'f', 4, 64)
}

// baseVisibility is what renderers without SMIL show: the last keyframe.
func (a animation) baseVisibility(k int) string {
	if k == a.keyframes-1 {
		return "visible"
	}
	return "hidden"
}

// visibility shows keyframe k's texts from its start until the next one;
// the last stays through the hold.
func (a animation) visibility(k int) string {
	if a.static() {
		return ""
	}
	var values, times string
	switch {
	case k == 0:
		values, times = "visible;hidden", "0;"+a.keyTime(1)
	case k == a.keyframes-1:
		values, times = "hidden;visible", "0;"+a.keyTime(k)
	default:
		values, times = "hidden;visible;hidden", "0;"+a.keyTime(k)+";"+a.keyTime(k+1)
	}
	return fmt.Sprintf(`<animate attributeName="visibility" calcMode="discrete" values="%s" keyTimes="%s" dur="%s" repeatCount="indefinite"/>`,
		values, times, a.dur())
}

// holdVisibility shows an element only while the last frame holds.
func (a animation) holdVisibility() string {
	if a.static() {
		return ""
	}
	return fmt.Sprintf(`<animate attributeName="visibility" calcMode="discrete" values="hidden;visible" keyTimes="0;%s" dur="%s" repeatCount="indefinite"/>`,
		a.keyTime(a.keyframes-1), a.dur())
}

// WriteMessageSVG writes a small image in the same terminal style with a
// title and a few lines of text, for when a repository cannot be drawn
// (not found, too large) or is still being drawn.
func WriteMessageSVG(w io.Writer, title string, lines ...string) error {
	width := 2*svgPad + DefaultSVGOptions.Cols*cellW
	top := svgPad + titleBar
	height := top + (len(lines)+2)*cellH + svgPad

	b := bufio.NewWriter(w)
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" xml:space="preserve" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`+"\n",
		width, height, width, height)
	fmt.Fprintf(b, "<title>strata · %s</title>\n", html.EscapeString(title))
	b.WriteString(`<style>text{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono",monospace;font-size:` +
		strconv.Itoa(fontSize) + `px;white-space:pre;fill:#8b949e}.b{font-weight:bold;fill:#e6edf3}</style>` + "\n")
	fmt.Fprintf(b, `<rect x="0.5" y="0.5" width="%d" height="%d" rx="8" fill="#0d1117" stroke="#30363d"/>`+"\n", width-1, height-1)
	for i, c := range []string{"#ff5f57", "#febc2e", "#28c840"} {
		fmt.Fprintf(b, `<circle cx="%d" cy="%d" r="6" fill="%s"/>`+"\n", svgPad+6+i*20, svgPad+6, c)
	}
	fmt.Fprintf(b, `<text x="%d" y="%d">%s<tspan class="b">%s</tspan></text>`+"\n",
		svgPad, baseline(top), html.EscapeString(prompt), html.EscapeString(title))
	for i, line := range lines {
		fmt.Fprintf(b, `<text x="%d" y="%d">%s</text>`+"\n", svgPad, baseline(top+(i+2)*cellH), html.EscapeString(line))
	}
	b.WriteString("</svg>\n")
	return b.Flush()
}
