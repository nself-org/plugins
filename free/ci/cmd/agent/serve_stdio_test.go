package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// checkoutLease bundles the head of a fresh repo and returns the lease for it.
func checkoutLease(t *testing.T, id string, run []string) (protocol.Lease, []byte) {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "marker.txt"), []byte("from-the-bundle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "marker.txt")
	git(t, repo, "commit", "-q", "-m", "one")
	rev := git(t, repo, "rev-parse", "HEAD")
	dest := filepath.Join(t.TempDir(), "c.bundle")
	digest, size, err := checkout.Create(context.Background(), nil, repo, rev, dest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Lease{LeaseID: id, AttemptID: "attempt-" + id, Epoch: 1, Token: "opaque-token-7f3a", CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "static", Run: run}, Checkout: protocol.Checkout{Revision: rev, BundleDigest: digest, Size: size},
		Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z"}, data
}

func sendBundle(p *stdioPeer, l protocol.Lease, data []byte) {
	for off := 0; off < len(data); off += 4096 {
		end := min(off+4096, len(data))
		p.send("chunk", protocol.Chunk{Stream: checkout.Stream, Offset: int64(off), Data: base64.StdEncoding.EncodeToString(data[off:end]), Final: end == len(data)}, l.Epoch, l.LeaseID)
	}
}

func resultOf(t *testing.T, p *stdioPeer) (*protocol.Result, string) {
	t.Helper()
	var logs []string
	for {
		m, err := p.dec.Decode()
		if err != nil {
			t.Fatal(err)
		}
		switch b := m.Body.(type) {
		case *protocol.Log:
			logs = append(logs, b.Text)
		case *protocol.Result:
			return b, strings.Join(logs, "\n")
		}
	}
}

func handshake(p *stdioPeer) {
	p.next("hello")
	p.send("welcome", protocol.Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s", HeartbeatMS: 60000, AgentTTLMS: 120000}, 0, "")
}

// TestServeStdioStagesTheCheckoutBeforeTheJob proves the bundle sent as chunk frames
// becomes the job's working tree at the leased revision.
func TestServeStdioStagesTheCheckoutBeforeTheJob(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	p := startServe(t, "--coordinator", "peer-1")
	handshake(p)
	l, data := checkoutLease(t, "co1", []string{"sh", "-c", "cat marker.txt; git rev-parse HEAD"})
	p.send("lease", l, l.Epoch, l.LeaseID)
	sendBundle(p, l, data)
	res, logs := resultOf(t, p)
	if !strings.Contains(logs, "from-the-bundle") || !strings.Contains(logs, l.Checkout.Revision) {
		t.Fatalf("job did not run in the staged checkout (result %v):\n%s", validEvidence(t, res.Evidence)["result"], logs)
	}
}

// TestServeStdioRefusesABundleWithTheWrongDigest proves a corrupted bundle fails the
// lease before any step runs.
func TestServeStdioRefusesABundleWithTheWrongDigest(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	p := startServe(t, "--coordinator", "peer-1")
	handshake(p)
	l, data := checkoutLease(t, "co2", []string{"sh", "-c", "echo STEP-RAN"})
	data[len(data)/2] ^= 0xff
	p.send("lease", l, l.Epoch, l.LeaseID)
	sendBundle(p, l, data)
	res, logs := resultOf(t, p)
	if strings.Contains(logs, "STEP-RAN") || strings.Contains(string(res.Evidence), `"result":"pass"`) {
		t.Fatalf("a bundle with a bad digest ran a step:\n%s\n%s", res.Evidence, logs)
	}
}
