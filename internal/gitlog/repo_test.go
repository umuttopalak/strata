package gitlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/umuttopalak/strata/internal/testutil"
)

func TestOpenRepo(t *testing.T) {
	ctx := context.Background()

	empty := testutil.NewRepo(t)

	full := testutil.NewRepo(t)
	full.Write("a.txt", "hi\n")
	full.Commit("first", testutil.Day(2020, 1, 1))
	sub := filepath.Join(full.Dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want error
	}{
		{"missing path", filepath.Join(t.TempDir(), "nope"), ErrPathMissing},
		{"file not dir", filepath.Join(full.Dir, "a.txt"), ErrNotDir},
		{"not a repo", t.TempDir(), ErrNotRepo},
		{"empty repo", empty.Dir, ErrEmptyRepo},
		{"valid repo", full.Dir, nil},
		{"subdirectory", sub, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := OpenRepo(ctx, tt.path)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want != nil {
				return
			}
			want, _ := filepath.EvalSymlinks(full.Dir)
			got, _ := filepath.EvalSymlinks(repo.Root)
			if got != want {
				t.Errorf("Root = %q, want %q", got, want)
			}
		})
	}
}

func TestShallow(t *testing.T) {
	ctx := context.Background()
	src := testutil.NewRepo(t)
	for i := range 3 {
		src.Write("a.txt", testutil.Lines(i+1))
		src.Commit("c", testutil.Day(2020, 1, 1+i))
	}
	full, err := OpenRepo(ctx, src.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if full.Shallow(ctx) {
		t.Error("full repository reported as shallow")
	}

	dst := filepath.Join(t.TempDir(), "clone")
	src.Git("clone", "-q", "--depth", "1", "file://"+src.Dir, dst)
	clone, err := OpenRepo(ctx, dst)
	if err != nil {
		t.Fatal(err)
	}
	if !clone.Shallow(ctx) {
		t.Error("depth-1 clone not reported as shallow")
	}
}

func TestCountCommits(t *testing.T) {
	r := testutil.NewRepo(t)
	for i := range 4 {
		r.Write("a", testutil.Lines(i+1))
		r.Commit("c", testutil.Day(2020, 1, 1+i))
	}
	repo, err := OpenRepo(context.Background(), r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := repo.CountCommits(context.Background()); err != nil || n != 4 {
		t.Errorf("CountCommits = %d, %v", n, err)
	}
}
