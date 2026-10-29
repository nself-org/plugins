package main

import (
	"errors"
	"strings"
	"testing"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestRunsRerunSha(t *testing.T) {
	never := func(string) (string, error) { t.Fatal("lookup must not run for a sha"); return "", nil }
	known := func(string) bool { return true }
	for name, tc := range map[string]struct {
		ref    string
		lookup func(string) (string, error)
		exists func(string) bool
		head   cirun.Revision
		want   string // "" = ok; else a substring of the refusal
	}{
		"sha without a record, clean at head": {shaA, never, known, cirun.Revision{SHA: shaA}, ""},
		"run id resolves to its revision":     {"run-1", func(string) (string, error) { return shaA, nil }, known, cirun.Revision{SHA: shaA}, ""},
		"dirty tree refuses":                  {shaA, never, known, cirun.Revision{SHA: shaA, Dirty: true}, "uncommitted changes"},
		"head is another commit":              {shaB, never, known, cirun.Revision{SHA: shaA}, "check out the revision first"},
		"unknown commit":                      {shaA, never, func(string) bool { return false }, cirun.Revision{SHA: shaA}, "not a commit"},
		"no commit":                           {shaA, never, known, cirun.Revision{}, "no commit"},
		"unknown run":                         {"nope", func(string) (string, error) { return "", errors.New("E626: no run") }, known, cirun.Revision{SHA: shaA}, "E626"},
	} {
		got, err := rerunRevision(tc.ref, tc.lookup, tc.exists, tc.head)
		switch {
		case tc.want == "" && (err != nil || got != shaA):
			t.Errorf("%s: %q %v", name, got, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: want %q, got %v", name, tc.want, err)
		}
	}
	if _, err := rerunRevision(shaB, never, known, cirun.Revision{SHA: shaA, Dirty: true}); cirun.CodeOf(err) != cirun.CodeRun {
		t.Errorf("refusals carry %s, got %v", cirun.CodeRun, err)
	}
}

func TestRunsRerunRegistered(t *testing.T) {
	if _, ok := runsSubs["rerun"]; !ok {
		t.Fatal("rerun is not a runs subcommand")
	}
	if f := newRerunFlagSet(&rerunFlags{}); f.Lookup("where") == nil || f.Lookup("job") == nil {
		t.Fatal("rerun lacks --where or --job")
	}
}
