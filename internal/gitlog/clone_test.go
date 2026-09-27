package gitlog

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/umuttopalak/strata/internal/testutil"
)

func TestIsRemote(t *testing.T) {
	remote := []string{
		"https://github.com/torvalds/linux",
		"https://gitlab.com/group/sub/project.git",
		"ssh://git@example.com/repo.git",
		"git@github.com:owner/repo.git",
		"file:///tmp/repo",
		"github.com/owner/repo",
		"codeberg.org/owner/repo",
	}
	local := []string{".", "./repo", "../projects/x", "/abs/path", `C:\code\repo`, "owner/repo", "github.com"}
	for _, a := range remote {
		if !IsRemote(a) {
			t.Errorf("IsRemote(%q) = false", a)
		}
	}
	for _, a := range local {
		if IsRemote(a) {
			t.Errorf("IsRemote(%q) = true", a)
		}
	}
}

func TestRemoteURLAndName(t *testing.T) {
	tests := []struct{ arg, url, name string }{
		{"github.com/owner/repo", "https://github.com/owner/repo", "repo"},
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git", "repo"},
		{"https://github.com/owner/repo/", "https://github.com/owner/repo/", "repo"},
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git", "repo"},
	}
	for _, tt := range tests {
		if got := RemoteURL(tt.arg); got != tt.url {
			t.Errorf("RemoteURL(%q) = %q, want %q", tt.arg, got, tt.url)
		}
		if got := RepoName(tt.url); got != tt.name {
			t.Errorf("RepoName(%q) = %q, want %q", tt.url, got, tt.name)
		}
	}
}

// Cloning over file:// exercises the same path as a network URL.
func TestCloneAndReadBare(t *testing.T) {
	src := testutil.NewRepo(t)
	src.Write("src/a.go", testutil.Lines(10))
	src.Commit("one", testutil.Day(2020, 1, 1))
	src.Git("tag", "v1")
	src.Write("src/a.go", testutil.Lines(15))
	src.Commit("two", testutil.Day(2020, 2, 1))

	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "clone")
	if err := Clone(ctx, "file://"+src.Dir, dir, nil); err != nil {
		t.Fatal(err)
	}
	repo, err := OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo on a bare clone: %v", err)
	}
	var subjects []string
	var tags []string
	if err := repo.Walk(ctx, LogOptions{}, func(c Commit) error {
		subjects = append(subjects, c.Subject)
		tags = append(tags, c.Tags...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(subjects, []string{"one", "two"}) || !slices.Equal(tags, []string{"v1"}) {
		t.Errorf("subjects %v tags %v", subjects, tags)
	}
	if err := RemoveClone(dir); err != nil {
		t.Fatalf("RemoveClone: %v", err)
	}
}

func TestCloneFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clone")
	err := Clone(context.Background(), "file://"+filepath.Join(t.TempDir(), "missing"), dir, nil)
	if !errors.Is(err, ErrCloneFailed) {
		t.Fatalf("err = %v, want ErrCloneFailed", err)
	}
}

func TestProgressWriter(t *testing.T) {
	var got []CloneProgress
	w := &progressWriter{report: func(p CloneProgress) { got = append(got, p) }}
	// Git rewrites a line with \r and ends phases with \n; writes may split
	// anywhere.
	chunks := []string{
		"Cloning into bare repository '/tmp/x'...\n",
		"remote: Enumerating objects: 4142, done.\n",
		"remote: Counting objects:  50% (698/1396)\rremote: Counting objects: 100% (1396/1396), done.\n",
		"remote: Compressing objects: 100% (352/352), done.\n",
		"Receiving objects:  12% (498/4142), 180.00 KiB | 350.00 KiB/s\rReceiving obj",
		"ects: 100% (4142/4142), 1.15 MiB | 1.35 MiB/s, done.\n",
		"Resolving deltas:  40% (1016/2538)\r",
	}
	for _, c := range chunks {
		w.Write([]byte(c))
	}
	want := []CloneProgress{
		{"preparing", 50, ""},
		{"preparing", 100, ""},
		{"preparing", 100, ""},
		{"receiving", 12, "180.00 KiB"},
		{"receiving", 100, "1.15 MiB"},
		{"resolving", 40, ""},
	}
	if !slices.Equal(got, want) {
		t.Errorf("progress:\n got %v\nwant %v", got, want)
	}
}
