package timeline

import (
	"slices"
	"strings"
	"time"
)

// EventKind is a type of notable moment in a history.
type EventKind int

const (
	EventFirstCommit EventKind = iota
	EventTag                   // Name is the tag, or several joined by " · "
	EventCleanup               // Value is the net number of lines removed
	EventRestructure           // Value is the number of files changed
	EventQuiet                 // Value is the length of the silence in days
	EventMilestone             // Value is the line count passed
	EventContributor           // Name is the author
)

// Priority orders events that land in the same frame; higher wins.
func (k EventKind) Priority() int {
	switch k {
	case EventTag:
		return 6
	case EventFirstCommit, EventCleanup, EventRestructure:
		return 5
	case EventQuiet:
		return 4
	case EventMilestone:
		return 3
	}
	return 2
}

// Event is a notable moment, attached to the commit where it happened.
type Event struct {
	Kind   EventKind
	Commit int // 1-based commit number
	Frame  int // frame in which that commit lands
	Name   string
	Value  int64
}

// Thresholds for detection. They aim at moments a viewer would want
// pointed out, not at every sizeable commit.
const (
	quietGap           = 60 * 24 * time.Hour
	cleanupMinLines    = 1000 // net lines removed…
	cleanupShare       = 5    // …and at least 1/cleanupShare of the code
	restructureFiles   = 50   // files changed…
	restructureMin     = 1000 // …with at least this many lines both added and removed
	contributorShare   = 20   // a major contributor made 1/contributorShare of the commits…
	contributorCommits = 5    // …and at least this many
)

var milestones = []int64{1_000, 10_000, 100_000, 1_000_000, 10_000_000}

// detectEvents walks the commits in order. seedLines are the lines that
// existed before the first one (--since); a replay that starts from a
// baseline has no "first commit". per is how many commits a frame holds.
func detectEvents(records []record, seedLines int64, fromBaseline bool, per int) []Event {
	counts := map[string]int{}
	for _, r := range records {
		counts[r.caption.Author]++
	}
	major := max(contributorCommits, len(records)/contributorShare)
	seen := map[string]bool{}

	var events []Event
	lines := seedLines
	nextMilestone := 0
	for nextMilestone < len(milestones) && lines >= milestones[nextMilestone] {
		nextMilestone++ // already passed before the replay starts
	}

	for i, r := range records {
		c := r.caption
		commit := i + 1
		add := func(kind EventKind, name string, value int64) {
			events = append(events, Event{Kind: kind, Commit: commit,
				Frame: (commit + per - 1) / per, Name: name, Value: value})
		}

		if i == 0 && !fromBaseline {
			add(EventFirstCommit, "", 0)
		}
		if len(r.tags) > 0 {
			tags := slices.Clone(r.tags)
			slices.Sort(tags)
			add(EventTag, strings.Join(tags, " · "), 0)
		}
		if i > 0 {
			if gap := r.at.Sub(records[i-1].at); gap >= quietGap {
				add(EventQuiet, "", int64(gap/(24*time.Hour)))
			}
		}

		before := lines
		added, deleted := int64(c.Added), int64(c.Deleted)
		lines += added - deleted
		switch {
		case c.Files >= restructureFiles && min(added, deleted) >= restructureMin &&
			2*min(added, deleted) >= max(added, deleted):
			add(EventRestructure, "", int64(c.Files))
		case deleted-added >= cleanupMinLines && (deleted-added)*cleanupShare >= before:
			add(EventCleanup, "", deleted-added)
		}

		// One commit may pass several milestones; announce the largest.
		passed := int64(0)
		for nextMilestone < len(milestones) && lines >= milestones[nextMilestone] {
			passed = milestones[nextMilestone]
			nextMilestone++
		}
		if passed > 0 {
			add(EventMilestone, "", passed)
		}

		if !seen[c.Author] {
			seen[c.Author] = true
			if i > 0 && counts[c.Author] >= major {
				add(EventContributor, c.Author, 0)
			}
		}
	}
	return events
}
