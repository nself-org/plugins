//go:build security

package redteam

// Purpose: red team group C, pairing and node key identity (TM-39, TM-20, TM-21). The attacker races
// the single-use pairing code, guesses it to lock the offer, presents a spelling of the code, pins a
// public key another node already holds, and sends terminal control sequences through the fields a
// joining node chooses (os, arch) toward the admin who must compare a fingerprint.
// Inputs:  a real store and CA in t.TempDir, the production pairing code helpers and approver prompt.
// Outputs: pass (the control held) or fail (the attack worked).
// Constraints: loopback and in-process only; clocks are injected.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestC39CodeIsSingleUseUnderRace: sixteen simultaneous redemptions of one code yield exactly one OK,
// a spelling variant of the code hits the same offer, and enough wrong guesses lock it for good.
func TestC39CodeIsSingleUseUnderRace(t *testing.T) {
	r := cNewRig(t)
	ctx := context.Background()
	code, err := pairing.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	put := func(id, c string) {
		o := store.PKIOffer{ID: id, CodeHash: pairing.HashCode(c), CertTTL: int64(time.Hour), ExpiresAt: r.now.Add(time.Hour).UnixNano()}
		if err := r.st.PKIOfferPut(ctx, o); err != nil {
			t.Fatal(err)
		}
	}
	put("o1", code)
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := map[store.CodeClaim]int{}
	spell := []string{code, strings.ToLower(code), code[:5] + "-" + code[5:], " " + code + " ", code[:3] + " " + code[3:]}
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, res, err := r.st.PKIOfferClaim(ctx, pairing.HashCode(spell[i%len(spell)]), false)
			if err != nil {
				t.Errorf("claim: %v", err)
			}
			mu.Lock()
			results[res]++
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if results[store.CodeOK] != 1 {
		t.Errorf("a single-use code was redeemed %d times: %v", results[store.CodeOK], results)
	}
	// guessing locks a fresh offer; the right code is then refused too
	code2, _ := pairing.GenerateCode()
	put("o2", code2)
	for i := 0; i < store.MaxCodeAttempts; i++ {
		wrong, _ := pairing.GenerateCode()
		if _, res, _ := r.st.PKIOfferClaim(ctx, pairing.HashCode(wrong), false); res == store.CodeOK {
			t.Fatal("a random guess redeemed an offer")
		}
	}
	if _, res, _ := r.st.PKIOfferClaim(ctx, pairing.HashCode(code2), false); res == store.CodeOK {
		t.Error("the right code redeemed an offer after the guess limit")
	}
	for _, bad := range []string{"", "short", strings.Repeat("A", 9), strings.Repeat("A", 11), "0123456789", "ABCDEFGHI\x00", "ABCDEFGH\u0000I"} {
		if pairing.ValidShape(bad) {
			t.Errorf("code shape %q is valid", bad)
		}
	}
}

// TestC39ReusedPublicKeyIsRefused (TM-20, TM-39): a node key already pinned to node a must not be pinned
// to node b, neither by the enrollment pin nor by the first-hello pin (the evidence verifier, the
// revocation set and the quarantine all key on the key id).
func TestC39ReusedPublicKeyIsRefused(t *testing.T) {
	if !openFinding(t, "C03") {
		return
	}
	r := cNewRig(t)
	ctx := context.Background()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := r.ca.PinNodeKey(ctx, "node-a", pub); err != nil {
		t.Fatal(err)
	}
	if err := r.ca.PinNodeKey(ctx, "node-b", pub); err == nil {
		t.Error("PinNodeKey pinned node a's public key to node b")
	}
	if err := r.ca.CheckNodeKey(ctx, "node-c", pub); err == nil {
		t.Error("the first hello of node c pinned node a's public key")
	}
}

// TestC39PromptCannotInjectTerminalControls: the os and arch a node reports reach the admin's terminal;
// C1 controls (CSI as U+009B), bidi overrides and C0 controls must not.
func TestC39PromptCannotInjectTerminalControls(t *testing.T) {
	var out bytes.Buffer
	ap := &pairing.PromptApprover{In: bufio.NewReader(strings.NewReader("\n")), Out: &out}
	evil := "linux\u009b2J‮evil⁦x​\x1b[2J\x07"
	_, err := ap.Approve(context.Background(), pairing.ApprovalRequest{OfferID: "o", NodeID: "n", NodeName: "n", OS: evil, Arch: evil, SourceAddr: evil, Label: evil,
		CSRFingerprint: "SHA256:abc", NodeKeyID: "0123456789abcdef", CertTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out.String() {
		switch {
		case r == '\n':
		case r < 0x20 || r == 0x7f, r >= 0x80 && r <= 0x9f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200b:
			if !openFinding(t, "C02") {
				return
			}
			t.Fatalf("the admin prompt printed control rune %U from a node-chosen field", r)
		}
	}
	if !utf8.ValidString(out.String()) {
		t.Error("the prompt printed invalid UTF-8")
	}
}

// TestC39ApprovalNeedsTheExactFingerprint: a typed fingerprint that differs in case, prefix, length or
// padding is a denial; Equal on two empty strings is true, so the denial on empty input must come from the caller.
func TestC39ApprovalNeedsTheExactFingerprint(t *testing.T) {
	fp := pki.SPKIFingerprint([]byte("a public key"))
	// positive control: the exact fingerprint (then four empty answers for the trust fields) approves, for a
	// new node and for an existing one, so every denial below is a decision and not a prompt that always denies.
	for _, existing := range []bool{false, true} {
		ap := &pairing.PromptApprover{In: bufio.NewReader(strings.NewReader(fp + "\n\n\n\n\n")), Out: &bytes.Buffer{}}
		dec, err := ap.Approve(context.Background(), pairing.ApprovalRequest{OfferID: "o", NodeID: "n", NodeName: "n", CSRFingerprint: fp, NodeKeyID: "0123456789abcdef", CertTTL: time.Hour, Existing: existing})
		if err != nil || !dec.Approve {
			t.Fatalf("control: the exact fingerprint (existing=%v) was not approved: approve=%v err=%v", existing, dec.Approve, err)
		}
	}
	for _, typed := range []string{"", strings.ToLower(fp), strings.TrimPrefix(fp, "SHA256:"), fp[:len(fp)-1], fp + "=", fp + "x", "SHA256:" + strings.ToUpper(fp[7:]), "  "} {
		var out bytes.Buffer
		ap := &pairing.PromptApprover{In: bufio.NewReader(strings.NewReader(typed + "\n")), Out: &out}
		dec, err := ap.Approve(context.Background(), pairing.ApprovalRequest{OfferID: "o", NodeID: "n", NodeName: "n", CSRFingerprint: fp, NodeKeyID: "0123456789abcdef", CertTTL: time.Hour})
		if err != nil || dec.Approve {
			t.Errorf("typed %q: approve=%v err=%v", typed, dec.Approve, err)
		}
	}
	if !pki.Equal("", "") {
		t.Log("pki.Equal of two empty fingerprints is false")
	} else {
		var out bytes.Buffer
		ap := &pairing.PromptApprover{In: bufio.NewReader(strings.NewReader("\n")), Out: &out}
		dec, _ := ap.Approve(context.Background(), pairing.ApprovalRequest{OfferID: "o", NodeID: "n", NodeName: "n", CSRFingerprint: "", CertTTL: time.Hour})
		if dec.Approve {
			t.Error("an empty node fingerprint was approved by an empty answer")
		}
	}
	if d := pairing.DefaultTrust(); len(d.SecretClasses) != 1 || d.SecretClasses[0] != "none" {
		t.Errorf("the default trust grants secrets: %+v", d)
	} else {
		for _, a := range d.Accepts {
			if a == "untrusted" || a == "collaborator" {
				t.Errorf("the default trust accepts %s work", a)
			}
		}
	}
}
