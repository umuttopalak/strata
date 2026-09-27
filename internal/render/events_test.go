package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/umuttopalak/strata/internal/timeline"
)

func TestEventText(t *testing.T) {
	tests := []struct {
		e    timeline.Event
		want string
	}{
		{timeline.Event{Kind: timeline.EventFirstCommit}, "▲ first commit"},
		{timeline.Event{Kind: timeline.EventTag, Name: "v1.2.0"}, "◆ v1.2.0"},
		{timeline.Event{Kind: timeline.EventCleanup, Value: 698_700}, "▼ big cleanup · −698.7k lines"},
		{timeline.Event{Kind: timeline.EventRestructure, Value: 340}, "⇄ restructure · 340 files"},
		{timeline.Event{Kind: timeline.EventQuiet, Value: 185}, "… quiet for 6 months"},
		{timeline.Event{Kind: timeline.EventQuiet, Value: 800}, "… quiet for 2 years"},
		{timeline.Event{Kind: timeline.EventMilestone, Value: 10_000}, "★ 10k lines"},
		{timeline.Event{Kind: timeline.EventMilestone, Value: 1_000_000}, "★ 1M lines"},
		{timeline.Event{Kind: timeline.EventContributor, Name: "Ada"}, "+ Ada joins"},
	}
	for _, tt := range tests {
		if got := EventText(tt.e); got != tt.want {
			t.Errorf("EventText(%+v) = %q, want %q", tt.e, got, tt.want)
		}
	}
}

// eventTimeline has 40 frames with events at frames 5 (two), 12 and 13.
func eventTimeline() *timeline.Timeline {
	frames := make([][]int64, 40)
	for i := range frames {
		frames[i] = []int64{int64(i * 10)}
	}
	tl := timelineOf([]string{"src"}, frames...)
	tl.Events = []timeline.Event{
		{Kind: timeline.EventMilestone, Frame: 5, Value: 1000},
		{Kind: timeline.EventTag, Frame: 5, Name: "v1"},
		{Kind: timeline.EventQuiet, Frame: 12, Value: 90},
		{Kind: timeline.EventContributor, Frame: 13, Name: "Grace"},
	}
	return tl
}

func TestActiveEvent(t *testing.T) {
	tl := eventTimeline()
	hold := eventHold(len(tl.Frames)) // 40/12 → 3
	tests := []struct {
		frame int
		want  string
	}{
		{4, ""},
		{5, "◆ v1"}, // the tag outranks the milestone in the same frame
		{5 + hold - 1, "◆ v1"},
		{5 + hold, ""},
		{12, "… quiet for 3 months"},
		{13, "+ Grace joins"}, // the most recent event wins
		{13 + hold, ""},
	}
	for _, tt := range tests {
		got := ""
		if e, ok := activeEvent(tl, tt.frame); ok {
			got = EventText(e)
		}
		if got != tt.want {
			t.Errorf("frame %d: %q, want %q", tt.frame, got, tt.want)
		}
	}
}

func TestHeaderWithEvent(t *testing.T) {
	tl := eventTimeline()
	l := NewLayout(tl)
	for _, width := range []int{1, 12, 20, 30, 60, 120} {
		p := Draw(tl, l, 5, "repo", width, 12)
		header := p.Lines()[0]
		if w := ansi.StringWidth(header); w != width {
			t.Fatalf("width %d: header is %d wide: %q", width, w, header)
		}
		if ansi.Strip(p.Styled()[0]) != header {
			t.Fatalf("width %d: styled header differs", width)
		}
		if width >= 60 {
			if !strings.Contains(header, "◆ v1") {
				t.Errorf("width %d: event missing: %q", width, header)
			}
			// Centred in the line, clear of the repo name and the date.
			at := strings.Index(header, "◆")
			if at < len("$ strata repo")+2 || at > width/2 {
				t.Errorf("width %d: event at %d: %q", width, at, header)
			}
		}
	}
}

func TestSVGIncludesEvents(t *testing.T) {
	tl := eventTimeline()
	var b strings.Builder
	if err := WriteSVG(&b, tl, NewLayout(tl), DefaultSVGOptions); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `class="e">◆ v1</text>`) {
		t.Error("tag event missing from the SVG")
	}
}
