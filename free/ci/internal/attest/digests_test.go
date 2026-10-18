package attest_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestInputDigests_SortedSha256OverCanonicalConfig(t *testing.T) {
	cfg := map[string]any{"b": 1, "a": []any{"x", true}}
	d, err := attest.InputDigests(cfg, []attest.File{{Path: "z.lock", Content: []byte("z")}, {Path: "a.lock", Content: []byte("a")}},
		[]attest.File{{Path: "gen/b.go", Content: []byte("b")}, {Path: "gen/a.go", Content: nil}})
	if err != nil {
		t.Fatal(err)
	}
	canon, _ := trust.Canonical(cfg)
	sum := sha256.Sum256(canon)
	if d.Config != hex.EncodeToString(sum[:]) {
		t.Fatalf("config digest %s", d.Config)
	}
	if len(d.Lockfiles) != 2 || d.Lockfiles[0].Path != "a.lock" || d.Lockfiles[1].Path != "z.lock" {
		t.Fatalf("lockfiles not sorted: %+v", d.Lockfiles)
	}
	empty := sha256.Sum256(nil)
	if d.Generated[0].Path != "gen/a.go" || d.Generated[0].SHA256 != hex.EncodeToString(empty[:]) {
		t.Fatalf("generated %+v", d.Generated)
	}
}

func TestInputDigests_ConfigKeyOrderDoesNotMatter(t *testing.T) {
	a, _ := attest.InputDigests(map[string]any{"x": 1, "y": 2}, nil, nil)
	b, _ := attest.InputDigests(map[string]any{"y": 2, "x": 1}, nil, nil)
	c, _ := attest.InputDigests(map[string]any{"y": 2, "x": 3}, nil, nil)
	if a.Config != b.Config || a.Config == c.Config {
		t.Fatalf("digests %s %s %s", a.Config, b.Config, c.Config)
	}
}

func TestInputDigests_EmptyListsAreNotNil(t *testing.T) {
	d, err := attest.InputDigests(map[string]any{}, nil, nil)
	if err != nil || d.Lockfiles == nil || d.Generated == nil {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestInputDigests_Refusals(t *testing.T) {
	ok := map[string]any{"a": 1}
	for name, run := range map[string]func() error{
		"nil config":     func() error { _, e := attest.InputDigests(nil, nil, nil); return e },
		"float config":   func() error { _, e := attest.InputDigests(map[string]any{"a": 1.5}, nil, nil); return e },
		"duplicate path": func() error { _, e := attest.InputDigests(ok, []attest.File{{Path: "a"}, {Path: "a"}}, nil); return e },
		"empty path":     func() error { _, e := attest.InputDigests(ok, []attest.File{{Path: ""}}, nil); return e },
		"absolute":       func() error { _, e := attest.InputDigests(ok, nil, []attest.File{{Path: "/etc/passwd"}}); return e },
		"dotdot":         func() error { _, e := attest.InputDigests(ok, nil, []attest.File{{Path: "../x"}}); return e },
		"unclean":        func() error { _, e := attest.InputDigests(ok, nil, []attest.File{{Path: "a//b"}}); return e },
		"backslash":      func() error { _, e := attest.InputDigests(ok, nil, []attest.File{{Path: `a\b`}}); return e },
	} {
		if run() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// The same path may be a lockfile and a generated input of different lists.
func TestInputDigests_SamePathInTwoLists(t *testing.T) {
	if _, err := attest.InputDigests(map[string]any{}, []attest.File{{Path: "a"}}, []attest.File{{Path: "a"}}); err != nil {
		t.Fatal(err)
	}
}

func TestDigestAgreesWithFresh(t *testing.T) {
	ev := goodEvidence()
	locks := []attest.File{{Path: "go.sum", Content: []byte("one")}}
	d1, _ := attest.InputDigests(map[string]any{"a": 1}, locks, nil)
	ev.InputDigests = d1
	d2, _ := attest.InputDigests(map[string]any{"a": 1}, locks, nil)
	if f := attest.Fresh(ev, d2); f.State != "current" {
		t.Fatalf("%+v", f)
	}
	d3, _ := attest.InputDigests(map[string]any{"a": 1}, []attest.File{{Path: "go.sum", Content: []byte("two")}}, nil)
	if f := attest.Fresh(ev, d3); f.State != "stale" {
		t.Fatalf("%+v", f)
	}
}
