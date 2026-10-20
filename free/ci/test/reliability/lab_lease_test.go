//go:build integration

package reliability

// Purpose: the driver-level harness: a lease made by hand (rows, grant, protocol lease) and delivered to a real
//          agent through the agent transport driver, for the cases the run loop cannot place (a release job, an
//          at-most-once job), where the lease engine's own rules are what is under test.
// Inputs:  a node index and a job shape.
// Outputs: the attempt and lease ids, and a recording sink the test reads.
// Constraints: tests only; the job is the fixture script.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

// recSink records what the driver hands to the run; a real run's sink would commit the result.
type recSink struct {
	mu      sync.Mutex
	lines   []string
	results []protocol.Result
}

func (s *recSink) Log(_ context.Context, l protocol.Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, strings.TrimRight(l.Text, "\n"))
	return nil
}
func (s *recSink) Result(_ context.Context, r protocol.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, r)
	return nil
}
func (*recSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (*recSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (*recSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, context.Canceled
}

func (s *recSink) nResults() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.results)
}

// handLease is a lease delivered by hand.
type handLease struct {
	attempt, lease string
	epoch          uint64
	sink           *recSink
	done           chan error
}

// deliver creates a pipeline, a job of the given kind and an attempt, grants its lease on node i and hands it
// to the node's agent through the driver, as the dispatcher would. idempotent false or kind release makes the
// engine send a lost lease to needs-operator instead of retrying it.
func (l *lab) deliver(i int, name string, kind model.JobKind, idempotent bool, spec jobSpec) *handLease {
	t := l.t
	t.Helper()
	ctx := context.Background()
	st := l.c.st
	sha := l.commit()
	p := store.PipelineRow{ID: "p-" + name, Project: l.root, Revision: sha, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err := st.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.JobRow{ID: "j-" + name, PipelineID: p.ID, Name: name, Kind: string(kind), Idempotent: idempotent, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	h := &handLease{attempt: "a-" + name, lease: "lease-a-" + name, sink: &recSink{}, done: make(chan error, 1)}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: h.attempt, JobID: "j-" + name, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	d := store.Demand{AttemptID: h.attempt, LeaseID: h.lease, RunnerID: l.ids[i], CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: 45 * time.Second}
	fence, err := l.c.eng.Grant(ctx, "host", d, l.c.incarnation())
	if err != nil {
		t.Fatal(err)
	}
	h.epoch = uint64(fence)
	n, err := st.NodeGet(ctx, l.ids[i])
	if err != nil {
		t.Fatal(err)
	}
	var capab model.Capability
	if err = json.Unmarshal(n.Capability, &capab); err != nil {
		t.Fatal(err)
	}
	job := spec.job()
	job.Kind = kind
	pl := protocol.Lease{LeaseID: h.lease, AttemptID: h.attempt, Epoch: h.epoch, CompatMode: protocol.Compat15, Job: job,
		Checkout: protocol.Checkout{Revision: sha}, Secrets: []protocol.SecretRef{}, ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}
	go func() { h.done <- agent.Default.Run(context.Background(), capab, pl, h.sink) }()
	return h
}

// state is the attempt's state in the store.
func (l *lab) attemptState(attempt string) string {
	l.t.Helper()
	s, _, err := l.c.st.AttemptStateEpoch(context.Background(), attempt)
	if err != nil {
		l.t.Fatal(err)
	}
	return string(s)
}
