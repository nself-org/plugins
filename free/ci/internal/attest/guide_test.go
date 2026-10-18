package attest

// Purpose: the CI trust guide (D-0969) stays in step with the code: the page
// exists, names every stable reason key, and covers the concepts a reader
// needs to judge evidence.
// Inputs:  .github/wiki/guides/ci/Trust-Security-and-Privacy.md.
// Outputs: test failures naming the missing key or topic.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const guidePath = "../../../../.github/wiki/guides/ci/Trust-Security-and-Privacy.md"

func TestTrustGuideCoversEveryReasonKey(t *testing.T) {
	raw, err := os.ReadFile(filepath.FromSlash(guidePath))
	if err != nil {
		t.Fatalf("trust guide missing: %v", err)
	}
	doc := string(raw)
	for _, r := range reasonOrder {
		if !strings.Contains(doc, "`"+string(r)+"`") {
			t.Errorf("guide does not document reason key %s", r)
		}
	}
	for _, topic := range []string{"GateAccept", "E685", "freshness", "publication profile", "evidence binding", "signed-remote", "self-reported", "coordinator-recorded"} {
		if !strings.Contains(strings.ToLower(doc), strings.ToLower(topic)) {
			t.Errorf("guide does not cover %q", topic)
		}
	}
}

func TestReasonKeysAreAllInReasonOrder(t *testing.T) {
	// the guide test above walks reasonOrder; keep it complete
	all := []Reason{ReasonStale, ReasonUnbound, ReasonAdhoc, ReasonRepoMismatch, ReasonPolicyMismatch, ReasonJobsMismatch,
		ReasonTrustBelowMin, ReasonSelfReportedNotAdmitted, ReasonSignatureUnverifiable, ReasonRevisionMismatch,
		ReasonSelectionPartial, ReasonNotPassing, ReasonInvalid}
	in := map[Reason]bool{}
	for _, r := range reasonOrder {
		in[r] = true
	}
	for _, r := range all {
		if !in[r] {
			t.Errorf("%s missing from reasonOrder", r)
		}
	}
}
