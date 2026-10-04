package timeline

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/umuttopalak/strata/internal/gitlog"
	"github.com/umuttopalak/strata/internal/testutil"
)

var day = testutil.Day

type fakeSource struct {
	baseline []gitlog.FileChange
	commits  []gitlog.Commit
	walkErr  error
}

func (f fakeSource) Bounds(context.Context, gitlog.LogOptions) (gitlog.Bounds, error) {
	return gitlog.Bounds{}, nil
}

func (f fakeSource) Baseline(context.Context, gitlog.LogOptions) ([]gitlog.FileChange, error) {
	return f.baseline, nil
}

func (f fakeSource) Walk(_ context.Context, _ gitlog.LogOptions, fn func(gitlog.Commit) error) error {
	for _, c := range f.commits {
		if err := fn(c); err != nil {
			return err
		}
	}
	return f.walkErr
}

func commit(subject string, at time.Time, files ...gitlog.FileChange) gitlog.Commit {
	return gitlog.Commit{Hash: subject + "-hash", Subject: subject, Author: "a",
		Time: at, AuthorTime: at, Files: files}
}

func add(path string, n int) gitlog.FileChange { return gitlog.FileChange{Path: path, Added: n} }
func del(path string, n int) gitlog.FileChange { return gitlog.FileChange{Path: path, Deleted: n} }

// totals returns a frame's non-zero totals by column name.
func totals(tl *Timeline, f Frame) map[string]int64 {
	m := map[string]int64{}
	for i, name := range tl.Columns {
		if v := f.Total(i); v != 0 {
			m[name] = v
		}
	}
	return m
}

func build(t *testing.T, src fakeSource, opts Options) *Timeline {
	t.Helper()
	tl, err := Build(context.Background(), src, gitlog.LogOptions{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

func TestBuildOneFramePerCommit(t *testing.T) {
	tl := build(t, fakeSource{commits: []gitlog.Commit{
		commit("init", day(2020, 1, 1), add("src/a.go", 100), add("README.md", 5)),
		commit("docs", day(2020, 2, 1), add("docs/x.md", 10), del("src/a.go", 30), add("src/b.go", 4)),
		commit("drop", day(2020, 3, 1), del("docs/x.md", 10)),
	}}, Options{})

	if tl.Commits != 3 || len(tl.Frames) != 4 {
		t.Fatalf("commits = %d, frames = %d", tl.Commits, len(tl.Frames))
	}
	want := []map[string]int64{
		{},
		{"src": 100, RootKey: 5},
		{"src": 74, RootKey: 5, "docs": 10},
		{"src": 74, RootKey: 5},
	}
	for i, w := range want {
		f := tl.Frames[i]
		if f.Commit != i {
			t.Errorf("frame %d: Commit = %d", i, f.Commit)
		}
		if got := totals(tl, f); !maps.Equal(got, w) {
			t.Errorf("frame %d: totals = %v, want %v", i, got, w)
		}
	}
	if tl.Frames[0].Caption != nil {
		t.Error("frame 0 must have no caption")
	}
	c := tl.Frames[2].Caption
	if c.Subject != "docs" || c.Added != 14 || c.Deleted != 30 || !c.Time.Equal(day(2020, 2, 1)) {
		t.Errorf("caption = %+v", c)
	}

	// docs peaked at 10 before being deleted; src peaked at 100.
	peaks := map[string]int64{}
	for i, name := range tl.Columns {
		peaks[name] = tl.Peak[i]
	}
	if want := map[string]int64{"src": 100, RootKey: 5, "docs": 10}; !maps.Equal(peaks, want) {
		t.Errorf("peaks = %v, want %v", peaks, want)
	}

	// Touched holds 1-based commit numbers.
	last := tl.Frames[3]
	if got := []int{last.LastTouched(0), last.LastTouched(1), last.LastTouched(2)}; got[0] != 2 || got[1] != 1 || got[2] != 3 {
		t.Errorf("touched = %v, want [2 1 3]", got)
	}
}

func TestBuildGroupsLongHistories(t *testing.T) {
	var commits []gitlog.Commit
	for i := range 25 {
		commits = append(commits, commit("c", day(2020, 1, 1+i), add("a/x", 1)))
	}
	tl := build(t, fakeSource{commits: commits}, Options{MaxFrames: 10})

	// 25 commits in groups of 3: frames after 3, 6, …, 24 and the tail at 25,
	// plus the initial empty frame.
	if len(tl.Frames) != 10 {
		t.Fatalf("frames = %d, want 10", len(tl.Frames))
	}
	for i, f := range tl.Frames[1:] {
		want := min((i+1)*3, 25)
		if f.Commit != want || f.Total(0) != int64(want) {
			t.Errorf("frame %d: commit %d total %d, want %d", i+1, f.Commit, f.Total(0), want)
		}
	}
}

func TestBuildClampsNegative(t *testing.T) {
	tl := build(t, fakeSource{commits: []gitlog.Commit{
		commit("drift", day(2020, 1, 1), del("a/x", 10)),
		commit("regrow", day(2020, 2, 1), add("a/x", 15)),
	}}, Options{})
	if v := tl.Frames[1].Total(0); v != 0 {
		t.Errorf("negative total published as %d", v)
	}
	// The running total stays exact: -10 + 15 = 5.
	if v := tl.Frames[2].Total(0); v != 5 {
		t.Errorf("final total = %d, want 5", v)
	}
}

func TestBuildSeedsBaseline(t *testing.T) {
	tl := build(t, fakeSource{
		baseline: []gitlog.FileChange{add("src/old.go", 40), add("top.txt", 2)},
		commits:  []gitlog.Commit{commit("change", day(2020, 2, 1), add("src/new.go", 10))},
	}, Options{})

	if got, want := totals(tl, tl.Frames[0]), map[string]int64{"src": 40, RootKey: 2}; !maps.Equal(got, want) {
		t.Errorf("frame 0 = %v, want %v", got, want)
	}
	if tl.Frames[1].LastTouched(1) != 0 {
		t.Error("baseline-only column should count as never touched")
	}
}

func TestBuildExcludesCommitsAndBaseline(t *testing.T) {
	tl := build(t, fakeSource{
		baseline: []gitlog.FileChange{add("vendor/old.go", 100), add("src/old.go", 40)},
		commits: []gitlog.Commit{
			commit("changes", day(2020, 2, 1), add("vendor/new.go", 50), add("src/new.go", 10), add("foo_test.go", 20)),
		},
	}, Options{Exclude: []string{"vendor", "test"}})

	if got, want := totals(tl, tl.Frames[0]), map[string]int64{"src": 40}; !maps.Equal(got, want) {
		t.Errorf("baseline = %v, want %v", got, want)
	}
	if got, want := totals(tl, tl.Frames[1]), map[string]int64{"src": 50}; !maps.Equal(got, want) {
		t.Errorf("final = %v, want %v", got, want)
	}
	if c := tl.Frames[1].Caption; c.Files != 1 || c.Added != 10 || c.Deleted != 0 {
		t.Errorf("filtered caption = %+v, want one included file and +10", c)
	}
}

func TestExcludedGlobPatterns(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     bool
	}{
		{"nested vendor preset", []string{"vendor"}, true},
		{"generated preset", []string{"generated"}, true},
		{"test suffix preset", []string{"test"}, true},
		{"directory glob", []string{"docs/**"}, true},
		{"filename glob", []string{"*.lock"}, true},
		{"kept source", []string{"vendor"}, false},
	}
	paths := []string{"pkg/vendor/x.go", "src/generated/code.go", "internal/foo_test.go", "docs/guide.md", "yarn.lock", "src/main.go"}
	for i, path := range paths {
		t.Run(path, func(t *testing.T) {
			if got := excluded(path, tests[i].patterns); got != tests[i].want {
				t.Errorf("excluded(%q, %v) = %v, want %v", path, tests[i].patterns, got, tests[i].want)
			}
		})
	}
}

func TestBuildErrors(t *testing.T) {
	boom := errors.New("boom")
	_, err := Build(context.Background(), fakeSource{walkErr: boom}, gitlog.LogOptions{}, Options{})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
	_, err = Build(context.Background(), fakeSource{}, gitlog.LogOptions{}, Options{})
	if !errors.Is(err, gitlog.ErrNoCommits) {
		t.Errorf("err = %v, want ErrNoCommits", err)
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
		name   string
		since  bool
		depth  int
		frames int
		first  map[string]int64
		last   map[string]int64
	}{
		{"whole history", false, 1, 4,
			map[string]int64{},
			map[string]int64{"src": 40, RootKey: 5, "docs": 7}},
		{"depth 2", false, 2, 4,
			map[string]int64{},
			map[string]int64{"src/net": 40, RootKey: 5, "docs": 7}},
		{"since with baseline", true, 1, 2,
			map[string]int64{"src": 140, RootKey: 5},
			map[string]int64{"src": 40, RootKey: 5, "docs": 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var log gitlog.LogOptions
			if tt.since {
				log.Since = day(2020, 7, 1)
			}
			tl, err := Build(context.Background(), repo, log, Options{Depth: tt.depth})
			if err != nil {
				t.Fatal(err)
			}
			if len(tl.Frames) != tt.frames {
				t.Fatalf("frames = %d, want %d", len(tl.Frames), tt.frames)
			}
			if got := totals(tl, tl.Frames[0]); !maps.Equal(got, tt.first) {
				t.Errorf("first = %v, want %v", got, tt.first)
			}
			if got := totals(tl, tl.Frames[len(tl.Frames)-1]); !maps.Equal(got, tt.last) {
				t.Errorf("last = %v, want %v", got, tt.last)
			}
		})
	}
}

func columnNames(tl *Timeline) map[string]bool {
	m := map[string]bool{}
	for i, name := range tl.Columns {
		if tl.Frames[len(tl.Frames)-1].Total(i) > 0 {
			m[name] = true
		}
	}
	return m
}

func TestBuildAutoDepth(t *testing.T) {
	tests := []struct {
		name  string
		files []gitlog.FileChange
		depth int
		want  []string
	}{
		{"enough top-level folders", []gitlog.FileChange{
			add("a/x/1", 1), add("b/2", 1), add("c/3", 1), add("d/4", 1), add("top.txt", 1),
		}, 1, []string{"a", "b", "c", RootKey, "d"}},
		{"flat repository splits root files", []gitlog.FileChange{
			add("main.py", 10), add("game.py", 20), add("README.md", 3), add("util.py", 4),
		}, 2, []string{"main.py", "game.py", "README.md", "util.py"}},
		{"single src folder goes deeper", []gitlog.FileChange{
			add("src/core/a", 1), add("src/net/b", 1), add("src/ui/c", 1), add("src/db/d", 1), add("go.mod", 1),
		}, 2, []string{"src/core", "src/net", "src/ui", "src/db", "go.mod"}},
		{"java layout needs depth four", []gitlog.FileChange{
			add("src/main/java/app/A.java", 1), add("src/main/java/db/B.java", 1),
			add("src/main/java/web/C.java", 1), add("src/test/java/app/T.java", 1),
		}, 4, []string{"src/main/java/app", "src/main/java/db", "src/main/java/web", "src/test/java/app"}},
		{"too few anywhere keeps the richest level", []gitlog.FileChange{
			add("only/one/file", 5),
		}, 1, []string{"only"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := build(t, fakeSource{commits: []gitlog.Commit{
				commit("all", day(2020, 1, 1), tt.files...),
			}}, Options{})
			if tl.Depth != tt.depth {
				t.Errorf("depth = %d, want %d", tl.Depth, tt.depth)
			}
			want := map[string]bool{}
			for _, n := range tt.want {
				want[n] = true
			}
			if got := columnNames(tl); !maps.Equal(got, want) {
				t.Errorf("columns = %v, want %v", got, want)
			}
		})
	}
}

func TestBuildExplicitDepthIsKept(t *testing.T) {
	tl := build(t, fakeSource{commits: []gitlog.Commit{
		commit("flat", day(2020, 1, 1), add("a.py", 1), add("b.py", 1)),
	}}, Options{Depth: 1})
	if tl.Depth != 1 || len(tl.Columns) != 1 || tl.Columns[0] != RootKey {
		t.Fatalf("depth %d columns %v", tl.Depth, tl.Columns)
	}
}
