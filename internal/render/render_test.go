package render

import (
	"flag"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/umuttopalak/summit/internal/timeline"
)

var update = flag.Bool("update", false, "rewrite golden files")

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestScale(t *testing.T) {
	tests := []struct {
		v, ceiling int64
		want       float64
	}{
		{0, 100, 0},
		{-5, 100, 0},
		{5, 0, 0},
		{100, 100, 1},
		{200, 100, 1}, // above the ceiling clamps
		{10_000, 100_000, 0.75},
		{1_000, 100_000, 0.5},
		{1, 100_000_000, minHeight},
	}
	for _, tt := range tests {
		if got := Scale(tt.v, tt.ceiling); !near(got, tt.want) {
			t.Errorf("Scale(%d, %d) = %v, want %v", tt.v, tt.ceiling, got, tt.want)
		}
	}
}

func frameOf(totals ...int64) timeline.Frame {
	f := timeline.Frame{Totals: totals}
	for _, v := range totals {
		f.Ceiling = max(f.Ceiling, v)
	}
	return f
}

func names(ps []Peak) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestSelect(t *testing.T) {
	cols := []string{"a", "b", "c", "d", "e"}
	f := frameOf(50, 0, 10, 300, 20)

	if got := names(Select(f, cols, 10)); !slices.Equal(got, []string{"a", "c", "d", "e"}) {
		t.Errorf("all fit: %v", got)
	}

	got := Select(f, cols, 3)
	if n := names(got); !slices.Equal(n, []string{"a", "d", OtherName}) {
		t.Fatalf("merged: %v", n)
	}
	if other := got[2]; other.Value != 30 || other.Col != OtherCol {
		t.Errorf("other = %+v, want 30 lines", other)
	}

	if n := names(Select(f, cols, 0)); !slices.Equal(n, []string{OtherName}) {
		t.Errorf("limit 0: %v", n)
	}
	if n := names(Select(frameOf(0, 0), cols, 3)); len(n) != 0 {
		t.Errorf("empty frame: %v", n)
	}
}

func TestShape(t *testing.T) {
	peaks := []Peak{{"a", 1000, 0}, {"b", 100_000, 1}, {"c", 10, 2}}
	ter := Shape(peaks, 100_000, 90, 40)

	for i, p := range peaks {
		// Samples are centred on cells; slot centres fall between two cells.
		c := ter.Centers[i]
		if !near(c, float64(i)*30+15) {
			t.Errorf("%s centre = %v", p.Name, c)
		}
		x := int(c)
		want := Scale(p.Value, 100_000)
		if got := ter.Heights[x]; got > want+1e-9 || got < want*0.95 {
			t.Errorf("%s summit = %v, want about %v", p.Name, got, want)
		}
		if ter.Owner[x] != i {
			t.Errorf("%s summit owned by %d", p.Name, ter.Owner[x])
		}
	}
	for x, h := range ter.Heights {
		if h < 0 || h > 1 {
			t.Fatalf("height[%d] = %v out of range", x, h)
		}
		if (h == 0) != (ter.Owner[x] == -1) {
			t.Fatalf("sample %d: height %v with owner %d", x, h, ter.Owner[x])
		}
	}
	// There must be a valley between the tall peak and its neighbours.
	if valley := ter.Heights[30]; valley >= ter.Heights[45]*0.8 {
		t.Errorf("no valley: %v vs summit %v", valley, ter.Heights[45])
	}

	empty := Shape(nil, 100, 10, 10)
	if len(empty.Heights) != 10 || slices.Max(empty.Heights) != 0 {
		t.Errorf("empty terrain = %+v", empty)
	}
}

func TestRasterize(t *testing.T) {
	ter := Terrain{Heights: []float64{0, 0.5, 1, 1.0 / 16}, Owner: []int{-1, 0, 1, 2}}
	cells, owner := rasterize(ter, 2)
	got := []string{string(cells[0]), string(cells[1])}
	want := []string{"  █ ", " ██▁"}
	if !slices.Equal(got, want) {
		t.Fatalf("cells = %q, want %q", got, want)
	}
	if !slices.Equal(owner[0], []int{-1, -1, 1, -1}) || !slices.Equal(owner[1], []int{-1, 0, 1, 2}) {
		t.Fatalf("owner = %v", owner)
	}
}

func TestShorten(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"src", 10, "src"},
		{"node_modules/lodash", 10, "…/lodash"},
		{"node_modules/lodash", 7, "lodash"},
		{"node_modules/lodash", 4, "lod…"},
		{"verylongname", 5, "very…"},
		{"x", 1, "x"},
		{"xy", 1, "…"},
	}
	for _, tt := range tests {
		if got := shorten(tt.in, tt.n); got != tt.want {
			t.Errorf("shorten(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

func TestDrawDimensions(t *testing.T) {
	cols := []string{"src", "docs", "a-very-long-folder-name"}
	for _, size := range [][2]int{{1, 1}, {7, 3}, {80, 20}, {200, 50}} {
		p := Draw(frameOf(100, 20, 5), cols, size[0], size[1])
		lines := p.Lines()
		if len(lines) != size[1]+2 {
			t.Fatalf("%v: %d lines", size, len(lines))
		}
		for i, l := range lines {
			if n := utf8.RuneCountInString(l); n != size[0] {
				t.Fatalf("%v: line %d is %d cells wide: %q", size, i, n, l)
			}
		}
		if styled := p.Styled(); len(styled) != len(lines) {
			t.Fatalf("%v: styled has %d lines", size, len(styled))
		}
	}
}

// Golden files pin down the look of the terrain. After an intended change
// in shape, inspect the diff and run: go test ./internal/render -update
func TestGolden(t *testing.T) {
	cols := []string{"src", "docs", "(root)", "tests", "vendor", "tools", "web", "api"}
	tests := []struct {
		name          string
		frame         timeline.Frame
		width, height int
	}{
		{"empty", frameOf(), 40, 6},
		{"single", frameOf(5000), 40, 10},
		{"range", frameOf(120_000, 3_000, 400, 25_000), 80, 16},
		{"crowded", frameOf(900, 12_000, 50, 7_000, 60_000, 3, 800, 20_000), 60, 12},
		{"shrunk", timeline.Frame{Totals: []int64{800, 40}, Ceiling: 100_000}, 40, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(Draw(tt.frame, cols, tt.width, tt.height).Lines(), "\n") + "\n"
			path := filepath.Join("testdata", "golden", tt.name+".txt")
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
