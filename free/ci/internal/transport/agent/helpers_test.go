package agent

// Purpose: shared fixtures of the pull-carrier tests: a real SQLite store, a real CA and token keys, a lease
//          engine and a seeded running attempt.
// Inputs:  none. Outputs: helpers only.
// Constraints: fake synthetic values only; the CA clock is shifted by the test, never slept on.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type rig struct {
	t      *testing.T
	dir    string
	st     *store.Store
	ca     *pki.CA
	tokens *pki.Tokens
	eng    *lease.Engine
	skew   atomic.Int64 // nanoseconds added to the CA clock
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, dir: t.TempDir()}
	st, err := store.Open(filepath.Join(r.dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r.st = st
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: r.dir, Now: func() time.Time { return time.Now().Add(time.Duration(r.skew.Load())) }})
	if err != nil {
		t.Fatal(err)
	}
	r.ca = ca
	if r.tokens, err = ca.Tokens(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.eng = lease.New(st, nil, lease.Config{})
	return r
}

// reopen closes the store and opens it again: a coordinator restart.
func (r *rig) reopen() {
	r.t.Helper()
	if err := r.st.Close(); err != nil {
		r.t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(r.dir, "state.db"), store.Options{})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = st.Close() })
	r.st = st
	r.eng = lease.New(st, nil, lease.Config{})
}

// seedLease creates one running attempt with a granted lease held by runner; it returns the lease id, the
// attempt id and the epoch.
func (r *rig) seedLease(n, runner string) (leaseID, attemptID string, epoch uint64) {
	r.t.Helper()
	ctx := context.Background()
	p := store.PipelineRow{ID: "p" + n, Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	steps := []func() error{
		func() error { return r.st.CreatePipeline(ctx, p) },
		func() error {
			return r.st.CreateJob(ctx, store.JobRow{ID: "j" + n, PipelineID: p.ID, Name: n, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"})
		},
		func() error { return r.st.CreateAttempt(ctx, store.AttemptRow{ID: "a" + n, JobID: "j" + n, N: 1}, "") },
	}
	for _, f := range steps {
		if err := f(); err != nil {
			r.t.Fatal(err)
		}
	}
	d := store.Demand{AttemptID: "a" + n, LeaseID: "l" + n, RunnerID: runner, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: time.Minute}
	fence, err := r.eng.Grant(ctx, "host", d, 1)
	if err != nil {
		r.t.Fatal(err)
	}
	_, aep, err := r.st.AttemptStateEpoch(ctx, "a"+n)
	if err != nil {
		r.t.Fatal(err)
	}
	if err = r.st.Transition(ctx, "a"+n, "leased", "running", store.TransitionDetail{Epoch: aep}); err != nil {
		r.t.Fatal(err)
	}
	return "l" + n, "a" + n, uint64(fence)
}

// mint signs a token for the attempt on the CA clock.
func (r *rig) mint(jti, runner, attemptID string, epoch uint64) string {
	r.t.Helper()
	tok, err := r.tokens.Mint(pki.TokenClaims{JTI: jti, JobID: "j", AttemptID: attemptID, Revision: "r", RunnerID: runner, Epoch: int64(epoch)}, time.Minute)
	if err != nil {
		r.t.Fatal(err)
	}
	return tok
}

func (r *rig) gate() *Gate { return &Gate{Tokens: r.tokens, Bindings: r.st, Leases: r.st, Certs: r.st} }

// issueCert records a node certificate for node with the CA and returns its serial; each one is issued after the last.
func (r *rig) issueCert(node string) string {
	r.t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		r.t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		r.t.Fatal(err)
	}
	is, err := r.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: node})
	if err != nil {
		r.t.Fatal(err)
	}
	return is.Serial
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func transportHeader() transport.BlobHeader { return transport.BlobHeader{Stream: "artifact"} }

func ctxBG() context.Context { return context.Background() }
