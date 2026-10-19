package authz_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// FuzzArtifactPath seeds from the traversal corpus and asserts that nothing an
// Extractor admits can name a place outside the extraction root: the cleaned
// name is relative and has no "..", a symlink target only climbs at its start
// and never above the root, and no admitted name sits below an admitted symlink.
func FuzzArtifactPath(f *testing.F) {
	for _, c := range loadCorpus(f) {
		var first, second corpusEntry
		first = c.Entries[0]
		second = c.Entries[len(c.Entries)-1]
		a, b := first.entry(), second.entry()
		f.Add(a.Name, a.LinkTarget, uint32(a.Mode), a.Hardlink, a.Size, a.Packed, b.Name, b.LinkTarget, uint32(b.Mode), b.Size)
	}
	f.Fuzz(func(t *testing.T, n1, l1 string, m1 uint32, h1 bool, s1, p1 int64, n2, l2 string, m2 uint32, s2 int64) {
		x := authz.NewExtractor(authz.Limits{})
		e1 := authz.Entry{Name: n1, LinkTarget: l1, Mode: fs.FileMode(m1), Hardlink: h1, Size: s1, Packed: p1}
		e2 := authz.Entry{Name: n2, LinkTarget: l2, Mode: fs.FileMode(m2), Size: s2, Packed: p1}
		c1, err1 := x.Admit(e1)
		if err1 == nil {
			checkAdmitted(t, e1, c1)
		}
		c2, err2 := x.Admit(e2)
		if err2 != nil {
			return
		}
		checkAdmitted(t, e2, c2)
		if err1 == nil && e1.Mode.Type() == fs.ModeSymlink && strings.HasPrefix(strings.ToLower(c2), strings.ToLower(c1)+"/") {
			t.Fatalf("%q admitted below admitted symlink %q", c2, c1)
		}
	})
}

func checkAdmitted(t *testing.T, e authz.Entry, clean string) {
	t.Helper()
	if e.Hardlink || e.Mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		t.Fatalf("admitted a hardlink or set-id entry %+v", e)
	}
	switch e.Mode.Type() {
	case 0, fs.ModeDir, fs.ModeSymlink:
	default:
		t.Fatalf("admitted special file type %v", e.Mode)
	}
	assertContained(t, clean)
	if strings.ContainsAny(clean, "\x00:") && len(clean) >= 2 && clean[1] == ':' {
		t.Fatalf("drive letter in %q", clean)
	}
	if e.Mode.Type() == fs.ModeSymlink {
		depth := strings.Count(clean, "/")
		climbing := true
		for _, c := range strings.Split(e.LinkTarget, "/") {
			switch c {
			case "", ".":
			case "..":
				if !climbing {
					t.Fatalf("link %q climbs after descending", e.LinkTarget)
				}
				if depth--; depth < 0 {
					t.Fatalf("link %q at %q leaves the root", e.LinkTarget, clean)
				}
			default:
				climbing = false
			}
		}
		if strings.HasPrefix(e.LinkTarget, "/") || strings.Contains(e.LinkTarget, "\x00") {
			t.Fatalf("link target %q admitted", e.LinkTarget)
		}
	}
}
