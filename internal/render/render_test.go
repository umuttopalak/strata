package render

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/umuttopalak/strata/internal/timeline"
)

var update = flag.Bool("update", false, "rewrite golden files")

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestScale(t *testing.T) {
	tests := []struct {
		v, max int64
		want   float64
	}{
		{0, 100, 0},
		{-5, 100, 0},
		{5, 0, 0},
		{100, 100, 1},
		{200, 100, 1},
		{9, 99, math.Log(10) / math.Log(100)},
	}
	for _, tt := range tests {
		if got := Scale(tt.v, tt.max); !near(got, tt.want) {
			t.Errorf("Scale(%d, %d) = %v, want %v", tt.v, tt.max, got, tt.want)
		}
	}
}

func TestCentreOut(t *testing.T) {
	var ranked []Slot
	for _, n := range []string{"A", "B", "C", "D", "E"} {
		ranked = append(ranked, Slot{Name: n})
	}
	var got []string
	for _, s := range centreOut(ranked) {
		got = append(got, s.Name)
	}
	if want := []string{"E", "C", "A", "B", "D"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// timelineOf builds a timeline straight from frames of totals.
func timelineOf(cols []string, frames ...[]int64) *timeline.Timeline {
	tl := &timeline.Timeline{Columns: cols, Peak: make([]int64, len(cols)), Commits: len(frames) - 1}
	for i, totals := range frames {
		f := timeline.Frame{Commit: i, Totals: totals, Touched: make([]int, len(totals))}
		for c, v := range totals {
			tl.Peak[c] = max(tl.Peak[c], v)
			f.Touched[c] = i
		}
		if i > 0 {
			f.Caption = &timeline.Caption{Author: "Ada", Subject: fmt.Sprintf("commit %d", i),
				Time: time.Date(2020, time.Month(i), 1, 0, 0, 0, 0, time.UTC), Added: 10 * i, Deleted: i}
		}
		tl.Frames = append(tl.Frames, f)
	}
	return tl
}

func TestNewLayout(t *testing.T) {
	var cols []string
	var final []int64
	for i := range 15 {
		cols = append(cols, fmt.Sprintf("d%02d", i))
		final = append(final, int64(i+1)*10) // d14 is the largest
	}
	transient := make([]int64, 15)
	transient[0] = 5000 // d00 was huge once, now small
	tl := timelineOf(cols, make([]int64, 15), transient, final)
	l := NewLayout(tl)

	if len(l.Slots) != MaxPeaks {
		t.Fatalf("slots = %d, want %d", len(l.Slots), MaxPeaks)
	}
	// With an even count the largest stands just left of the centre.
	mid := l.Slots[(len(l.Slots)-1)/2]
	if mid.Name != "d14" {
		t.Errorf("middle slot = %s, want the largest final folder d14", mid.Name)
	}
	var other *Slot
	for i := range l.Slots {
		if l.Slots[i].Name == OtherName {
			other = &l.Slots[i]
		}
	}
	// Ranked by final size: d14..d04 kept, d03..d00 merged.
	if other == nil || len(other.Cols) != 4 {
		t.Fatalf("other = %+v, want 4 merged columns", other)
	}
	if l.Max != 5000 {
		t.Errorf("Max = %d, want 5000 (the transient peak)", l.Max)
	}
	if vals := l.Values(tl.Frames[1]); slices.Max(vals) != 5000 {
		t.Errorf("values = %v", vals)
	}
}

func TestNewLayoutFewFolders(t *testing.T) {
	tl := timelineOf([]string{"a", "gone", "b"}, []int64{0, 0, 0}, []int64{10, 0, 0}, []int64{50, 0, 20})
	l := NewLayout(tl)
	var names []string
	for _, s := range l.Slots {
		names = append(names, s.Name)
	}
	// "gone" never had lines, so it gets no mountain; the larger "a" takes
	// the centre-left position and "b" goes to its right.
	if want := []string{"a", "b"}; !slices.Equal(names, want) {
		t.Fatalf("slots = %v, want %v", names, want)
	}
}

func TestHeights(t *testing.T) {
	const width, rows = 90, 20.0
	h, owner := Heights([]int64{1000, 1000, 1000}, 1000, width, rows)

	for x, v := range h {
		if v < 0 || v > rows {
			t.Fatalf("h[%d] = %v out of range", x, v)
		}
	}
	for i := range 3 {
		c := i*30 + 15
		// The summit is the peak's own height plus a little from its
		// neighbours, within the ±6% texture.
		if h[c] < rows*peakFill*0.94 || owner[c] != i {
			t.Errorf("summit %d: h = %v owner = %d", i, h[c], owner[c])
		}
	}
	// Between equal peaks the slopes meet in a valley, not a plateau.
	if valley := h[30]; valley > h[15]*0.8 {
		t.Errorf("valley %v too high next to summit %v", valley, h[15])
	}
	if h[30] < 1 {
		t.Errorf("valley %v: neighbouring peaks should join into a ridge", h[30])
	}

	// Empty input and zero values produce flat ground.
	h, owner = Heights(nil, 100, 10, rows)
	if slices.Max(h) != 0 || slices.Max(owner) != -1 {
		t.Errorf("empty: %v %v", h, owner)
	}
	h, _ = Heights([]int64{0, 0}, 100, 10, rows)
	if slices.Max(h) != 0 {
		t.Errorf("zeros: %v", h)
	}
}

func TestHeightsLogScale(t *testing.T) {
	// Drawn alone so no neighbouring slope adds to them.
	h, _ := Heights([]int64{10}, 100_000, 200, 40)
	small := h[100]
	h, _ = Heights([]int64{100_000}, 100_000, 200, 40)
	large := h[100]
	// log(11)/log(100001) ≈ 0.21: small folders stay visible.
	if small < 40*peakFill*0.18 || small > 40*peakFill*0.26 {
		t.Errorf("small folder height = %v", small)
	}
	if large < 40*peakFill*0.94 {
		t.Errorf("large folder height = %v", large)
	}
}

func TestHeightsCentresLargest(t *testing.T) {
	for n := 1; n <= 12; n++ {
		values := make([]int64, n)
		for i := range values {
			values[i] = 10
		}
		mid := (n - 1) / 2
		values[mid] = 10_000
		h, owner := Heights(values, 10_000, 120, 30)
		// The texture may shift the very highest cell a little on broad
		// hills, but the middle column belongs to the largest folder and
		// stands near full height.
		if owner[60] != mid || h[60] < 30*peakFill*0.93 {
			t.Errorf("n=%d: middle column owner %d height %v", n, owner[60], h[60])
		}
	}
}

func TestTextureStretchesWithSpread(t *testing.T) {
	// Narrow hills keep the spec's ripple unchanged.
	for x := range 20 {
		want := 0.035*math.Sin(float64(x)*1.7) + 0.025*math.Sin(float64(x)*0.63)
		if got := texture(x, 5); !near(got, want) {
			t.Fatalf("texture(%d, 5) = %v, want %v", x, got, want)
		}
	}
	// Wide hills change little from one column to the next.
	for x := range 100 {
		if d := math.Abs(texture(x+1, 30) - texture(x, 30)); d > 0.03 {
			t.Fatalf("texture jumps %v between columns %d and %d", d, x, x+1)
		}
	}
}

func TestCellAt(t *testing.T) {
	tests := []struct {
		h, level float64
		want     Cell
	}{
		{3, 3, Rock},
		{3.2, 3, Rock},
		{2.5, 3, RockHalf},
		{2.7, 3, RockHalf},
		{2.49, 3, Sky},
		{0, 1, Sky},
	}
	for _, tt := range tests {
		if got := cellAt(tt.h, tt.level); got != tt.want {
			t.Errorf("cellAt(%v, %v) = %v, want %v", tt.h, tt.level, got, tt.want)
		}
	}
}

func TestSnowy(t *testing.T) {
	const rows = 20.0
	tests := []struct {
		name    string
		h       float64
		age     int
		commits int
		want    bool
	}{
		{"very tall", 15, 0, 60, true},
		{"tall and old", 10, 10, 60, true},
		{"tall but recent", 10, 9, 60, false},
		{"old but low", 8, 50, 60, false},
		{"low and recent", 3, 0, 60, false},
	}
	for _, tt := range tests {
		if got := snowy(tt.h, rows, tt.age, tt.commits); got != tt.want {
			t.Errorf("%s: snowy = %v", tt.name, got)
		}
	}
}

func TestDrawSnowOnSummit(t *testing.T) {
	tl := timelineOf([]string{"a"}, []int64{0}, []int64{1000})
	p := Draw(tl, NewLayout(tl), 1, "repo", 30, 24)
	for x := range p.Width {
		top := -1
		for r := range p.Cells {
			if p.Cells[r][x] != Sky {
				top = r
				break
			}
		}
		if top < 0 {
			continue
		}
		if c := p.Cells[top][x]; c == Snow || c == SnowHalf {
			for r := top + 1; r < len(p.Cells); r++ {
				if p.Cells[r][x] == Snow || p.Cells[r][x] == SnowHalf {
					t.Fatalf("column %d has snow below its top cell", x)
				}
			}
			return
		}
	}
	t.Fatal("a peak at full height should be snow-capped")
}

func TestDrawChrome(t *testing.T) {
	tl := timelineOf([]string{"src", "docs"}, []int64{0, 0}, []int64{400, 30}, []int64{900, 60})
	tl.Frames[2].Caption.Subject = strings.Repeat("a very long commit message ", 10)
	l := NewLayout(tl)

	for _, size := range [][2]int{{1, 1}, {8, 5}, {20, 6}, {80, 24}, {200, 60}} {
		for i := range tl.Frames {
			p := Draw(tl, l, i, "my-repo", size[0], size[1])
			plain, styled := p.Lines(), p.Styled()
			if len(plain) != max(size[1]-1, 4) || len(styled) != len(plain) {
				t.Fatalf("%v frame %d: %d plain, %d styled lines", size, i, len(plain), len(styled))
			}
			for j := range plain {
				if w := ansi.StringWidth(plain[j]); w != size[0] {
					t.Fatalf("%v frame %d: plain line %d is %d wide: %q", size, i, j, w, plain[j])
				}
				if ansi.Strip(styled[j]) != plain[j] {
					t.Fatalf("%v frame %d: styled line %d differs:\n%q\n%q", size, i, j, ansi.Strip(styled[j]), plain[j])
				}
			}
		}
	}

	p := Draw(tl, l, 2, "my-repo", 80, 24)
	lines := p.Lines()
	if !strings.HasPrefix(lines[0], "$ strata my-repo") || !strings.HasSuffix(lines[0], "2020-02") {
		t.Errorf("header = %q", lines[0])
	}
	footer := lines[len(lines)-1]
	if !strings.HasSuffix(footer, "commit 2/2") || !strings.Contains(footer, "…") ||
		!strings.HasPrefix(footer, "2020-02-01 · Ada · +20 -2 · a very long") {
		t.Errorf("footer = %q", footer)
	}
	// Frame 0 has no commit yet but already shows the date it starts from.
	if first := Draw(tl, l, 0, "my-repo", 80, 24).Lines(); !strings.HasSuffix(first[0], "2020-01") ||
		!strings.HasSuffix(first[len(first)-1], "commit 0/2") {
		t.Errorf("frame 0 = %q / %q", first[0], first[len(first)-1])
	}
}

// Golden files pin down the look. After an intended change, inspect the
// diff and run: go test ./internal/render -update
func TestGolden(t *testing.T) {
	cols := []string{"src", "docs", "(root)", "tests", "vendor", "tools"}
	tl := timelineOf(cols,
		[]int64{0, 0, 0, 0, 0, 0},
		[]int64{120, 10, 30, 0, 0, 0},
		[]int64{5_000, 300, 80, 900, 20_000, 0},
		[]int64{40_000, 2_500, 150, 9_000, 0, 60},
	)
	// vendor was dropped in the last commit; docs was not touched since 1.
	tl.Frames[3].Touched[1] = 1
	l := NewLayout(tl)

	for _, tt := range []struct {
		frame         int
		width, height int
	}{
		{0, 40, 8}, {1, 60, 14}, {2, 60, 14}, {3, 60, 14}, {3, 100, 24},
	} {
		name := fmt.Sprintf("frame%d_%dx%d", tt.frame, tt.width, tt.height)
		t.Run(name, func(t *testing.T) {
			got := strings.Join(Draw(tl, l, tt.frame, "demo", tt.width, tt.height).Lines(), "\n") + "\n"
			path := filepath.Join("testdata", "golden", name+".txt")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if got != string(want) {
				t.Errorf("mismatch with %s\ngot:\n%s\nwant:\n%s", path, got, want)
			}
		})
	}
}
