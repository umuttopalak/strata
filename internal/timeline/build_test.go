package timeline

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/umuttopalak/summit/internal/gitlog"
	"github.com/umuttopalak/summit/internal/testutil"
)

type fakeSource struct {
	bounds  gitlog.Bounds
	commits []gitlog.Commit
	walkErr error
}

func (f fakeSource) Bounds(context.Context, gitlog.LogOptions) (gitlog.Bounds, error) {
	return f.bounds, nil
}

func (f fakeSource) Baseline(context.Context, gitlog.LogOptions) ([]gitlog.FileChange, error) {
	return nil, nil
}

func (f fakeSource) Walk(_ context.Context, _ gitlog.LogOptions, fn func(gitlog.Commit) error) error {
	for _, c := range f.commits {
		if err := fn(c); err != nil {
			return err
		}
	}
	return f.walkErr
}

func TestBuildMarksDone(t *testing.T) {
	bounds, opts := fourSlices()
	src := fakeSource{bounds: bounds, commits: []gitlog.Commit{
		commit("init", day(2020, 1, 1), add("a/x", 1)),
	}}
	tl := &Timeline{}
	if err := Build(context.Background(), src, gitlog.LogOptions{}, opts, tl); err != nil {
		t.Fatal(err)
	}
	if done, err := tl.Done(); !done || err != nil {
		t.Fatalf("Done = %v, %v", done, err)
	}
	if tl.Len() != 4 {
		t.Fatalf("Len = %d", tl.Len())
	}
}

func TestBuildPropagatesErrors(t *testing.T) {
	bounds, opts := fourSlices()
	boom := errors.New("boom")
	src := fakeSource{bounds: bounds, walkErr: boom,
		commits: []gitlog.Commit{commit("init", day(2020, 1, 1), add("a/x", 1))}}
	tl := &Timeline{}
	if err := Build(context.Background(), src, gitlog.LogOptions{}, opts, tl); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if done, err := tl.Done(); !done || !errors.Is(err, boom) {
		t.Fatalf("Done = %v, %v", done, err)
	}
}

// End to end against a real repository, including --since with a baseline.
func TestBuildRealRepo(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("README.md", testutil.Lines(5))
	r.Write("src/core/a.go", testutil.Lines(100))
	r.Commit("init", day(2020, 1, 1))
	r.Write("src/net/b.go", testutil.Lines(40))
	r.Commit("net", day(2020, 6, 1))
	r.Remove("src/core/a.go")
	r.Write("docs/guide.md", testutil.Lines(7))
	r.Commit("rewrite", day(2021, 1, 1))

	repo, err := gitlog.OpenRepo(context.Background(), r.Dir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		since bool
		depth int
		first map[string]int64
		last  map[string]int64
	}{
		{"whole history", false, 1,
			map[string]int64{"src": 100, RootKey: 5},
			map[string]int64{"src": 40, RootKey: 5, "docs": 7}},
		{"depth 2", false, 2,
			map[string]int64{"src/core": 100, RootKey: 5},
			map[string]int64{"src/net": 40, RootKey: 5, "docs": 7}},
		{"since with baseline", true, 1,
			map[string]int64{"src": 140, RootKey: 5},
			map[string]int64{"src": 40, RootKey: 5, "docs": 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var log gitlog.LogOptions
			if tt.since {
				log.Since = day(2020, 7, 1)
			}
			tl := &Timeline{}
			err := Build(context.Background(), repo, log, Options{Depth: tt.depth, Frames: 10}, tl)
			if err != nil {
				t.Fatal(err)
			}
			if tl.Len() != 10 {
				t.Fatalf("Len = %d", tl.Len())
			}
			if got := totals(tl, tl.At(0)); !maps.Equal(got, tt.first) {
				t.Errorf("first = %v, want %v", got, tt.first)
			}
			if got := totals(tl, tl.At(9)); !maps.Equal(got, tt.last) {
				t.Errorf("last = %v, want %v", got, tt.last)
			}
		})
	}
}

// A reader polling the timeline while Build runs must see consistent
// frames; run with -race to check the locking.
func TestBuildConcurrentReaders(t *testing.T) {
	src := fakeSource{bounds: gitlog.Bounds{First: day(2000, 1, 1), Last: day(2020, 1, 1)}}
	for i := range 5000 {
		at := day(2000, 1, 1).Add(time.Duration(i) * 35 * time.Hour)
		src.commits = append(src.commits, commit("c", at, add(fmt.Sprintf("d%d/f", i%50), 1)))
	}
	tl := &Timeline{}
	go func() { _ = Build(context.Background(), src, gitlog.LogOptions{}, Options{}, tl) }()

	for {
		done, err := tl.Done()
		if err != nil {
			t.Fatal(err)
		}
		if n := tl.Len(); n > 0 {
			f := tl.At(n - 1)
			cols := tl.Columns()
			if len(f.Totals) > len(cols) {
				t.Fatalf("frame %d has %d totals but only %d columns", n-1, len(f.Totals), len(cols))
			}
		}
		if done {
			break
		}
	}
	if tl.Len() != DefaultFrames {
		t.Fatalf("Len = %d", tl.Len())
	}
	var sum int64
	for _, v := range tl.At(DefaultFrames - 1).Totals {
		sum += v
	}
	if sum != 5000 {
		t.Fatalf("final lines = %d, want 5000", sum)
	}
}
