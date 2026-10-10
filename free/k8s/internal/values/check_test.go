// Purpose: tests for the parity check (Check), the writer's file modes, and
// the image splitter.
//
// Inputs: testdata/full.
//
// Outputs: test results.
//
// Constraints: no docker.
package values

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fullValuesBytes(t *testing.T) ([]byte, *Model) {
	t.Helper()
	m, v, _ := mapFixture(t, "full")
	b, err := MarshalValues(v)
	if err != nil {
		t.Fatal(err)
	}
	return b, m
}

func TestCheckCleanOnFixture(t *testing.T) {
	b, m := fullValuesBytes(t)
	diffs, err := Check(b, m)
	if err != nil || len(diffs) != 0 {
		t.Fatalf("diffs=%v err=%v", diffs, err)
	}
}

func TestCheckNamesAddedService(t *testing.T) {
	b, m := fullValuesBytes(t)
	m.Services["brand-new"] = ComposeSvc{Image: "busybox:1.36"}
	diffs, _ := Check(b, m)
	if len(diffs) != 1 || diffs[0].Service != "brand-new" || diffs[0].Kind != DiffMissing {
		t.Fatalf("diffs = %v", diffs)
	}
}

func TestCheckNamesRemovedService(t *testing.T) {
	b, m := fullValuesBytes(t)
	delete(m.Services, "mailpit")
	diffs, _ := Check(b, m)
	if len(diffs) != 1 || diffs[0].Service != "mailpit" || diffs[0].Kind != DiffExtra {
		t.Fatalf("diffs = %v", diffs)
	}
}

func TestCheckNamesChangedTag(t *testing.T) {
	b, m := fullValuesBytes(t)
	h := m.Services["hasura"]
	h.Image = "hasura/graphql-engine:v9.9.9"
	m.Services["hasura"] = h
	diffs, _ := Check(b, m)
	if len(diffs) != 1 || diffs[0].Service != "hasura" || diffs[0].Kind != DiffDiffers ||
		!strings.Contains(diffs[0].Detail, "v9.9.9") {
		t.Fatalf("diffs = %v", diffs)
	}
}

func TestCheckUnsupportedCountsAsPresent(t *testing.T) {
	b, m := fullValuesBytes(t)
	delete(m.Services, "worker-one")
	diffs, _ := Check(b, m)
	if len(diffs) != 1 || diffs[0].Service != "worker-one" || diffs[0].Kind != DiffExtra {
		t.Fatalf("diffs = %v", diffs)
	}
}

func TestCheckRejectsGarbage(t *testing.T) {
	_, m := fullValuesBytes(t)
	if _, err := Check([]byte("services: [1, 2"), m); err == nil {
		t.Fatal("want an error for invalid yaml")
	}
}

func TestWriteModes(t *testing.T) {
	_, v, s := mapFixture(t, "full")
	dir := filepath.Join(t.TempDir(), "k8s")
	// An old, looser secrets file must be tightened.
	mustWrite(t, filepath.Join(dir, SecretsFile), "old")
	if err := os.Chmod(filepath.Join(dir, SecretsFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, v, s); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, SecretsFile))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("secrets mode = %o, want 600", st.Mode().Perm())
	}
	b, _ := os.ReadFile(filepath.Join(dir, ValuesFile))
	if !strings.HasPrefix(string(b), GeneratedMarker) {
		t.Errorf("values.yaml lacks the GENERATED marker")
	}
	again := filepath.Join(t.TempDir(), "k8s")
	if err := Write(again, v, s); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(filepath.Join(again, ValuesFile))
	if string(b) != string(b2) {
		t.Error("output is not byte-stable")
	}
}

func TestSplitImage(t *testing.T) {
	cases := []struct{ in, repo, tag, digest string }{
		{"postgres:16-alpine", "postgres", "16-alpine", ""},
		{"nginx", "nginx", "latest", ""},
		{"localhost:5000/app:2", "localhost:5000/app", "2", ""},
		{"localhost:5000/app", "localhost:5000/app", "latest", ""},
		{"ghcr.io/o/i:1@sha256:abc", "ghcr.io/o/i", "1", "sha256:abc"},
		{"ghcr.io/o/i@sha256:abc", "ghcr.io/o/i", "", "sha256:abc"},
	}
	for _, c := range cases {
		r, tg, d := SplitImage(c.in)
		if r != c.repo || tg != c.tag || d != c.digest {
			t.Errorf("SplitImage(%q) = %q %q %q", c.in, r, tg, d)
		}
	}
}
