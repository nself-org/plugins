package coordinator

// Purpose: the decline answer of the Ingress registry (P7-HOST-16, review F4): an Accept error that
// has a DeclineReason() method is a verified request that must not enqueue, answered 200 with
// accepted:false and the reason, never 403. Any other error stays 403 (ingress_test.go).
// Inputs:  a fake Ingress whose Accept returns a declining error.
// Outputs: pass or fail.
// Constraints: shares register/fakePost with ingress_test.go.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type declineErr struct{ reason string }

func (d declineErr) Error() string         { return "internal text " + d.reason }
func (d declineErr) DeclineReason() string { return d.reason }

func TestIngressDeclineIs200NotAcceptedAndStoresNothing(t *testing.T) {
	f := &fakeIngress{name: "zz-fake-f", err: fmt.Errorf("wrapped: %w", declineErr{reason: "no handler for fake"})}
	register(t, f)
	r := newRig(t, nil)
	w := fakePost(r, "zz-fake-f", []byte(`{}`))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"accepted":false`) || !strings.Contains(w.Body.String(), "no handler for fake") || queued(t, r.st) != 0 {
		t.Fatalf("decline = %d %s queued=%d", w.Code, w.Body, queued(t, r.st))
	}
	if strings.Contains(w.Body.String(), "internal text") {
		t.Fatal("the error text reached the answer; only the reason may")
	}
	f.err = errors.New("plain failure")
	if w = fakePost(r, "zz-fake-f", []byte(`{}`)); w.Code != 403 {
		t.Fatalf("a plain Accept error = %d, want 403", w.Code)
	}
}
