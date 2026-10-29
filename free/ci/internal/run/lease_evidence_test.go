package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// D-1299: a lease the run's adapter delivers carries the attempt's recorded facts (run, repository id derived
// as the verifier derives it, source trust, policy digest, selection mode, job name), so the node can name them
// in the evidence it signs. An attempt with no recorded lease holder gets no context.
func TestDeliveredLeaseCarriesTheAttemptsEvidenceContext(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p := store.PipelineRow{ID: "p1", Project: "acme/widgets", Revision: "0123456789abcdef0123456789abcdef01234567", Trigger: "local", SourceTrust: "collaborator",
		PrivacyZone: "local-only", PolicyDigest: "policy-1", InputDigest: "i", SelectionMode: "affected", Status: "queued"}
	for _, f := range []func() error{
		func() error { return st.CreatePipeline(ctx, p) },
		func() error {
			return st.CreateJob(ctx, store.JobRow{ID: "j1", PipelineID: "p1", Name: "unit", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"})
		},
		func() error { return st.CreateAttempt(ctx, store.AttemptRow{ID: "a1", JobID: "j1", N: 1}, "") },
		func() error {
			return st.CreateJob(ctx, store.JobRow{ID: "j2", PipelineID: "p1", Name: "lint", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"})
		},
		func() error { return st.CreateAttempt(ctx, store.AttemptRow{ID: "a2", JobID: "j2", N: 1}, "") },
	} {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lease.New(st, nil, lease.Config{}).Grant(ctx, "host", store.Demand{AttemptID: "a1", LeaseID: "lease-a1", RunnerID: "x1", CPU: 1, MemMB: 1,
		CapacityCPU: 100, CapacityMemMB: 100, TTL: time.Minute}, 1); err != nil {
		t.Fatal(err)
	}
	probe := &probeDriver{kind: fmt.Sprintf("evidenceprobe%d", time.Now().UnixNano())}
	if err := transport.Register(probe); err != nil {
		t.Fatal(err)
	}
	var n model.Capability
	n.Identity.ID, n.Identity.Transport = "x1", probe.kind
	adapter := newTransportAdapter(newRemoteSink(st), nil, "")
	sum := sha256.Sum256([]byte("acme/widgets"))
	want := protocol.LeaseEvidence{RunID: "p1", RepoID: hex.EncodeToString(sum[:8]), SourceTrust: "collaborator", PolicyDigest: "policy-1", SelectionMode: "affected", Job: "unit"}
	for _, c := range []struct {
		attempt string
		want    *protocol.LeaseEvidence
	}{{"a1", &want}, {"a2", nil}} {
		d := deliveryFor(n, model.Job{Kind: "test", Run: []string{"true"}})
		d.AttemptID, d.Trust = c.attempt, "collaborator"
		if err := adapter.Deliver(ctx, d); err != nil {
			t.Fatal(err)
		}
		probe.mu.Lock()
		got := probe.lease.Evidence
		probe.mu.Unlock()
		if (got == nil) != (c.want == nil) || got != nil && *got != *c.want {
			t.Fatalf("attempt %s: lease evidence %+v, want %+v", c.attempt, got, c.want)
		}
	}
}
