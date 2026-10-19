package authz_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// runCase feeds a corpus case through an Extractor and returns the error of the
// last entry; every earlier entry must be admitted.
func runCase(t *testing.T, c corpusCase) (string, error) {
	t.Helper()
	x := authz.NewExtractor(authz.Limits{})
	var clean string
	var err error
	for i, e := range c.Entries {
		clean, err = x.Admit(e.entry())
		if i < len(c.Entries)-1 && err != nil {
			t.Fatalf("setup entry %d (%q) refused: %v", i, e.Name, err)
		}
	}
	return clean, err
}

func TestArtifactPath_TraversalCorpus(t *testing.T) {
	rejected, accepted := 0, 0
	for _, c := range loadCorpus(t) {
		t.Run(c.ID, func(t *testing.T) {
			clean, err := runCase(t, c)
			if c.Reason == "" {
				accepted++
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				assertContained(t, clean)
				return
			}
			rejected++
			var pe *authz.PathError
			if !errors.As(err, &pe) {
				t.Fatalf("want *PathError %s, got %v (clean %q)", c.Reason, err, clean)
			}
			if string(pe.Reason) != c.Reason {
				t.Fatalf("reason %s, want %s", pe.Reason, c.Reason)
			}
			if !strings.HasPrefix(pe.Error(), authz.CodeArtifactPath+":") {
				t.Fatalf("error text %q lacks the code", pe.Error())
			}
		})
	}
	if rejected < 50 || accepted < 8 {
		t.Fatalf("corpus too small: %d refusals, %d accepts", rejected, accepted)
	}
}

func assertContained(t testing.TB, clean string) {
	t.Helper()
	if clean == "" || strings.HasPrefix(clean, "/") || strings.Contains(clean, `\`) {
		t.Fatalf("bad clean name %q", clean)
	}
	for _, c := range strings.Split(clean, "/") {
		if c == ".." || c == "" || (c == "." && clean != ".") {
			t.Fatalf("component %q in %q", c, clean)
		}
	}
}

// Every refusal class named by the Ticket appears in the corpus.
func TestArtifactPath_CorpusCoversEveryClass(t *testing.T) {
	want := []string{"zip-slip", "symlink-then-file", "absolute", "nul-byte", "windows-drive", "hardlink-out", "fifo",
		"block-device", "case-fold-collision", "ratio-bomb-single", "size-bomb-single"}
	have := map[string]bool{}
	for _, c := range loadCorpus(t) {
		have[c.ID] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("corpus lacks %s", id)
		}
	}
}

func TestArtifactPath_NamesLimitsAndBytes(t *testing.T) {
	file := fs.FileMode(0o644)
	rows := []struct {
		name string
		in   string
		want authz.PathReason
	}{
		{"invalid utf8", "a/\xff\xfe", authz.PathUTF8},
		{"component too long", strings.Repeat("a", 256), authz.PathTooLong},
		{"name too long", strings.Repeat("abcdefgh/", 600) + "x", authz.PathTooLong},
		{"too deep", strings.Repeat("d/", 128) + "x", authz.PathTooDeep},
	}
	for _, r := range rows {
		_, err := authz.ArtifactPath(authz.Entry{Name: r.in, Mode: file, Size: 1, Packed: 1}, authz.Limits{})
		var pe *authz.PathError
		if !errors.As(err, &pe) || pe.Reason != r.want {
			t.Errorf("%s: got %v want %s", r.name, err, r.want)
		}
	}
	got, err := authz.ArtifactPath(authz.Entry{Name: strings.Repeat("d/", 127) + "x", Mode: file, Size: 1, Packed: 1}, authz.Limits{})
	if err != nil {
		t.Fatalf("depth 128 at the limit refused: %v", err)
	}
	assertContained(t, got)
}

func TestExtractor_EntryCountLimit(t *testing.T) {
	x := authz.NewExtractor(authz.Limits{MaxEntries: 3})
	for _, n := range []string{"a", "b", "c"} {
		if _, err := x.Admit(authz.Entry{Name: n, Mode: 0o644, Size: 1, Packed: 1}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := x.Admit(authz.Entry{Name: "d", Mode: 0o644, Size: 1, Packed: 1})
	var pe *authz.PathError
	if !errors.As(err, &pe) || pe.Reason != authz.SizeEntries {
		t.Fatalf("got %v", err)
	}
}

func TestExtractor_CustomLimitsTighten(t *testing.T) {
	lim := authz.Limits{MaxFileBytes: 10, MaxRatio: 2, RatioFloor: 4}
	if _, err := authz.ArtifactPath(authz.Entry{Name: "a", Mode: 0o644, Size: 11, Packed: 11}, lim); err == nil {
		t.Fatal("11 bytes over a 10 byte limit accepted")
	}
	if _, err := authz.ArtifactPath(authz.Entry{Name: "a", Mode: 0o644, Size: 9, Packed: 2}, lim); err == nil {
		t.Fatal("ratio 4 over a limit of 2 accepted")
	}
	if _, err := authz.ArtifactPath(authz.Entry{Name: "a", Mode: 0o644, Size: 9, Packed: 5}, lim); err != nil {
		t.Fatalf("ratio 1 refused: %v", err)
	}
}

// A refused entry leaves the extractor exactly as it was.
func TestExtractor_RefusedEntryIsNotRecorded(t *testing.T) {
	x := authz.NewExtractor(authz.Limits{})
	if _, err := x.Admit(authz.Entry{Name: "../x", Mode: 0o644}); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := x.Admit(authz.Entry{Name: "x", Mode: 0o644, Size: 1, Packed: 1}); err != nil {
		t.Fatalf("clean retry refused: %v", err)
	}
}
