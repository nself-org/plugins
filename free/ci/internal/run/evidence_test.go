package run

// Purpose: pins the trust class buildEvidence records for a run.
// Inputs:  evidenceInput values with and without a trigger source.
// Outputs: test results only.
// Constraints: no disk, no network.

import (
	"testing"
	"time"
)

// D-0971: an empty source must never be recorded as owner trust, which meets
// every gate minimum. It falls to the lowest class and the caller sets it.
func TestBuildEvidenceSourceTrustDefaultsToUntrusted(t *testing.T) {
	now := time.Now()
	for _, c := range []struct{ source, want string }{
		{"", "untrusted"},
		{"owner", "owner"},
		{"internal", "internal"},
	} {
		e, _, err := buildEvidence(evidenceInput{RunID: "r", Project: "p", Source: c.source, Start: now, End: now})
		if err != nil {
			t.Fatalf("source %q: %v", c.source, err)
		}
		if string(e.SourceTrust) != c.want {
			t.Errorf("source %q: SourceTrust = %q, want %q", c.source, e.SourceTrust, c.want)
		}
	}
}
