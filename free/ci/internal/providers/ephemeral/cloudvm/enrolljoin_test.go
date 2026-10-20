package cloudvm_test

// Purpose: the scripted node of the enrollment tests: it reads the pairing code and the root
// fingerprint out of the user data the fake nself received (what the VM's cloud-init would do), and
// joins through the real enrollment listener of `ci serve` (pairing.ServeWhileOffers).
// Inputs:  the rig. Outputs: join results. Constraints: loopback only, synthetic keys.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

var fpArg = regexp.MustCompile(`--expect-fingerprint '([^']+)'`)

// userDataOf returns the pairing code and root fingerprint the VM of an intent was given.
func (r *rig) userDataOf(token string) (code, fp string) {
	r.t.Helper()
	for _, c := range r.br.callsOf("provision") {
		if !has(c.Argv, "intent="+token) {
			continue
		}
		p := parse(r.t, c.UserData)
		m := fpArg.FindStringSubmatch(p.script)
		if m == nil {
			r.t.Fatal("no fingerprint in the join script")
		}
		return strings.TrimSpace(p.codeFile), m[1]
	}
	r.t.Fatalf("no provision call carried intent %s", token)
	return "", ""
}

// serve runs the enrollment listener the way `ci serve` does and returns the enroll URL and a stop func.
func (r *rig) serve() (url string, stop func()) {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.pair.ServeWhileOffers(ctx, r.enrollAdr, "https://127.0.0.1") }()
	stop = func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				r.t.Errorf("ServeWhileOffers: %v", err)
			}
		case <-time.After(3 * time.Second):
			r.t.Error("ServeWhileOffers did not return")
		}
	}
	r.t.Cleanup(stop)
	return r.enrollURL, stop
}

func isOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func eventually(t testing.TB, within time.Duration, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(within); ; time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
		if time.Now().After(end) {
			t.Fatalf("not within %s: %s", within, what)
		}
	}
}

// join enrolls a scripted node named name with the given code and fingerprint.
func (r *rig) join(code, fp, name string) (enroll.JoinResult, error) {
	r.t.Helper()
	r.clk.Advance(2 * time.Second) // earlier requests of this source never count against this one
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		r.t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return enroll.Join(ctx, enroll.JoinOptions{URL: r.enrollURL, ExpectFingerprint: fp, Name: name, OS: "linux", Arch: "amd64", MachineID: "machine-1", AgentVersion: "1.5.0",
		NodeKey: pub, NodeKeyID: pki.NodeKeyID(pub), NodeSign: func(m []byte) ([]byte, error) { return ed25519.Sign(priv, m), nil },
		Out: io.Discard, Now: r.clk.Now, Sleep: r.clk.Advance, Code: func() (string, error) { return code, nil }})
}
