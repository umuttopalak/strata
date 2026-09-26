package timeline

import "testing"

func TestKeyFor(t *testing.T) {
	tests := []struct {
		path  string
		depth int
		want  string
	}{
		{"README.md", 1, RootKey},
		{"README.md", 3, RootKey},
		{"/abs.txt", 1, RootKey},
		{"src/main.go", 1, "src"},
		{"src/main.go", 2, "src"},
		{"src/net/http/server.go", 1, "src"},
		{"src/net/http/server.go", 2, "src/net"},
		{"src/net/http/server.go", 3, "src/net/http"},
		{"src/net/http/server.go", 9, "src/net/http"},
		{"docs/ünicode file.md", 1, "docs"},
	}
	for _, tt := range tests {
		if got := KeyFor(tt.path, tt.depth); got != tt.want {
			t.Errorf("KeyFor(%q, %d) = %q, want %q", tt.path, tt.depth, got, tt.want)
		}
	}
}

func TestFrameTotalOutOfRange(t *testing.T) {
	f := Frame{Totals: []int64{5}}
	if f.Total(0) != 5 || f.Total(3) != 0 {
		t.Fatalf("Total = %d, %d", f.Total(0), f.Total(3))
	}
}
