package render

import (
	"fmt"

	"github.com/umuttopalak/strata/internal/timeline"
)

// EventText is how an event reads in the header.
func EventText(e timeline.Event) string {
	switch e.Kind {
	case timeline.EventFirstCommit:
		return "▲ first commit"
	case timeline.EventTag:
		return "◆ " + e.Name
	case timeline.EventCleanup:
		return "▼ big cleanup · −" + compact(e.Value) + " lines"
	case timeline.EventRestructure:
		return fmt.Sprintf("⇄ restructure · %d files", e.Value)
	case timeline.EventQuiet:
		return "… quiet for " + span(e.Value)
	case timeline.EventMilestone:
		return "★ " + compact(e.Value) + " lines"
	case timeline.EventContributor:
		return "+ " + e.Name + " joins"
	}
	return ""
}

// eventHold is how many frames an event stays in the header: about two
// seconds of a full-length replay, and at least a few frames in short ones.
func eventHold(frames int) int {
	return min(max(frames/12, 3), 25)
}

// activeEvent returns the event to show at frame i: the most recent one
// still within its hold, the highest priority among several in one frame.
func activeEvent(tl *timeline.Timeline, i int) (timeline.Event, bool) {
	hold := eventHold(len(tl.Frames))
	var best timeline.Event
	found := false
	for _, e := range tl.Events {
		if e.Frame > i {
			break // events are in commit order, hence frame order
		}
		if i-e.Frame >= hold {
			continue
		}
		if !found || e.Frame > best.Frame ||
			(e.Frame == best.Frame && e.Kind.Priority() > best.Kind.Priority()) {
			best, found = e, true
		}
	}
	return best, found
}

// compact writes 1234 as 1.2k and 2500000 as 2.5M.
func compact(n int64) string {
	switch {
	case n >= 1_000_000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1e6)) + "M"
	case n >= 1_000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1e3)) + "k"
	}
	return fmt.Sprint(n)
}

func trimZero(s string) string {
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		return s[:len(s)-2]
	}
	return s
}

// span writes a number of days as weeks, months or years.
func span(days int64) string {
	switch {
	case days >= 730:
		return fmt.Sprintf("%d years", days/365)
	case days >= 60:
		return fmt.Sprintf("%d months", days/30)
	case days >= 14:
		return fmt.Sprintf("%d weeks", days/7)
	}
	return fmt.Sprintf("%d days", days)
}
