package gitlog

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/umuttopalak/strata/internal/testutil"
)

// historyRepo builds this history. Walk follows main only, so the feature
// commit shows up folded into the merge:
//
//	2020-01-01  init       README.md 5, src/a.go 100, docs/guide.md 10
//	2020-02-01  shrink     src/a.go 100→80, src/b/c.go +30, img.png (binary)
//	2020-02-15  feature    lib/x.go +50, README.md 5→7          (on a branch)
//	2020-02-20  docs       docs/guide.md 10→12, "docs/ünicode file.md" +4, README.md 5→6
//	2020-03-01  merge      feature into main; README.md conflict resolved to 9 lines
//	2020-04-01  cleanup    docs/guide.md deleted
func historyRepo(t *testing.T) *Repo {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Write("README.md", testutil.Lines(5))
	r.Write("src/a.go", testutil.Lines(100))
	r.Write("docs/guide.md", testutil.Lines(10))
	r.Commit("init", testutil.Day(2020, 1, 1))

	r.Write("src/a.go", testutil.Lines(80))
	r.Write("src/b/c.go", testutil.Lines(30))
	r.Write("img.png", "\x89PNG\x00\x00\x00binary")
	r.Commit("shrink", testutil.Day(2020, 2, 1))

	r.Git("checkout", "-q", "-b", "feature")
	r.Write("lib/x.go", testutil.Lines(50))
	r.Write("README.md", testutil.Lines(7))
	r.Commit("feature", testutil.Day(2020, 2, 15))

	r.Git("checkout", "-q", "main")
	r.Write("docs/guide.md", testutil.Lines(12))
	r.Write("docs/ünicode file.md", testutil.Lines(4))
	r.Write("README.md", testutil.Lines(6))
	r.Commit("docs", testutil.Day(2020, 2, 20))

	r.Merge("feature", "merge", testutil.Day(2020, 3, 1), func() {
		r.Write("README.md", testutil.Lines(9))
	})

	r.Remove("docs/guide.md")
	r.Commit("cleanup", testutil.Day(2020, 4, 1))

	repo, err := OpenRepo(context.Background(), r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func walkAll(t *testing.T, repo *Repo, opts LogOptions) []Commit {
	t.Helper()
	var got []Commit
	if err := repo.Walk(context.Background(), opts, func(c Commit) error {
		got = append(got, c)
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return got
}

func subjects(cs []Commit) []string {
	var s []string
	for _, c := range cs {
		s = append(s, c.Subject)
	}
	return s
}

func files(c Commit) map[string][2]int {
	m := map[string][2]int{}
	for _, f := range c.Files {
		m[f.Path] = [2]int{f.Added, f.Deleted}
	}
	return m
}

func TestWalk(t *testing.T) {
	repo := historyRepo(t)
	got := walkAll(t, repo, LogOptions{})

	wantSubjects := []string{"init", "shrink", "docs", "merge", "cleanup"}
	if s := subjects(got); !slices.Equal(s, wantSubjects) {
		t.Fatalf("subjects = %v, want %v", s, wantSubjects)
	}

	wantFiles := []map[string][2]int{
		{"README.md": {5, 0}, "src/a.go": {100, 0}, "docs/guide.md": {10, 0}},
		{"src/a.go": {0, 20}, "src/b/c.go": {30, 0}}, // img.png is binary
		{"docs/guide.md": {2, 0}, "docs/ünicode file.md": {4, 0}, "README.md": {1, 0}},
		{"lib/x.go": {50, 0}, "README.md": {3, 0}}, // relative to first parent

		{"docs/guide.md": {0, 12}},
	}
	for i, c := range got {
		if f := files(c); !maps.Equal(f, wantFiles[i]) {
			t.Errorf("commit %q files = %v, want %v", c.Subject, f, wantFiles[i])
		}
	}

	first := got[0]
	if !first.Time.Equal(testutil.Day(2020, 1, 1)) || !first.AuthorTime.Equal(testutil.Day(2020, 1, 1)) {
		t.Errorf("init time = %v / %v", first.Time, first.AuthorTime)
	}
	if first.Author != "Test Author" || len(first.Hash) < 40 {
		t.Errorf("init author/hash = %q / %q", first.Author, first.Hash)
	}
}

// Summing every numstat in the walk must reproduce the final tree's sizes,
// including the lines added while resolving the merge conflict.
func TestWalkTotalsMatchFinalTree(t *testing.T) {
	repo := historyRepo(t)
	totals := map[string]int{}
	for _, c := range walkAll(t, repo, LogOptions{}) {
		for _, f := range c.Files {
			totals[f.Path] += f.Added - f.Deleted
			if totals[f.Path] == 0 {
				delete(totals, f.Path)
			}
		}
	}

	final, err := repo.Baseline(context.Background(), LogOptions{Since: testutil.Day(2100, 1, 1)})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{}
	for _, f := range final {
		want[f.Path] = f.Added
	}
	if len(totals) != len(want) {
		t.Fatalf("totals = %v, want %v", totals, want)
	}
	for k, v := range want {
		if totals[k] != v {
			t.Errorf("%s: walk total %d, tree has %d", k, totals[k], v)
		}
	}
}

func TestWalkSince(t *testing.T) {
	repo := historyRepo(t)
	opts := LogOptions{Since: testutil.Day(2020, 2, 10)}

	got := subjects(walkAll(t, repo, opts))
	if want := []string{"docs", "merge", "cleanup"}; !slices.Equal(got, want) {
		t.Fatalf("subjects = %v, want %v", got, want)
	}

	base, err := repo.Baseline(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	gotBase := map[string][2]int{}
	for _, f := range base {
		gotBase[f.Path] = [2]int{f.Added, f.Deleted}
	}
	wantBase := map[string][2]int{
		"README.md": {5, 0}, "src/a.go": {80, 0}, "src/b/c.go": {30, 0}, "docs/guide.md": {10, 0},
	}
	if !maps.Equal(gotBase, wantBase) {
		t.Errorf("baseline = %v, want %v", gotBase, wantBase)
	}
}

func TestBaselineBeforeHistory(t *testing.T) {
	repo := historyRepo(t)
	for _, since := range []time.Time{{}, testutil.Day(2019, 1, 1)} {
		base, err := repo.Baseline(context.Background(), LogOptions{Since: since})
		if err != nil || len(base) != 0 {
			t.Errorf("since %v: baseline = %v, err = %v", since, base, err)
		}
	}
}

func TestBounds(t *testing.T) {
	repo := historyRepo(t)
	ctx := context.Background()

	b, err := repo.Bounds(ctx, LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !b.First.Equal(testutil.Day(2020, 1, 1)) || !b.Last.Equal(testutil.Day(2020, 4, 1)) {
		t.Errorf("bounds = %v .. %v", b.First, b.Last)
	}

	b, err = repo.Bounds(ctx, LogOptions{Since: testutil.Day(2020, 2, 10)})
	if err != nil || !b.First.Equal(testutil.Day(2020, 2, 10)) {
		t.Errorf("since bounds = %v, err = %v", b.First, err)
	}

	if _, err := repo.Bounds(ctx, LogOptions{Since: testutil.Day(2021, 1, 1)}); !errors.Is(err, ErrNoCommits) {
		t.Errorf("err = %v, want ErrNoCommits", err)
	}
}

func TestWalkStops(t *testing.T) {
	repo := historyRepo(t)

	t.Run("callback error", func(t *testing.T) {
		stop := errors.New("stop")
		calls := 0
		err := repo.Walk(context.Background(), LogOptions{}, func(Commit) error {
			calls++
			return stop
		})
		if !errors.Is(err, stop) || calls != 1 {
			t.Fatalf("err = %v, calls = %d", err, calls)
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		err := repo.Walk(ctx, LogOptions{}, func(Commit) error {
			calls++
			cancel()
			return nil
		})
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("err = %v, calls = %d", err, calls)
		}
	})
}
