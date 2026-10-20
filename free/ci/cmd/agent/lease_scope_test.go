package main

// Purpose: the agent runs a lease under netscope.Effective(class, declared scope) and refuses what it forbids (D-1331).

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func scopeLease(trust model.TrustClass, network model.NetworkScope) protocol.Lease {
	return protocol.Lease{LeaseID: "l1", AttemptID: "a-scope", Epoch: 1, Trust: trust, Network: network, Job: model.Job{Kind: "static", Run: []string{"true"}},
		Checkout: protocol.Checkout{Revision: rev40}}
}

func TestLeaseScope(t *testing.T) {
	for _, tc := range []struct {
		trust   model.TrustClass
		network model.NetworkScope
		want    model.NetworkScope
		refused bool
	}{
		{"owner", "", "lan", false}, {"internal", "", "lan", false}, {"collaborator", "", "internet", false},
		{"untrusted", "", "restricted", false}, {"", "", "restricted", false}, {"bogus", "", "restricted", false},
		{"untrusted", "internet", "internet", false}, {"owner", "none", "none", false}, {"owner", "internet", "internet", false},
		{"untrusted", "lan", "", true}, {"untrusted", "privileged", "", true}, {"", "lan", "", true}, {"owner", "wan", "", true},
	} {
		got, err := jobScope(leaseSpec(scopeLease(tc.trust, tc.network), "c", ""))
		if tc.refused != (err != nil) || got != tc.want {
			t.Errorf("trust %q network %q: scope %q err %v, want %q refused=%v", tc.trust, tc.network, got, err, tc.want, tc.refused)
		}
	}
}

func TestExecWithoutATrustClassKeepsTheDefault(t *testing.T) {
	if got, err := jobScope(execSpec{}); got != "" || err != nil {
		t.Fatalf("scope %q err %v", got, err)
	}
}

// TestLeaseRunnerRefusesLANForUntrusted proves the refusal ends the attempt before anything runs.
func TestLeaseRunnerRefusesLANForUntrusted(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	res, err := leaseRunner{coordinator: "c"}.Run(context.Background(), session.JobRun{Lease: scopeLease("untrusted", "lan")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome.State != "failed" || res.Outcome.FailureClass != "infra" {
		t.Fatalf("outcome %+v, want failed/infra", res.Outcome)
	}
	ar, _, err := execute(context.Background(), leaseSpec(scopeLease("untrusted", "lan"), "c", ""), jobInputs{})
	var ce *ciexec.Error
	if err != nil || !errors.As(ar.Err, &ce) || ce.Code != "E679" || ar.Attempt.State != "failed" {
		t.Fatalf("execute: %+v err %v, want a failed attempt with E679", ar, err)
	}
}

// TestInvariant_I07 points I07 (untrusted never gets LAN) at the agent: whatever class and scope a lease
// names, an untrusted job's scope is never lan or privileged.
func TestInvariant_I07(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		scope, err := jobScope(leaseSpec(scopeLease(w.Trust, w.Network), "c", ""))
		if w.Trust != "untrusted" && w.Trust != "" {
			return nil
		}
		lan := w.Network == "lan" || w.Network == "privileged"
		switch {
		case lan && err == nil:
			return &i07Error{"an untrusted lease declaring " + string(w.Network) + " was not refused"}
		case scope == "lan" || scope == "privileged":
			return &i07Error{"an untrusted lease runs under " + string(scope)}
		}
		return nil
	})
}

type i07Error struct{ msg string }

func (e *i07Error) Error() string { return "I07: " + e.msg }
