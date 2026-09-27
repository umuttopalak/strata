package timeline

import (
	"fmt"
	"slices"
	"testing"

	"github.com/umuttopalak/strata/internal/gitlog"
)

type ev struct {
	kind   EventKind
	commit int
	name   string
	value  int64
}

func events(tl *Timeline) []ev {
	var out []ev
	for _, e := range tl.Events {
		out = append(out, ev{e.Kind, e.Commit, e.Name, e.Value})
	}
	return out
}

func by(author string, c gitlog.Commit) gitlog.Commit {
	c.Author = author
	return c
}

func tagged(c gitlog.Commit, tags ...string) gitlog.Commit {
	c.Tags = tags
	return c
}

// files returns n file changes, each adding a and deleting d lines.
func files(n, a, d int) []gitlog.FileChange {
	var out []gitlog.FileChange
	for i := range n {
		out = append(out, gitlog.FileChange{Path: fmt.Sprintf("pkg/f%d.go", i), Added: a, Deleted: d})
	}
	return out
}

func TestDetectEvents(t *testing.T) {
	tl := build(t, fakeSource{commits: []gitlog.Commit{
		commit("init", day(2020, 1, 1), add("src/a", 500)),
		tagged(commit("release", day(2020, 1, 2), add("src/a", 600)), "v1.0", "stable"), // passes 1k
		commit("vendor", day(2020, 1, 3), add("vendor/x", 200_000)),                     // passes 10k and 100k
		commit("unvendor", day(2020, 1, 4), del("vendor/x", 200_000)),                   // cleanup
		commit("small cleanup", day(2020, 1, 5), del("src/a", 100)),                     // too small
		commit("move", day(2020, 1, 6), files(60, 30, 30)...),                           // restructure
		commit("back", day(2020, 5, 1), add("src/a", 1)),                                // after ~4 months
	}}, Options{})

	want := []ev{
		{EventFirstCommit, 1, "", 0},
		{EventTag, 2, "stable · v1.0", 0}, // several tags on one commit: one event
		{EventMilestone, 2, "", 1_000},
		{EventMilestone, 3, "", 100_000}, // only the largest of the two
		{EventCleanup, 4, "", 200_000},
		{EventRestructure, 6, "", 60},
		{EventQuiet, 7, "", 116},
	}
	if got := events(tl); !slices.Equal(got, want) {
		t.Errorf("events:\n got %v\nwant %v", got, want)
	}
}

func TestDetectContributors(t *testing.T) {
	var commits []gitlog.Commit
	for i := range 40 {
		author := "Ada"
		switch {
		case i >= 10 && i%2 == 0:
			author = "Grace" // 15 commits from commit 11 on: major
		case i == 21:
			author = "Linus" // one drive-by commit: not announced
		}
		commits = append(commits, by(author, commit("c", day(2020, 1, 1+i%28), add("a/x", 1))))
	}
	tl := build(t, fakeSource{commits: commits}, Options{})
	if tl.Frames[22].Caption.Author != "Linus" {
		t.Fatal("fixture: commit 22 should be Linus's")
	}

	var joins []ev
	for _, e := range events(tl) {
		if e.kind == EventContributor {
			joins = append(joins, e)
		}
	}
	// Ada made the first commit, which is announced as such instead.
	if want := []ev{{EventContributor, 11, "Grace", 0}}; !slices.Equal(joins, want) {
		t.Errorf("joins = %v, want %v", joins, want)
	}
}

func TestDetectEventsFromBaseline(t *testing.T) {
	tl := build(t, fakeSource{
		baseline: []gitlog.FileChange{add("src/old", 5_000)},
		commits: []gitlog.Commit{
			commit("a", day(2020, 1, 1), add("src/a", 4_000)), // 9k: no milestone yet
			commit("b", day(2020, 1, 2), add("src/a", 2_000)), // passes 10k
		},
	}, Options{})
	// No "first commit" when replaying from a baseline, and the 1k
	// milestone was passed before the replay started.
	if want := []ev{{EventMilestone, 2, "", 10_000}}; !slices.Equal(events(tl), want) {
		t.Errorf("events = %v, want %v", events(tl), want)
	}
}

func TestEventFramesFollowGrouping(t *testing.T) {
	var commits []gitlog.Commit
	for i := range 25 {
		c := commit("c", day(2020, 1, 1+i), add("a/x", 1))
		if i == 7 {
			c = tagged(c, "v1")
		}
		commits = append(commits, c)
	}
	tl := build(t, fakeSource{commits: commits}, Options{MaxFrames: 10})
	// Groups of 3: commit 8 lands in frame 3 (commits 7–9).
	for _, e := range tl.Events {
		if e.Kind == EventTag && (e.Commit != 8 || e.Frame != 3) {
			t.Errorf("tag at commit %d frame %d, want commit 8 frame 3", e.Commit, e.Frame)
		}
		if f := tl.Frames[e.Frame]; f.Commit < e.Commit || f.Commit-e.Commit >= 3 {
			t.Errorf("%v: frame %d shows commit %d", e.Kind, e.Frame, f.Commit)
		}
	}
}
