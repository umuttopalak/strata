package player

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestParseKeys(t *testing.T) {
	tests := []struct {
		in   string
		want []key
	}{
		{" ", []key{keyPause}},
		{"+-=_", []key{keyFaster, keySlower, keyFaster, keySlower}},
		{"\x1b[D\x1b[C", []key{keyBack, keyForward}},
		{"\x1bOD\x1bOC", []key{keyBack, keyForward}},
		{"hl", []key{keyBack, keyForward}},
		{"q", []key{keyQuit}},
		{"\x03", []key{keyInterrupt}},
		{"\x1b[A\x1b[Bx\x1b", nil}, // up/down, unknown and a lone ESC are ignored
	}
	for _, tt := range tests {
		if got := parseKeys([]byte(tt.in)); !slices.Equal(got, tt.want) {
			t.Errorf("parseKeys(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNextSpeed(t *testing.T) {
	tests := []struct {
		cur  float64
		dir  int
		want float64
	}{
		{1, 1, 2},
		{1, -1, 0.5},
		{16, 1, 16},
		{0.25, -1, 0.25},
		{3, 1, 4}, // a --speed between steps snaps to the next one
		{3, -1, 2},
	}
	for _, tt := range tests {
		if got := nextSpeed(tt.cur, tt.dir); got != tt.want {
			t.Errorf("nextSpeed(%v, %d) = %v, want %v", tt.cur, tt.dir, got, tt.want)
		}
	}
}

func TestStateHandle(t *testing.T) {
	s := state{pos: 5, last: 20, speed: 1}

	s.handle(keyBack)
	if s.pos != 0 {
		t.Errorf("seek back clamps at 0, got %v", s.pos)
	}
	s.handle(keyForward)
	s.handle(keyForward)
	s.handle(keyForward)
	if s.pos != 20 {
		t.Errorf("seek forward clamps at the last frame, got %v", s.pos)
	}

	s.handle(keyPause)
	s.handle(keyFaster)
	if !s.paused || s.speed != 2 || s.status() != "paused 2×" {
		t.Errorf("state = %+v, status %q", s, s.status())
	}
	s.handle(keyPause)
	s.handle(keySlower)
	if s.status() != "" {
		t.Errorf("status = %q, want empty at normal speed", s.status())
	}

	if err := s.handle(keyQuit); !errors.Is(err, errQuit) {
		t.Errorf("q = %v", err)
	}
	if err := s.handle(keyInterrupt); !errors.Is(err, context.Canceled) {
		t.Errorf("Ctrl+C = %v", err)
	}
}
