package gitlog

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func parseAll(t *testing.T, input string) []Commit {
	t.Helper()
	var got []Commit
	if err := Parse(strings.NewReader(input), func(c Commit) error {
		got = append(got, c)
		return nil
	}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return got
}

func TestParseFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/numstat.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := parseAll(t, string(data))

	want := []Commit{
		{
			Hash: "aaa111", Time: time.Unix(1577836800, 0), AuthorTime: time.Unix(1577836700, 0),
			Author: "Ada Lovelace", Subject: "initial commit",
			Files: []FileChange{
				{Path: "README.md", Added: 10, Deleted: 0},
				{Path: "src/main.go", Added: 120, Deleted: 0},
			},
		},
		{
			// Binary file skipped, quoted path unescaped, UTF-8 kept verbatim.
			Hash: "bbb222", Time: time.Unix(1577923200, 0), AuthorTime: time.Unix(1577923200, 0),
			Author: "Grace Hopper", Subject: "add assets: logo\tand docs",
			Tags: []string{"v1.0", "stable"},
			Files: []FileChange{
				{Path: "docs/tab\there.md", Added: 3, Deleted: 1},
				{Path: "docs/ünicode.md", Added: 7, Deleted: 0},
			},
		},
		{
			Hash: "ccc333", Time: time.Unix(1578009600, 0), AuthorTime: time.Unix(1578009600, 0),
			Author: "Ada Lovelace", Subject: "",
		},
		{
			Hash: "ddd444", Time: time.Unix(1578096000, 0), AuthorTime: time.Unix(1578096000, 0),
			Author: "Linus", Subject: "remove main",
			Files: []FileChange{{Path: "src/main.go", Added: 0, Deleted: 120}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got:\n%+v\nwant:\n%+v", got, want)
	}
}

func TestParseCRLF(t *testing.T) {
	input := "\x1eh1\x1f10\x1f10\x1fa\x1f\x1fs\r\n\r\n1\t2\tx.go\r\n"
	got := parseAll(t, input)
	if len(got) != 1 || len(got[0].Files) != 1 || got[0].Files[0] != (FileChange{"x.go", 1, 2}) {
		t.Fatalf("got %+v", got)
	}
	if got[0].Subject != "s" {
		t.Errorf("subject = %q", got[0].Subject)
	}
}

func TestParseEmpty(t *testing.T) {
	if got := parseAll(t, ""); len(got) != 0 {
		t.Fatalf("got %d commits from empty input", len(got))
	}
}

func TestParseErrors(t *testing.T) {
	tests := map[string]string{
		"stats before commit": "1\t2\tx.go\n",
		"short header":        "\x1eh1\x1f10\n",
		"bad timestamp":       "\x1eh1\x1fnope\x1f10\x1fa\x1f\x1fs\n",
		"bad numstat":         "\x1eh1\x1f10\x1f10\x1fa\x1f\x1fs\nx\ty\tz\n",
		"missing path":        "\x1eh1\x1f10\x1f10\x1fa\x1f\x1fs\n1\t2\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			err := Parse(strings.NewReader(input), func(Commit) error { return nil })
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParseStopsOnCallbackError(t *testing.T) {
	input := "\x1ea\x1f1\x1f1\x1fx\x1f\x1fs\n\x1eb\x1f2\x1f2\x1fx\x1f\x1fs\n"
	stop := errors.New("stop")
	calls := 0
	err := Parse(strings.NewReader(input), func(Commit) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, calls)
	}
}
