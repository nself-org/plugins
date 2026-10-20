//go:build security

package redteam

// Purpose: red team group C, job token parts (TM-21), second half of the identity group.
// Inputs:  see the sibling file of the same group.
// Outputs: pass (the control held) or fail (the attack worked).
// Constraints: local fixtures only; see c_evidence_test.go header.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestC21JobTokenParts (TM-21): a job token with an edited claim, a swapped key id, a foreign signer,
// extra or padded parts or an expired exp is refused; exp equal to now is expired; minting beyond the cap is refused.
func TestC21JobTokenParts(t *testing.T) {
	r := cNewRig(t)
	tk, err := r.ca.Tokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now := r.now.Unix()
	tok, err := tk.Mint(pki.TokenClaims{JTI: "j1", JobID: "job", AttemptID: "att", Revision: "rev", RunnerID: "n1", Epoch: 3}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tk.Verify(tok); err != nil {
		t.Fatalf("control: %v", err)
	}
	p := strings.Split(tok, ".")
	enc := base64.RawURLEncoding.EncodeToString
	_, foreign, _ := ed25519.GenerateKey(rand.Reader)
	head := p[0] + "." + enc([]byte(`{"jti":"j1","job_id":"job","attempt_id":"att","revision":"rev","runner_id":"n9","epoch":3,"exp":`+cItoa(now+300)+`}`))
	forged := head + "." + enc(ed25519.Sign(foreign, []byte(head)))
	for name, v := range map[string]string{
		"edited claims, old signature": p[0] + "." + enc([]byte(`{"jti":"j1","job_id":"job","attempt_id":"att","revision":"rev","runner_id":"n9","epoch":3,"exp":`+cItoa(now+300)+`}`)) + "." + p[2],
		"foreign signer, real key id":  forged,
		"unknown key id":               enc([]byte("0000000000000000")) + "." + p[1] + "." + p[2],
		"four parts":                   tok + ".x",
		"two parts":                    p[0] + "." + p[1],
		"padded signature":             p[0] + "." + p[1] + "." + base64.URLEncoding.EncodeToString(cMustDecode(p[2])),
		"empty":                        "",
		"dots":                         "..",
		"alg none":                     enc([]byte(`{"alg":"none"}`)) + "." + p[1] + ".",
	} {
		if _, err := tk.Verify(v); err == nil {
			t.Errorf("%s: verified", name)
		}
	}
	r.now = r.now.Add(time.Minute + pki.TokenSlack) // exp == now
	if _, err := tk.Verify(tok); err == nil {
		t.Error("a token verified at its exp second")
	}
	r.now = time.Unix(1_800_000_000, 0)
	for _, c := range []struct {
		exp     int64
		timeout time.Duration
	}{{now + 361, time.Minute}, {now, time.Minute}, {-1, time.Minute}, {now + 10, -time.Hour}, {0, -time.Hour}, {1 << 62, time.Hour}} {
		if _, err := tk.Mint(pki.TokenClaims{JTI: "j", Exp: c.exp}, c.timeout); err == nil {
			t.Errorf("exp %d timeout %s was minted", c.exp, c.timeout)
		}
	}
}

func cMustDecode(s string) []byte { b, _ := base64.RawURLEncoding.DecodeString(s); return b }

func cItoa(n int64) string { return strconv.FormatInt(n, 10) }
