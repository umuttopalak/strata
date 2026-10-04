package export

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/umuttopalak/strata/internal/gitlog"
	"github.com/umuttopalak/strata/internal/timeline"
)

func TestBuildDocument(t *testing.T) {
	tm := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	tl := &timeline.Timeline{
		Columns: []string{"src", "(root)"}, Commits: 2, Depth: 1, Peak: []int64{10, 2},
		Frames: []timeline.Frame{{Commit: 0, Totals: []int64{0, 0}, Touched: []int{0, 0}},
			{Commit: 2, Totals: []int64{10, 2}, Touched: []int{2, 1}, Caption: &timeline.Caption{Hash: "abc", Author: "A", Subject: "grow", Time: tm, Added: 12, Deleted: 0, Files: 2}}},
		Events: []timeline.Event{{Kind: timeline.EventMilestone, Commit: 2, Frame: 1, Value: 1_000}},
	}
	d := Build("repo", gitlog.Bounds{First: tm, Last: tm.Add(time.Hour)}, tl)
	if d.SchemaVersion != 1 || d.Repository != "repo" || d.Totals.Frames != 2 {
		t.Fatalf("bad document header: %#v", d)
	}
	if len(d.Frames) != 2 || len(d.Commits) != 1 || d.Frames[1].Caption.Hash != "abc" {
		t.Fatalf("bad frame/commit export: %#v", d)
	}
	if d.Events[0].Kind != "milestone" {
		t.Errorf("event kind = %q", d.Events[0].Kind)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"schema_version":1`)) {
		t.Errorf("schema version missing: %s", b)
	}
}
