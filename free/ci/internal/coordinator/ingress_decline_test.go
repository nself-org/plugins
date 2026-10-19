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

// An Ingress that holds per-repo secrets skips --allowed-repos only for a delivery that was verified with
// such a secret (facts.auth per-repo); a shared-secret or secretless delivery stays under the allowlist.
func TestIngressSecretHolderSkipsTheAllowlistOnlyForPerRepoAuth(t *testing.T) {
	outside := func(facts string) Delivery {
		return Delivery{Source: "fake", DeliveryID: "f-" + facts, Project: "x/outside", Revision: testSHA, Facts: []byte(`{"auth":"` + facts + `"}`)}
	}
	cases := []struct {
		name    string
		secrets bool
		auth    string
		want    int
	}{
		{"per-repo secret holder", true, "per-repo", 202},
		{"secret holder, shared secret", true, "shared", 403},
		{"secret holder, no auth (--insecure)", true, "none", 403},
		{"ingress without secrets, per-repo claim", false, "per-repo", 403},
	}
	for i, c := range cases {
		f := &fakeIngress{name: fmt.Sprintf("zz-fake-g%d", i), secrets: c.secrets, deliver: outside(c.auth)}
		register(t, f)
		r := newRig(t, nil)
		if w := fakePost(r, f.name, []byte(`{}`)); w.Code != c.want || (c.want == 403 && queued(t, r.st) != 0) {
			t.Errorf("%s: %d %s queued=%d, want %d", c.name, w.Code, w.Body, queued(t, r.st), c.want)
		}
	}
}
