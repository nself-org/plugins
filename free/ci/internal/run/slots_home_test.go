package run

import (
	"os"
	"path/filepath"
	"testing"
)

// ciHome must return the actual CI state directory, not the parent of the slot
// namespace. On Linux the slot dir is an abstract socket path (@nself-ci/...),
// so deriving the home from it would point hygiene at the wrong location and
// skip workspace cleanup. This test pins the contract for all platforms.
func TestCiHomeIsIndependentOfSlotNamespace(t *testing.T) {
	explicit := t.TempDir()
	if got := ciHome(explicit); got != explicit {
		t.Fatalf("ciHome with explicit home: got %q, want %q", got, explicit)
	}

	envHome := t.TempDir()
	t.Setenv("NSELF_CI_HOME", envHome)
	if got := ciHome(""); got != envHome {
		t.Fatalf("ciHome from NSELF_CI_HOME: got %q, want %q", got, envHome)
	}

	t.Setenv("NSELF_CI_HOME", "")
	user, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(user, ".nself", "ci")
	if got := ciHome(""); got != want {
		t.Fatalf("ciHome default: got %q, want %q", got, want)
	}
}
