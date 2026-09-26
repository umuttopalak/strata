package timeline

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/umuttopalak/summit/internal/gitlog"
	"github.com/umuttopalak/summit/internal/testutil"
)

var day = testutil.Day

func commit(subject string, at time.Time, files ...gitlog.FileChange) gitlog.Commit {
	return gitlog.Commit{Hash: subject + "-hash", Subject: subject, Author: "a",
		Time: at, AuthorTime: at, Files: files}
}

func add(path string, n int) gitlog.FileChange { return gitlog.FileChange{Path: path, Added: n} }
func del(path string, n int) gitlog.FileChange { return gitlog.FileChange{Path: path, Deleted: n} }

// totals returns a frame's non-zero totals by column name.
func totals(tl *Timeline, f Frame) map[string]int64 {
	m := map[string]int64{}
	for i, name := range tl.Columns() {
		if v := f.Total(i); v != 0 {
			m[name] = v
		}
	}
	return m
}

// Four 30-day slices: Jan 1–31, Jan 31–Mar 1, Mar 1–31, Mar 31–Apr 30.
func fourSlices() (gitlog.Bounds, Options) {
	return gitlog.Bounds{First: day(2020, 1, 1), Last: day(2020, 4, 30)}, Options{Depth: 1, Frames: 4}
}

func TestBuilderSlices(t *testing.T) {
	bounds, opts := fourSlices()
	tl := &Timeline{}
	b := NewBuilder(bounds, opts, tl)

	b.Add(commit("init", day(2020, 1, 1), add("src/a.go", 100), add("README.md", 5)))
	b.Add(commit("more", day(2020, 1, 20), add("src/b.go", 50)))
	// Feb is quiet except for this one; Mar is fully quiet.
	b.Add(commit("docs", day(2020, 2, 10), add("docs/x.md", 10), del("src/a.go", 30)))
	b.Add(commit("last", day(2020, 4, 30), del("docs/x.md", 10)))
	b.Finish()

	if done, _ := tl.Done(); done {
		t.Fatal("builder must not mark the timeline done; Build does")
	}
	if tl.Len() != 4 || tl.Expected() != 4 {
		t.Fatalf("Len = %d, Expected = %d", tl.Len(), tl.Expected())
	}
	if got, want := tl.Columns(), []string{"src", RootKey, "docs"}; !slices.Equal(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}

	want := []struct {
		totals  map[string]int64
		commits int
		caption string
		max     int64
	}{
		{map[string]int64{"src": 150, RootKey: 5}, 2, "more", 150},
		{map[string]int64{"src": 120, RootKey: 5, "docs": 10}, 1, "docs", 150},
		{map[string]int64{"src": 120, RootKey: 5, "docs": 10}, 0, "", 150},
		{map[string]int64{"src": 120, RootKey: 5}, 1, "last", 150},
	}
	for i, w := range want {
		f := tl.At(i)
		if f.Index != i {
			t.Errorf("frame %d: Index = %d", i, f.Index)
		}
		if got := totals(tl, f); !maps.Equal(got, w.totals) {
			t.Errorf("frame %d: totals = %v, want %v", i, got, w.totals)
		}
		if f.Commits != w.commits || f.Max != w.max {
			t.Errorf("frame %d: commits = %d, max = %d", i, f.Commits, f.Max)
		}
		caption := ""
		if f.Caption != nil {
			caption = f.Caption.Subject
		}
		if caption != w.caption {
			t.Errorf("frame %d: caption = %q, want %q", i, caption, w.caption)
		}
	}

	if !tl.At(0).Time.Equal(day(2020, 1, 31)) || !tl.At(3).Time.Equal(day(2020, 4, 30)) {
		t.Errorf("frame times = %v .. %v", tl.At(0).Time, tl.At(3).Time)
	}
	// docs was last touched by "last" even though it is now empty.
	if got := tl.At(3).Touched[2]; got != day(2020, 4, 30).Unix() {
		t.Errorf("docs touched = %v", time.Unix(got, 0))
	}
}

func TestBuilderPublishedFramesAreImmutable(t *testing.T) {
	bounds, opts := fourSlices()
	tl := &Timeline{}
	b := NewBuilder(bounds, opts, tl)
	b.Add(commit("init", day(2020, 1, 1), add("src/a.go", 100)))
	b.Add(commit("late", day(2020, 4, 1), add("src/a.go", 1), add("new/x.go", 1)))
	b.Finish()

	first := tl.At(0)
	if first.Totals[0] != 100 || len(first.Totals) != 1 {
		t.Fatalf("first frame changed after later commits: %v", first.Totals)
	}
}

func TestBuilderOutOfOrderDates(t *testing.T) {
	bounds, opts := fourSlices()
	tl := &Timeline{}
	b := NewBuilder(bounds, opts, tl)
	b.Add(commit("march", day(2020, 3, 15), add("a/x", 1)))
	b.Add(commit("skewed", day(2020, 1, 5), add("a/x", 1))) // clock skew
	b.Finish()

	if f := tl.At(2); f.Commits != 2 || f.Caption.Subject != "skewed" {
		t.Fatalf("skewed commit should join the current slice, got %d commits", f.Commits)
	}
	if tl.Len() != 4 {
		t.Fatalf("Len = %d", tl.Len())
	}
}

func TestBuilderClampsNegative(t *testing.T) {
	bounds, opts := fourSlices()
	tl := &Timeline{}
	b := NewBuilder(bounds, opts, tl)
	b.Add(commit("drift", day(2020, 1, 1), del("a/x", 10)))
	b.Add(commit("regrow", day(2020, 4, 1), add("a/x", 15)))
	b.Finish()

	if v := tl.At(0).Total(0); v != 0 {
		t.Errorf("negative total published as %d", v)
	}
	// The accumulator stays exact: -10 + 15 = 5.
	if v := tl.At(3).Total(0); v != 5 {
		t.Errorf("final total = %d, want 5", v)
	}
}

func TestBuilderSeed(t *testing.T) {
	bounds, opts := fourSlices()
	tl := &Timeline{}
	b := NewBuilder(bounds, opts, tl)
	b.Seed([]gitlog.FileChange{add("src/old.go", 40), add("top.txt", 2)})
	b.Add(commit("change", day(2020, 2, 1), add("src/new.go", 10)))
	b.Finish()

	if got, want := totals(tl, tl.At(0)), map[string]int64{"src": 40, RootKey: 2}; !maps.Equal(got, want) {
		t.Errorf("first frame = %v, want %v", got, want)
	}
	if got := tl.At(0).Touched[0]; got != 0 {
		t.Errorf("seeded column touched = %d, want 0 (unknown)", got)
	}
	if got := tl.At(3).Total(0); got != 50 {
		t.Errorf("final src = %d, want 50", got)
	}
}

func TestBuilderSingleInstant(t *testing.T) {
	at := day(2020, 1, 1)
	tl := &Timeline{}
	b := NewBuilder(gitlog.Bounds{First: at, Last: at}, Options{}, tl)
	b.Add(commit("only", at, add("x/y", 3)))
	b.Finish()

	if tl.Len() != 1 || tl.At(0).Total(0) != 3 || tl.At(0).Caption.Subject != "only" {
		t.Fatalf("Len = %d, frame = %+v", tl.Len(), tl.At(0))
	}
}

func TestBuilderDefaults(t *testing.T) {
	tl := &Timeline{}
	NewBuilder(gitlog.Bounds{First: day(2000, 1, 1), Last: day(2020, 1, 1)}, Options{}, tl).Finish()
	if tl.Len() != DefaultFrames {
		t.Fatalf("Len = %d, want %d", tl.Len(), DefaultFrames)
	}
}

func TestBuilderCeilingEasesDown(t *testing.T) {
	tl := &Timeline{}
	b := NewBuilder(gitlog.Bounds{First: day(2020, 1, 1), Last: day(2020, 1, 11)}, Options{Frames: 10}, tl)
	b.Add(commit("big", day(2020, 1, 1), add("vendor/x", 1000), add("src/y", 100)))
	b.Add(commit("drop", day(2020, 1, 2), del("vendor/x", 1000)))
	b.Finish()

	if c := tl.At(0).Ceiling; c != 1000 {
		t.Fatalf("frame 0 ceiling = %d, want 1000", c)
	}
	prev := int64(1000)
	for i := 1; i < tl.Len(); i++ {
		f := tl.At(i)
		if f.Ceiling >= prev || f.Ceiling < 100 {
			t.Fatalf("frame %d ceiling = %d, want below %d and at least 100", i, f.Ceiling, prev)
		}
		if f.Max != 1000 {
			t.Fatalf("frame %d max = %d, want 1000", i, f.Max)
		}
		prev = f.Ceiling
	}
}
