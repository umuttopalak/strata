// Package export writes machine-readable representations of a timeline.
package export

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/umuttopalak/strata/internal/gitlog"
	"github.com/umuttopalak/strata/internal/timeline"
)

const SchemaVersion = 1

type Document struct {
	SchemaVersion int      `json:"schema_version"`
	Repository    string   `json:"repository"`
	Bounds        Bounds   `json:"bounds"`
	Depth         int      `json:"depth"`
	Columns       []string `json:"columns"`
	Totals        Totals   `json:"totals"`
	Frames        []Frame  `json:"frames"`
	Events        []Event  `json:"events"`
	Commits       []Commit `json:"commits"`
}

type Bounds struct {
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

type Totals struct {
	Commits int     `json:"commits"`
	Frames  int     `json:"frames"`
	Final   []int64 `json:"final"`
	Peak    []int64 `json:"peak"`
}

type Frame struct {
	Commit  int     `json:"commit"`
	Values  []int64 `json:"values"`
	Touched []int   `json:"touched"`
	Caption *Commit `json:"caption,omitempty"`
}

type Commit struct {
	Commit  int       `json:"commit"`
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Subject string    `json:"subject"`
	Time    time.Time `json:"time"`
	Added   int       `json:"added"`
	Deleted int       `json:"deleted"`
	Files   int       `json:"files"`
}

type Event struct {
	Kind   string `json:"kind"`
	Commit int    `json:"commit"`
	Frame  int    `json:"frame"`
	Name   string `json:"name,omitempty"`
	Value  int64  `json:"value,omitempty"`
}

func Build(repo string, bounds gitlog.Bounds, tl *timeline.Timeline) Document {
	d := Document{SchemaVersion: SchemaVersion, Repository: repo,
		Bounds: Bounds{First: bounds.First, Last: bounds.Last}, Depth: tl.Depth,
		Columns: append([]string(nil), tl.Columns...), Totals: Totals{
			Commits: tl.Commits, Frames: len(tl.Frames), Peak: append([]int64(nil), tl.Peak...),
		}, Events: make([]Event, 0, len(tl.Events)), Commits: []Commit{}}
	if len(tl.Frames) > 0 {
		d.Totals.Final = append([]int64(nil), tl.Frames[len(tl.Frames)-1].Totals...)
	}
	for _, f := range tl.Frames {
		out := Frame{Commit: f.Commit, Values: append([]int64(nil), f.Totals...), Touched: append([]int(nil), f.Touched...)}
		if f.Caption != nil {
			c := commit(f.Commit, f.Caption)
			out.Caption = &c
			d.Commits = append(d.Commits, c)
		}
		d.Frames = append(d.Frames, out)
	}
	for _, e := range tl.Events {
		d.Events = append(d.Events, Event{Kind: eventKind(e.Kind), Commit: e.Commit, Frame: e.Frame, Name: e.Name, Value: e.Value})
	}
	return d
}

func commit(number int, c *timeline.Caption) Commit {
	return Commit{Commit: number, Hash: c.Hash, Author: c.Author, Subject: c.Subject, Time: c.Time,
		Added: c.Added, Deleted: c.Deleted, Files: c.Files}
}

func eventKind(k timeline.EventKind) string {
	return [...]string{"first_commit", "tag", "cleanup", "restructure", "quiet", "milestone", "contributor"}[k]
}

func WriteJSON(path string, repo string, bounds gitlog.Bounds, tl *timeline.Timeline) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(Build(repo, bounds, tl)); err != nil {
		return err
	}
	return f.Close()
}

// WriteCSV uses record_type rows: frame_column, event, and commit.
func WriteCSV(path string, repo string, bounds gitlog.Bounds, tl *timeline.Timeline) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	header := []string{"schema_version", "repository", "first", "last", "record_type", "frame", "commit", "column", "value", "touched", "kind", "name", "event_value", "hash", "author", "subject", "time", "added", "deleted", "files"}
	if err = w.Write(header); err != nil {
		f.Close()
		return err
	}
	first, last := bounds.First.Format(time.RFC3339), bounds.Last.Format(time.RFC3339)
	write := func(row []string) error {
		return w.Write(append([]string{strconv.Itoa(SchemaVersion), repo, first, last}, row...))
	}
	for fi, frame := range tl.Frames {
		for ci, value := range frame.Totals {
			touched := 0
			if ci < len(frame.Touched) {
				touched = frame.Touched[ci]
			}
			row := make([]string, 16)
			row[0], row[1], row[2], row[3], row[4], row[5] = "frame_column", strconv.Itoa(fi), strconv.Itoa(frame.Commit), tl.Columns[ci], strconv.FormatInt(value, 10), strconv.Itoa(touched)
			if err = write(row); err != nil {
				f.Close()
				return err
			}
		}
	}
	for _, e := range tl.Events {
		row := make([]string, 16)
		row[0], row[1], row[2], row[6], row[7], row[8] = "event", strconv.Itoa(e.Frame), strconv.Itoa(e.Commit), eventKind(e.Kind), e.Name, strconv.FormatInt(e.Value, 10)
		if err = write(row); err != nil {
			f.Close()
			return err
		}
	}
	for _, frame := range tl.Frames {
		if frame.Caption == nil {
			continue
		}
		c := frame.Caption
		row := make([]string, 16)
		row[0], row[2], row[9], row[10], row[11], row[12], row[13], row[14], row[15] = "commit", strconv.Itoa(frame.Commit), c.Hash, c.Author, c.Subject, c.Time.Format(time.RFC3339), strconv.Itoa(c.Added), strconv.Itoa(c.Deleted), strconv.Itoa(c.Files)
		if err = write(row); err != nil {
			f.Close()
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func EncodeJSON(w io.Writer, repo string, bounds gitlog.Bounds, tl *timeline.Timeline) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Build(repo, bounds, tl))
}
