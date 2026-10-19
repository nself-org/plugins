package records

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// TestScanOverlapHoldsLongestRegisteredForm pins scan.go's contract: the longest
// form of the longest value redact.Register accepts fits the window overlap, so a
// value that straddles a window edge is still found.
func TestScanOverlapHoldsLongestRegisteredForm(t *testing.T) {
	if got := redact.MaxValueLen * redact.MaxFormExpansion; got > scanOverlap {
		t.Fatalf("longest form %d B exceeds the %d B scan overlap", got, scanOverlap)
	}
	if scanOverlap*2 > scanWindow {
		t.Fatalf("overlap %d must stay under half the window %d", scanOverlap, scanWindow)
	}
}
