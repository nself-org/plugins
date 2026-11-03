package run

import (
	"os"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
)

var goStack = []detect.Fact{{Kind: "stack", Name: "go"}}

func nsDir(t *testing.T, home string, f entries.Facts) string {
	t.Helper()
	ns, err := entries.NamespaceOf(f)
	if err != nil {
		t.Fatalf("namespace of %+v: %v", f, err)
	}
	return persistentGoCacheDir(home, ns)
}

// TestPersistentGoCacheSameNamespaceSameDir: two attempts of one namespace share the directory, and the setup points GOCACHE and GOMODCACHE inside it.
func TestPersistentGoCacheSameNamespaceSameDir(t *testing.T) {
	home := t.TempDir()
	f := entries.Facts{Project: "local:p", Trust: "owner", Protection: "unprotected"}
	if a, b := nsDir(t, home, f), nsDir(t, home, f); a != b {
		t.Fatalf("same namespace gave %s and %s", a, b)
	}
	setup, ok := tryPersistentGoCache(home, false, goStack, f)
	if !ok {
		t.Fatal("a Go process job did not get the persistent cache")
	}
	dir := nsDir(t, home, f)
	if setup.Env["GOCACHE"] != dir+"/build" || setup.Env["GOMODCACHE"] != dir+"/mod" {
		t.Fatalf("env %v, want both under %s", setup.Env, dir)
	}
	for _, d := range []string{setup.Env["GOCACHE"], setup.Env["GOMODCACHE"]} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", d, fi, err)
		}
	}
}

// TestPersistentGoCacheTrustSeparates: namespaces that differ only in trust class, or only in the untrusted PR, never share a directory.
func TestPersistentGoCacheTrustSeparates(t *testing.T) {
	home := t.TempDir()
	owner := nsDir(t, home, entries.Facts{Project: "local:p", Trust: "owner", Protection: "unprotected"})
	untrusted := nsDir(t, home, entries.Facts{Project: "local:p", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/p", PR: "7"})
	otherPR := nsDir(t, home, entries.Facts{Project: "local:p", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/p", PR: "8"})
	if owner == untrusted || untrusted == otherPR || owner == otherPR {
		t.Fatalf("shared directories: owner %s untrusted %s other PR %s", owner, untrusted, otherPR)
	}
}

// TestPersistentGoCacheSkipsContainerJobs: container jobs and non-Go jobs keep the snapshot path.
func TestPersistentGoCacheSkipsContainerJobs(t *testing.T) {
	home := t.TempDir()
	f := entries.Facts{Project: "local:p", Trust: "owner", Protection: "unprotected"}
	if _, ok := tryPersistentGoCache(home, true, goStack, f); ok {
		t.Fatal("a container job used the persistent Go cache")
	}
	if _, ok := tryPersistentGoCache(home, false, []detect.Fact{{Kind: "stack", Name: "node"}}, f); ok {
		t.Fatal("a non-Go job used the persistent Go cache")
	}
}
