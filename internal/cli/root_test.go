package cli

import (
	"strings"
	"testing"
)

func TestFlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		f       flags
		wantErr string
	}{
		{"defaults", flags{speed: 1, depth: 1}, ""},
		{"with since", flags{speed: 2, depth: 3, since: "2020-01-31"}, ""},
		{"zero speed", flags{speed: 0, depth: 1}, "--speed"},
		{"negative speed", flags{speed: -1, depth: 1}, "--speed"},
		{"automatic depth", flags{speed: 1, depth: 0}, ""},
		{"negative depth", flags{speed: 1, depth: -1}, "--depth"},
		{"bad since", flags{speed: 1, depth: 1, since: "31/01/2020"}, "--since"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.f.config(".")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want mention of %s", err, tt.wantErr)
			}
		})
	}
}

func TestSinceParsed(t *testing.T) {
	cfg, err := flags{speed: 1, depth: 1, since: "2020-01-31"}.config(".")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Since.Format("2006-01-02"); got != "2020-01-31" {
		t.Errorf("Since = %s", got)
	}
}
