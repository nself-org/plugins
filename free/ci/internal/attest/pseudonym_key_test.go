package attest_test

// Purpose: the keyed publication pseudonyms (D-1198): an HMAC under the operator key replaces the
// unkeyed hash, so a dictionary of host names recovers nothing; the key file is created once, private,
// and a wrong file is refused.

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

func TestKeyedPseudonymsResistADictionary(t *testing.T) {
	t.Cleanup(func() { _ = attest.UsePseudonymKey(nil) })
	facts := attest.RepoFacts{Visibility: attest.Public, RepoID: "local:abc"}
	unkeyed := attest.ProfileFor(facts)
	if err := attest.UsePseudonymKey(bytes.Repeat([]byte{7}, 32)); err != nil {
		t.Fatal(err)
	}
	keyed := attest.ProfileFor(facts)
	// the attacker's dictionary: the unkeyed hash of every host name they can guess
	guess := unkeyed.RunnerID("build-host-01")
	if got := keyed.RunnerID("build-host-01"); got == guess || !strings.HasPrefix(got, "runner-") || got != keyed.RunnerID("build-host-01") {
		t.Fatalf("keyed %q vs dictionary %q", got, guess)
	}
	if keyed.CoordinatorID("x") == keyed.RunnerID("x") || keyed.RunnerID("x") == attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public, RepoID: "local:other"}).RunnerID("x") {
		t.Fatal("pseudonyms must differ by kind and by repo")
	}
	if err := attest.UsePseudonymKey(bytes.Repeat([]byte{8}, 32)); err != nil {
		t.Fatal(err)
	}
	if attest.ProfileFor(facts).RunnerID("build-host-01") == keyed.RunnerID("build-host-01") {
		t.Fatal("another operator key must give another pseudonym")
	}
	if attest.UsePseudonymKey([]byte("short")) == nil {
		t.Fatal("a short key was accepted")
	}
}

func TestLoadPseudonymKeyCreatesOnceAndRefusesAWrongFile(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ci")
	k1, err := attest.LoadPseudonymKey(home)
	if err != nil || len(k1) != 32 {
		t.Fatalf("create: %d bytes, %v", len(k1), err)
	}
	if k2, err := attest.LoadPseudonymKey(home); err != nil || !bytes.Equal(k1, k2) {
		t.Fatalf("a second load must return the same key: %v", err)
	}
	path := filepath.Join(home, attest.PseudonymKeyFile)
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	if err = os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = attest.LoadPseudonymKey(home); err == nil {
		t.Fatal("a file of the wrong size was accepted")
	}
	if runtime.GOOS != "windows" {
		if err = os.WriteFile(path, bytes.Repeat([]byte{1}, 32), 0o644); err != nil {
			t.Fatal(err)
		}
		if err = os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err = attest.LoadPseudonymKey(home); err == nil {
			t.Fatal("a group-readable key file was accepted")
		}
	}
	if _, err = attest.LoadPseudonymKey(""); err == nil {
		t.Fatal("an empty home was accepted")
	}
}

// Attempts of one run start together: concurrent first loads agree on one key and none sees a partial file.
func TestLoadPseudonymKeyConcurrentFirstUse(t *testing.T) {
	home := t.TempDir()
	const n = 16
	keys := make([][]byte, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); keys[i], errs[i] = attest.LoadPseudonymKey(home) }()
	}
	wg.Wait()
	for i := range keys {
		if errs[i] != nil || !bytes.Equal(keys[i], keys[0]) {
			t.Fatalf("load %d: %v", i, errs[i])
		}
	}
	if left, _ := filepath.Glob(filepath.Join(home, ".pseudonym-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}
