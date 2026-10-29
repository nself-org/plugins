package run

// Purpose: an attempt start registers the operator pseudonym key of its CI home (D-1198), so the
// builder ids and status text published after it are keyed.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

func TestAttemptStartRegistersThePseudonymKey(t *testing.T) {
	t.Cleanup(func() { _ = attest.UsePseudonymKey(nil); pseudonymHome.Store(nil) })
	pseudonymHome.Store(nil)
	home := t.TempDir()
	if err := (attestStoreHook{}).AttemptStart(context.Background(), AttemptInfo{Home: home}); err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(filepath.Join(home, attest.PseudonymKeyFile))
	if err != nil || len(key) != 32 {
		t.Fatalf("key file: %d bytes, %v", len(key), err)
	}
	facts := attest.RepoFacts{Visibility: attest.Public, RepoID: "local:abc"}
	_ = attest.UsePseudonymKey(nil)
	unkeyed := attest.ProfileFor(facts).RunnerID("host-1")
	_ = attest.UsePseudonymKey(key)
	want := attest.ProfileFor(facts).RunnerID("host-1")
	_ = attest.UsePseudonymKey(nil)
	pseudonymHome.Store(nil)
	if err = (attestStoreHook{}).AttemptStart(context.Background(), AttemptInfo{Home: home}); err != nil {
		t.Fatal(err)
	}
	if got := attest.ProfileFor(facts).RunnerID("host-1"); got != want || got == unkeyed || !bytes.HasPrefix([]byte(got), []byte("runner-")) {
		t.Fatalf("profile after attempt start: %q (keyed %q, unkeyed %q)", got, want, unkeyed)
	}
	// a key file of the wrong shape fails the attempt rather than publishing unkeyed
	bad := t.TempDir()
	if err = os.WriteFile(filepath.Join(bad, attest.PseudonymKeyFile), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	pseudonymHome.Store(nil)
	if err = (attestStoreHook{}).AttemptStart(context.Background(), AttemptInfo{Home: bad}); err == nil {
		t.Fatal("a bad key file was accepted")
	}
}
