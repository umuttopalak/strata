package gitlog

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// FileChange is one numstat entry: lines added and deleted in a file.
type FileChange struct {
	Path    string
	Added   int
	Deleted int
}

// Commit is one commit with its per-file line changes.
type Commit struct {
	Hash       string
	Time       time.Time // committer date, used for ordering and bucketing
	AuthorTime time.Time // author date, shown to the user
	Author     string
	Subject    string
	Tags       []string // tags pointing at this commit
	Files      []FileChange
}

const (
	recordSep = '\x1e'
	fieldSep  = "\x1f"
)

// logFormat must stay in sync with parseHeader.
const logFormat = "--format=%x1e%H%x1f%ct%x1f%at%x1f%an%x1f%D%x1f%s"

// Parse reads `git log --numstat` output produced with logFormat and calls
// fn for each commit, in input order. It stops at the first error from fn.
func Parse(r io.Reader, fn func(Commit) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)

	var cur *Commit
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSuffix(sc.Text(), "\r")
		switch {
		case line == "":
			continue
		case line[0] == recordSep:
			if cur != nil {
				if err := fn(*cur); err != nil {
					return err
				}
			}
			c, err := parseHeader(line[1:])
			if err != nil {
				return fmt.Errorf("git log output line %d: %w", lineNo, err)
			}
			cur = &c
		default:
			if cur == nil {
				return fmt.Errorf("git log output line %d: file stats before any commit", lineNo)
			}
			fc, ok, err := parseNumstat(line)
			if err != nil {
				return fmt.Errorf("git log output line %d: %w", lineNo, err)
			}
			if ok {
				cur.Files = append(cur.Files, fc)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("reading git log output: %w", err)
	}
	if cur != nil {
		return fn(*cur)
	}
	return nil
}

func parseHeader(s string) (Commit, error) {
	f := strings.SplitN(s, fieldSep, 6)
	if len(f) != 6 {
		return Commit{}, fmt.Errorf("malformed commit header %q", s)
	}
	ct, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil {
		return Commit{}, fmt.Errorf("bad commit timestamp %q", f[1])
	}
	at, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil {
		return Commit{}, fmt.Errorf("bad author timestamp %q", f[2])
	}
	return Commit{
		Hash:       f[0],
		Time:       time.Unix(ct, 0),
		AuthorTime: time.Unix(at, 0),
		Author:     f[3],
		Tags:       tags(f[4]),
		Subject:    f[5],
	}, nil
}

// tags picks the tag names out of a %D ref list such as
// "HEAD -> main, tag: v1.2.0, origin/main".
func tags(refs string) []string {
	var out []string
	for _, r := range strings.Split(refs, ", ") {
		if name, ok := strings.CutPrefix(r, "tag: "); ok {
			out = append(out, name)
		}
	}
	return out
}

// parseNumstat parses "added<TAB>deleted<TAB>path". Binary files ("-\t-")
// report ok=false since they have no line counts.
func parseNumstat(line string) (FileChange, bool, error) {
	f := strings.SplitN(line, "\t", 3)
	if len(f) != 3 {
		return FileChange{}, false, fmt.Errorf("malformed numstat line %q", line)
	}
	if f[0] == "-" && f[1] == "-" {
		return FileChange{}, false, nil
	}
	added, err1 := strconv.Atoi(f[0])
	deleted, err2 := strconv.Atoi(f[1])
	if err1 != nil || err2 != nil {
		return FileChange{}, false, fmt.Errorf("malformed numstat line %q", line)
	}
	path := f[2]
	// Paths with control characters or quotes are C-quoted by git even with
	// core.quotePath=false; Go's Unquote understands the same escapes.
	if strings.HasPrefix(path, `"`) {
		if p, err := strconv.Unquote(path); err == nil {
			path = p
		}
	}
	return FileChange{Path: path, Added: added, Deleted: deleted}, true, nil
}
