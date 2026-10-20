package run

// Purpose: tests of the per-lease sink (remote_blob_hook.go): which deliveries
// get the artifact handler, that blobs, refs and the result reach the store
// through it, that the finalize hook then commits only what was delivered, and
// that every other stream and job keeps the old behaviour.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func shaOf(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func artifactJob(paths ...string) model.Job {
	j := model.Job{Kind: "build", Run: []string{"true"}}
	for _, p := range paths {
		j.Artifacts = append(j.Artifacts, model.ArtifactDecl{Path: p})
	}
	return j
}

func (e *remoteEnv) delivery(job model.Job) leaseFacts {
	return leaseFacts{AttemptID: e.id, LeaseID: e.lease, Epoch: e.epoch, Job: job}
}

func put(t *testing.T, s transport.Sink, data []byte) {
	t.Helper()
	w, err := s.Blob(context.Background(), transport.BlobHeader{Stream: "artifact", SHA256: shaOf(data), Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBindLeaseSinkOnlyChangesJobsThatDeclareOutputs(t *testing.T) {
	e := newRemoteEnv(t)
	e.sink.home = t.TempDir()
	if got := bindLeaseSink(e.sink, e.delivery(model.Job{Kind: "test", Run: []string{"true"}})); got != transport.Sink(e.sink) {
		t.Fatal("a job without artifacts got a bound sink")
	}
	hosted := e.delivery(artifactJob("out/a"))
	hosted.Hosted = true
	if got := bindLeaseSink(e.sink, hosted); got != transport.Sink(e.sink) {
		t.Fatal("a hosted lease got a bound sink")
	}
	e.sink.expect(e.id, testRev)
	if _, ok := bindLeaseSink(e.sink, e.delivery(artifactJob("out/a"))).(leaseSink); !ok {
		t.Fatal("a job that declares artifacts did not get the lease sink")
	}
	// no rendezvous for the attempt: the run does not await it; a job that declares outputs must not
	// be answered by the sink that drops them
	e2 := newRemoteEnv(t)
	got := bindLeaseSink(e2.sink, e2.delivery(artifactJob("out/a")))
	if _, ok := got.(refusedSink); !ok {
		t.Fatalf("an unexpected attempt got %T, want the refusing sink", got)
	}
	if err := got.ArtifactRef(context.Background(), protocol.ArtifactRef{LeaseID: e2.lease, Path: "out/a"}); err == nil {
		t.Fatal("the refusing sink accepted an artifact-ref")
	}
}

func TestLeaseSinkDeliversAndTheFinalizeHookCommits(t *testing.T) {
	ctx := context.Background()
	e := newRemoteEnv(t)
	e.sink.home = t.TempDir()
	e.sink.expect(e.id, testRev)
	job := artifactJob("out/*.txt")
	s := bindLeaseSink(e.sink, e.delivery(job))
	data := []byte("artifact bytes from a node")
	put(t, s, data)
	if err := s.ArtifactRef(ctx, protocol.ArtifactRef{LeaseID: e.lease, Path: "out/a.txt", SHA256: shaOf(data), Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if got := e.state(t); got != "running" {
		t.Fatalf("state %s after the first frame, want running", got)
	}
	if err := s.Result(ctx, protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: evJSON("pass")}); err != nil {
		t.Fatal(err)
	}
	if got := e.state(t); got != "finalizing" {
		t.Fatalf("state %s after the result, want finalizing", got)
	}
	_, aep, _ := e.st.AttemptStateEpoch(ctx, e.id)
	info := AttemptInfo{AttemptID: e.id, Job: job, Home: e.sink.home, Store: e.st}
	if err := artifactsOn.AttemptFinalize(ctx, info, aep); err != nil {
		t.Fatal(err)
	}
	recs, err := e.st.ArtifactList(ctx, records.Filter{Attempt: e.id})
	if err != nil || len(recs) != 1 || recs[0].State != records.StateCommitted || recs[0].Name != "out/a.txt" || recs[0].SHA256 != shaOf(data) {
		t.Fatalf("records = %+v, err %v", recs, err)
	}
}

func TestLeaseSinkNothingDeliveredFailsTheFinalize(t *testing.T) {
	ctx := context.Background()
	e := newRemoteEnv(t)
	e.sink.home = t.TempDir()
	e.sink.expect(e.id, testRev)
	job := artifactJob("out/a.txt")
	s := bindLeaseSink(e.sink, e.delivery(job))
	if err := s.Result(ctx, protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: evJSON("pass")}); err != nil {
		t.Fatal(err)
	}
	_, aep, _ := e.st.AttemptStateEpoch(ctx, e.id)
	err := artifactsOn.AttemptFinalize(ctx, AttemptInfo{AttemptID: e.id, Job: job, Home: e.sink.home, Store: e.st}, aep)
	if !records.IsReason(err, records.ReasonMissing) || !strings.Contains(err.Error(), "out/a.txt") {
		t.Fatalf("finalize = %v, want E712 missing for out/a.txt", err)
	}
}

func TestLeaseSinkKeepsTheOldBehaviourForOtherStreams(t *testing.T) {
	e := newRemoteEnv(t)
	e.sink.home = t.TempDir()
	e.sink.expect(e.id, testRev)
	s := bindLeaseSink(e.sink, e.delivery(artifactJob("out/a")))
	w, err := s.Blob(context.Background(), transport.BlobHeader{Stream: "cache", SHA256: shaOf([]byte("x")), Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := w.(*discardBlob); !ok {
		t.Fatalf("a cache blob went to %T, want the discarding writer", w)
	}
}

func TestLeaseSinkRefusesArtifactFramesAfterTheLeaseMovedOn(t *testing.T) {
	ctx := context.Background()
	e := newRemoteEnv(t)
	e.sink.home = t.TempDir()
	e.sink.expect(e.id, testRev)
	s := bindLeaseSink(e.sink, e.delivery(artifactJob("out/a")))
	// a newer incarnation took the lease: the engine fences the old epoch
	bound := e.delivery(artifactJob("out/a"))
	bound.Epoch = e.epoch + 1
	z := bindLeaseSink(e.sink, bound)
	data := []byte("zombie bytes")
	if _, err := z.Blob(ctx, transport.BlobHeader{Stream: "artifact", SHA256: shaOf(data), Size: int64(len(data))}); err == nil || !strings.Contains(err.Error(), "E714 stale_epoch") {
		t.Fatalf("zombie blob: %v, want E714 stale_epoch", err)
	}
	if err := z.ArtifactRef(ctx, protocol.ArtifactRef{LeaseID: e.lease, Path: "out/a", SHA256: shaOf(data), Size: int64(len(data))}); err == nil || !strings.Contains(err.Error(), "E714 stale_epoch") {
		t.Fatalf("zombie ref: %v", err)
	}
	put(t, s, []byte("the live lease still works"))
}

func TestRemoteOnlyArtifactJobsAreNotPinnedToTheCoordinatorHost(t *testing.T) {
	rs := &runState{c: &Coordinator{}, hooks: Hooks{}}
	job := artifactJob("out/a")
	if !rs.localOnly(job) {
		t.Fatal("an ordinary artifact job must stay on the coordinator host")
	}
	job.RemoteOnly = true
	if rs.localOnly(job) {
		t.Fatal("a remote-only artifact job must be placeable on a node")
	}
}
