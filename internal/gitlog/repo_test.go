package gitlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/umuttopalak/summit/internal/testutil"
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
