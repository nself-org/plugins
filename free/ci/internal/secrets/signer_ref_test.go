package secrets

import "testing"

func TestParseSignerReservedName(t *testing.T) {
	const raw = "release/NSELF_CI_RELEASE_SIGNING_KEY"
	r, err := ParseSigner(raw)
	if err != nil || r.Raw != raw || r.Class != "release" || r.Name != "NSELF_CI_RELEASE_SIGNING_KEY" {
		t.Fatalf("ref=%+v err=%v", r, err)
	}
	if _, err := Parse(raw); err == nil {
		t.Fatal("job parser accepted signer ref")
	}
	for _, bad := range []string{"release/invalid", "unknown/KEY", "release/KEY/EXTRA", "environment/ci/KEY"} {
		if _, err := ParseSigner(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
