package protocol

// Purpose: the lease carries the attempt's trust class and declared network scope to the agent (D-1331).
// A valid pair survives the codec; an unknown class or scope is refused rather than read as a default.

import (
	"bytes"
	"testing"
)

func leaseFrame(l Lease) Message {
	e := l.Epoch
	return Message{V: 1, Type: "lease", Seq: 1, Epoch: &e, LeaseID: l.LeaseID, Body: l}
}

func TestLeaseCarriesTrustAndNetwork(t *testing.T) {
	base := Lease{LeaseID: "l1", AttemptID: "a1", Epoch: 2, Token: "opaque", CompatMode: Compat15, Secrets: []SecretRef{}, ExpiresAt: "2026-10-08T20:00:00Z"}
	good := base
	good.Trust, good.Network = "untrusted", "internet"
	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(leaseFrame(good)); err != nil {
		t.Fatal(err)
	}
	dec := NewDecoder(&buf, CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true})
	m, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}
	got := m.Body.(*Lease)
	if got.Trust != "untrusted" || got.Network != "internet" {
		t.Fatalf("trust %q network %q after the codec", got.Trust, got.Network)
	}
	for name, mut := range map[string]func(*Lease){"trust": func(l *Lease) { l.Trust = "root" }, "network": func(l *Lease) { l.Network = "wan" }} {
		bad := base
		mut(&bad)
		var out bytes.Buffer
		if err := NewEncoder(&out).Encode(leaseFrame(bad)); err != nil {
			continue // refused at encode: fine
		}
		if _, err := NewDecoder(&out, CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}).Decode(); err == nil {
			t.Errorf("a lease with an unknown %s was accepted", name)
		}
	}
}
