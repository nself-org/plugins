//go:build security

package redteam

// Purpose: shared helpers of the P7-TRUST-18 red team suite. Each file in this package is one
// attack group from hq architecture/ci-threat-model.md; a test asserts the SECURE outcome, so a
// passing test means the attack failed (the control held) and a failing test means it worked.
// Inputs:  none. Outputs: openFinding, fakeSecret.
// Constraints: local fixtures only (loopback, temp dirs, in-process fakes), no real host, no
//          docker, no real secret. Every value is synthetic.

import (
	"os"
	"testing"
)

// fakeSecret is the synthetic secret value every attacker fixture tries to exfiltrate. It is
// obviously fake and long enough to be registered with a redactor (more than 6 bytes).
const fakeSecret = "redteam-FAKE-secret-value-0123456789"

// openFinding marks a test whose attack currently SUCCEEDS (a recorded finding, id like F07). By
// default the test only logs the finding and returns, so the suite stays green while the finding
// is open and the review file carries it; with NSELF_REDTEAM_OPEN=1 the assertions run and fail
// until the owning Ticket fixes the control. When the fix lands, delete the openFinding call so
// the test becomes a regular regression test.
// Constraints: never used for a control that holds; a critical or high finding must not stay open
//
//	at Epic close (the review file verify line), so those are fixed, not parked here.
func openFinding(t *testing.T, id string) bool {
	t.Helper()
	if os.Getenv("NSELF_REDTEAM_OPEN") == "1" {
		return true
	}
	t.Logf("open finding %s: attack succeeds today; assertions run with NSELF_REDTEAM_OPEN=1", id)
	return false
}
