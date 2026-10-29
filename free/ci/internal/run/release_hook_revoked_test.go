package run

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

type revokedLedger struct {
	attest.Ledger
	revoked bool
	err     error
}

func (l revokedLedger) Revoked(context.Context, string) (bool, error) { return l.revoked, l.err }

// TestReleaseRevokedFailsClosed: a ledger that is missing or fails must surface an
// error (which the release stash turns into a refusal), never "not revoked".
func TestReleaseRevokedFailsClosed(t *testing.T) {
	if _, err := releaseRevoked(nil, "ci-node-0123456789abcdef"); err == nil {
		t.Fatal("a missing ledger must be an error")
	}
	if _, err := releaseRevoked(revokedLedger{err: errors.New("db down")}, "k"); err == nil {
		t.Fatal("a failing ledger must be an error")
	}
	if ok, err := releaseRevoked(revokedLedger{revoked: true}, "k"); err != nil || !ok {
		t.Fatalf("revoked = %v, %v", ok, err)
	}
	if ok, err := releaseRevoked(revokedLedger{}, "k"); err != nil || ok {
		t.Fatalf("not revoked = %v, %v", ok, err)
	}
}
