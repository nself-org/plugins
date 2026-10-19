package authz_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestCacheNamespace_UntrustedIsPerHeadRepoAndPR(t *testing.T) {
	a := mustNS(t, "untrusted", authz.Unprotected, "fork-a/repo", "7")
	b := mustNS(t, "untrusted", authz.Unprotected, "fork-a/repo", "8")
	c := mustNS(t, "untrusted", authz.Unprotected, "fork-b/repo", "7")
	for name, other := range map[string]authz.Namespace{"other PR": b, "other head repo": c} {
		if a.Equal(other) || bytes.Equal(a.Canonical(), other.Canonical()) {
			t.Fatalf("%s shares a namespace", name)
		}
	}
	if !a.Equal(mustNS(t, "untrusted", authz.Unprotected, "fork-a/repo", "7")) {
		t.Fatal("same inputs must give the same namespace")
	}
}

func TestCacheNamespace_UntrustedNeedsHeadRepoAndPR(t *testing.T) {
	for _, in := range [][2]string{{"", "7"}, {"fork/repo", ""}, {"", ""}} {
		_, err := authz.CacheNamespace("proj-1", "untrusted", authz.Unprotected, in[0], in[1])
		var d *authz.Denied
		if !errors.As(err, &d) || d.Reason != authz.ReasonNamespace {
			t.Fatalf("head=%q pr=%q: got %v", in[0], in[1], err)
		}
	}
}

func TestCacheNamespace_UnknownTrustIsUntrusted(t *testing.T) {
	for _, class := range []model.TrustClass{"", "root", "OWNER", "trusted"} {
		ns, err := authz.CacheNamespace("proj-1", class, authz.Protected, "fork/repo", "1")
		if err != nil {
			t.Fatal(err)
		}
		if ns.Trust != "untrusted" || ns.Protection != authz.Unprotected || ns.PR != "1" {
			t.Fatalf("class %q -> %+v", class, ns)
		}
	}
	if _, err := authz.CacheNamespace("proj-1", "root", authz.Protected, "", ""); err == nil {
		t.Fatal("unknown class without PR must fail like an untrusted one")
	}
}

func TestCacheNamespace_TrustedClearsHeadRepoAndPR(t *testing.T) {
	for _, class := range trustedClasses {
		ns := mustNS(t, class, authz.Protected, "fork/repo", "9")
		if ns.HeadRepo != "" || ns.PR != "" || ns.Protection != authz.Protected {
			t.Fatalf("%s -> %+v", class, ns)
		}
	}
	if ns := mustNS(t, "owner", "bogus", "", ""); ns.Protection != authz.Unprotected {
		t.Fatalf("unknown protection must read unprotected: %+v", ns)
	}
}

func TestCacheNamespace_EmptyProjectRefused(t *testing.T) {
	if _, err := authz.CacheNamespace("", "owner", authz.Protected, "", ""); err == nil {
		t.Fatal("empty project accepted")
	}
}

func TestNamespace_TrustedAndUntrustedNeverEqual(t *testing.T) {
	un := mustNS(t, "untrusted", authz.Unprotected, "fork/repo", "1")
	for _, class := range trustedClasses {
		for _, p := range []authz.Protection{authz.Protected, authz.Unprotected} {
			if tr := mustNS(t, class, p, "", ""); tr.Equal(un) || un.Equal(tr) {
				t.Fatalf("%s/%s equals an untrusted namespace", class, p)
			}
		}
	}
}

func TestNamespace_EqualIsFieldwiseAndInvalidEqualsNothing(t *testing.T) {
	var zero authz.Namespace
	if zero.Equal(zero) || zero.Valid() {
		t.Fatal("zero namespace must equal nothing")
	}
	base := mustNS(t, "owner", authz.Protected, "", "")
	other := base
	other.Project = "proj-2"
	if base.Equal(other) {
		t.Fatal("different project equal")
	}
}

// Golden vector: the canonical encoding is API (P7-CACHE-01 stores it).
func TestNamespace_CanonicalGolden(t *testing.T) {
	ns := mustNS(t, "untrusted", authz.Unprotected, "fork/repo", "7")
	got := hex.EncodeToString(ns.Canonical())
	golden := "00000013" + hex.EncodeToString([]byte("nself-ci-cache-ns/1")) +
		"00000006" + hex.EncodeToString([]byte("proj-1")) +
		"00000009" + hex.EncodeToString([]byte("untrusted")) +
		"0000000b" + hex.EncodeToString([]byte("unprotected")) +
		"00000009" + hex.EncodeToString([]byte("fork/repo")) +
		"00000001" + hex.EncodeToString([]byte("7"))
	if got != golden {
		t.Fatalf("canonical encoding changed:\n got %s\nwant %s", got, golden)
	}
}

func TestNamespace_CanonicalIsInjective(t *testing.T) {
	// Moving a byte across a field boundary must change the encoding.
	a := mustNS(t, "untrusted", authz.Unprotected, "ab", "c")
	b := mustNS(t, "untrusted", authz.Unprotected, "a", "bc")
	if bytes.Equal(a.Canonical(), b.Canonical()) {
		t.Fatal("field boundary is not encoded")
	}
	seen := map[string]authz.Namespace{}
	for _, class := range append([]model.TrustClass{"untrusted"}, trustedClasses...) {
		for _, p := range []authz.Protection{authz.Protected, authz.Unprotected} {
			for _, head := range []string{"", "h", "h/r"} {
				for _, pr := range []string{"", "1", "12"} {
					ns, err := authz.CacheNamespace("proj-1", class, p, head, pr)
					if err != nil {
						continue
					}
					key := string(ns.Canonical())
					if old, dup := seen[key]; dup && old != ns {
						t.Fatalf("%+v and %+v encode alike", old, ns)
					}
					seen[key] = ns
				}
			}
		}
	}
}

func TestDecodeNamespace_RoundTripAndRejects(t *testing.T) {
	for _, ns := range []authz.Namespace{
		mustNS(t, "untrusted", authz.Unprotected, "fork/repo", "7"),
		mustNS(t, "owner", authz.Protected, "", ""),
		mustNS(t, "collaborator", authz.Unprotected, "", ""),
	} {
		got, err := authz.DecodeNamespace(ns.Canonical())
		if err != nil || !got.Equal(ns) {
			t.Fatalf("round trip of %+v: %+v %v", ns, got, err)
		}
	}
	good := mustNS(t, "owner", authz.Protected, "", "").Canonical()
	bad := map[string][]byte{
		"empty":     nil,
		"truncated": good[:len(good)-1],
		"trailing":  append(append([]byte{}, good...), 0),
		"short len": {0, 0},
		"huge len":  {0xff, 0xff, 0xff, 0xff, 'x'},
	}
	for name, b := range bad {
		if _, err := authz.DecodeNamespace(b); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	// A well-formed encoding of an invalid namespace is refused too.
	forged := authz.Namespace{Project: "proj-1", Trust: "untrusted", Protection: authz.Protected, HeadRepo: "h", PR: "1"}
	if _, err := authz.DecodeNamespace(forged.Canonical()); err == nil {
		t.Fatal("untrusted+protected decoded")
	}
}
