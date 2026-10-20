package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cr "github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func TestPrepareRemoteCacheBuildsPlanWithNodePlatform(t *testing.T) {
	ctx := context.Background()
	for _, invalidPolicy := range []bool{false, true} {
		name := "valid-policy"
		if invalidPolicy {
			name = "invalid-policy"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("NSELF_CI_CACHE_DISABLE", "")
			t.Setenv("NSELF_CI_ARTIFACT_MAX_FILE", "")
			if invalidPolicy {
				t.Setenv("NSELF_CI_ARTIFACT_MAX_FILE", "invalid")
			}
			e := newRemoteEnv(t)
			e.sink.home = t.TempDir()
			e.sink.expect(e.id, testRev)
			t.Cleanup(func() { remoteCaches.Delete(e.id) })
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var keys []string
			for _, platform := range []string{"linux/amd64", "linux/arm64"} {
				lease := protocol.Lease{LeaseID: e.lease, AttemptID: e.id, Epoch: uint64(e.epoch), Job: model.Job{
					Kind: "build", Isolation: "container", Container: &model.ContainerConfig{Image: "example.invalid/build@sha256:" + strings.Repeat("a", 64)},
				}}
				got := prepareCache(ctx, e.sink, &lease, root, "node", false, platform)
				if invalidPolicy {
					if got != transport.Sink(e.sink) || lease.Cache != nil {
						t.Fatalf("invalid policy advertised an unhandled cache plan: %T %+v", got, lease.Cache)
					}
					continue
				}
				if _, ok := got.(cacheLeaseSink); !ok {
					t.Fatalf("cache plan did not wrap delivery: %T", got)
				}
				if lease.Cache == nil || lease.Cache.Access != "rw" || len(lease.Cache.Save) != 1 || lease.Cache.Save[0].Spec != "buildx" {
					t.Fatalf("planned cache=%+v, want writable buildx save", lease.Cache)
				}
				save := lease.Cache.Save[0]
				if len(save.Dirs) != 1 || save.Dirs[0] != "buildx/layers" {
					t.Fatalf("planned cache dirs=%v", save.Dirs)
				}
				keys = append(keys, save.Key)
			}
			if !invalidPolicy && (keys[0] == keys[1] || !remoteCacheFinalize{}.Wants(AttemptInfo{AttemptID: e.id})) {
				t.Fatal("node platform did not partition cache keys or save was not registered")
			}
		})
	}
}

func TestRemoteCachePlatformUsesNodeFacts(t *testing.T) {
	var node model.Capability
	if remoteCachePlatform(node) != "" {
		t.Fatal("unknown node platform borrowed coordinator platform")
	}
	osName, arch := "linux", "amd64"
	node.Platform.OS.Value, node.Platform.Arch.Value = &osName, &arch
	if got := remoteCachePlatform(node); got != "linux/amd64" {
		t.Fatalf("node platform=%q", got)
	}
	arch = ""
	if remoteCachePlatform(node) != "" {
		t.Fatal("empty node architecture accepted")
	}
}

func TestPrepareRemoteCacheDisabledAndHosted(t *testing.T) {
	for _, mode := range []string{"hosted", "operator", "job", "job-env", "empty-root", "unknown-platform"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("NSELF_CI_CACHE_DISABLE", "")
			lease := &protocol.Lease{}
			root := t.TempDir()
			platform := "linux/amd64"
			sink := &remoteSink{}
			switch mode {
			case "operator":
				t.Setenv("NSELF_CI_CACHE_DISABLE", "1")
			case "job":
				lease.Job.Cache = &model.JobCacheConfig{Disable: true}
			case "job-env":
				lease.Job.Env = map[string]string{"NSELF_CI_CACHE_DISABLE": "1"}
			case "empty-root":
				root = ""
			case "unknown-platform":
				platform = ""
			}
			if got := prepareCache(context.Background(), sink, lease, root, "n1", mode == "hosted", platform); got != transport.Sink(sink) || lease.Cache != nil {
				t.Fatalf("disabled cache changed delivery: %T %+v", got, lease.Cache)
			}
		})
	}
}

func TestCacheLeaseSinkPreservesArtifacts(t *testing.T) {
	ctx := context.Background()
	e := newRemoteEnv(t)
	e.sink.home = t.TempDir()
	e.sink.expect(e.id, testRev)
	job := artifactJob("out/a")
	sink := cacheLeaseSink{Sink: bindLeaseSink(e.sink, e.delivery(job))}
	data := []byte("artifact with a cache wrapper")
	put(t, sink, data)
	if err := sink.ArtifactRef(ctx, protocol.ArtifactRef{LeaseID: e.lease, Path: "out/a", SHA256: shaOf(data), Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
}

type remoteCacheFinalizerFunc func(context.Context, time.Duration) error

func (f remoteCacheFinalizerFunc) Finalize(ctx context.Context, wait time.Duration) error {
	return f(ctx, wait)
}
func (f remoteCacheFinalizerFunc) Retract(context.Context) error { return nil }

func TestRemoteCacheFinalizeRegistrationAndDeadline(t *testing.T) {
	registered, cleanup := false, false
	for _, hook := range Registered.AttemptFinalizes {
		if _, ok := hook.(remoteCacheFinalize); ok {
			registered = true
		}
	}
	for _, hook := range Registered.AfterRuns {
		if _, ok := hook.(remoteCacheFinalize); ok {
			cleanup = true
		}
	}
	if !registered || !cleanup {
		t.Fatal("remote cache finalizer or cleanup was not registered")
	}
	id := "cache-finalize-deadline"
	hook := remoteCacheFinalize{}
	if hook.Wants(AttemptInfo{AttemptID: id}) {
		t.Fatal("unknown attempt requested finalization")
	}
	called := false
	remoteCaches.Store(id, &remoteCacheRecord{lease: remoteCacheFinalizerFunc(func(ctx context.Context, wait time.Duration) error {
		called = true
		deadline, ok := ctx.Deadline()
		if ctx.Err() != nil || !ok || time.Until(deadline) < 30*time.Second || wait != 30*time.Second {
			t.Fatalf("cache wait inherited short parent deadline: wait=%v deadline=%v err=%v", wait, deadline, ctx.Err())
		}
		return errors.New("E711 simulated cache refusal")
	})})
	t.Cleanup(func() { remoteCaches.Delete(id) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := hook.AttemptFinalize(ctx, AttemptInfo{AttemptID: id}, 1); err != nil || !called {
		t.Fatalf("cache failure was not best effort: %v called=%v", err, called)
	}
	if !hook.Wants(AttemptInfo{AttemptID: id}) {
		t.Fatal("session removed before settlement could retract it")
	}
}

type remoteCacheLedger struct {
	state     model.JobState
	retracted int
	err       error
}

func (s *remoteCacheLedger) AttemptStateEpoch(context.Context, string) (model.JobState, int64, error) {
	return s.state, 1, s.err
}

func TestRemoteCacheCleanupRetractsWhenStateCannotBeProven(t *testing.T) {
	id := "cache-unknown-state"
	ledger := &remoteCacheLedger{state: "passed", err: errors.New("state unavailable")}
	remoteCaches.Store(id, &remoteCacheRecord{run: id, attempt: id, lease: ledger, store: ledger})
	t.Cleanup(func() { remoteCaches.Delete(id) })
	oldWarn := cacheOn.warn
	var warnings []string
	cacheOn.warn = func(s string) { warnings = append(warnings, s) }
	t.Cleanup(func() { cacheOn.warn = oldWarn })
	if err := (remoteCacheFinalize{}).AfterRun(context.Background(), RunInfo{RunID: id}); err != nil {
		t.Fatal(err)
	}
	if ledger.retracted != 1 || len(warnings) == 0 || !strings.Contains(warnings[0], "state unavailable") {
		t.Fatalf("unproven pass retained cache: retract=%d warnings=%v", ledger.retracted, warnings)
	}
}

type remoteCacheFreshRetractor struct {
	remoteCacheLedger
	fresh bool
}

func (l *remoteCacheFreshRetractor) RetractUsing(ctx context.Context, ledger cr.Ledger) error {
	l.fresh = true
	_, _, err := ledger.AttemptStateEpoch(ctx, "run1.jobA.a1")
	return err
}

func TestRemoteCacheCleanupReopensClosedStore(t *testing.T) {
	e := newRemoteEnv(t)
	home := t.TempDir()
	// Checkpoint through a second live handle, then close the original handle.
	// SQLite backup is unnecessary here: make the fallback DB a separate fixture
	// with the same durable attempt identity.
	fresh, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = fresh.CreatePipeline(context.Background(), store.PipelineRow{ID: "run1", Project: "/proj", Revision: "rev", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err = fresh.CreateJob(context.Background(), store.JobRow{ID: "run1.jobA", PipelineID: "run1", Name: "jobA", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err = fresh.CreateAttempt(context.Background(), store.AttemptRow{ID: e.id, JobID: "run1.jobA", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	if err = fresh.Close(); err != nil {
		t.Fatal(err)
	}
	if err = e.st.Close(); err != nil {
		t.Fatal(err)
	}
	l := &remoteCacheFreshRetractor{}
	remoteCaches.Store(e.id, &remoteCacheRecord{run: "closed-cache-run", attempt: e.id, home: home, lease: l, store: e.st})
	t.Cleanup(func() { remoteCaches.Delete(e.id) })
	if err = (remoteCacheFinalize{}).AfterRun(context.Background(), RunInfo{RunID: "closed-cache-run"}); err != nil {
		t.Fatal(err)
	}
	if !l.fresh || l.retracted != 0 {
		t.Fatalf("closed ledger reused: fresh=%v old retractions=%d", l.fresh, l.retracted)
	}
}

func TestPrepareRemoteCacheSandboxedContainer(t *testing.T) {
	t.Setenv("NSELF_CI_CACHE_DISABLE", "")
	t.Setenv("NSELF_CI_ARTIFACT_MAX_FILE", "")
	e := newRemoteEnv(t)
	e.sink.home = t.TempDir()
	e.sink.expect(e.id, testRev)
	t.Cleanup(func() { remoteCaches.Delete(e.id) })
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lease := protocol.Lease{LeaseID: e.lease, AttemptID: e.id, Epoch: uint64(e.epoch), Job: model.Job{Kind: "build", Isolation: "sandboxed-container", Container: &model.ContainerConfig{Image: "example.invalid/build@sha256:" + strings.Repeat("a", 64)}}}
	got := prepareCache(context.Background(), e.sink, &lease, root, "node", false, "linux/amd64")
	if _, ok := got.(cacheLeaseSink); !ok || lease.Cache == nil || len(lease.Cache.Save) != 1 || lease.Cache.Save[0].Spec != "buildx" {
		t.Fatalf("sandboxed container did not plan container cache: %T %+v", got, lease.Cache)
	}
}
func (s *remoteCacheLedger) Finalize(context.Context, time.Duration) error { return nil }
func (s *remoteCacheLedger) Retract(context.Context) error {
	s.retracted++
	return nil
}

func TestRemoteCacheCleanupRetractsOnlyCancelledWriter(t *testing.T) {
	for _, state := range []model.JobState{"passed", "cancelled", "failed"} {
		t.Run(string(state), func(t *testing.T) {
			id := "cache-cleanup-" + string(state)
			ledger := &remoteCacheLedger{state: state}
			remoteCaches.Store(id, &remoteCacheRecord{run: id, attempt: id, lease: ledger, store: ledger})
			t.Cleanup(func() { remoteCaches.Delete(id) })
			hook := remoteCacheFinalize{}
			if err := hook.AfterRun(context.Background(), RunInfo{RunID: "other-run"}); err != nil || !hook.Wants(AttemptInfo{AttemptID: id}) {
				t.Fatal("another run removed the session")
			}
			if err := hook.AfterRun(context.Background(), RunInfo{RunID: id}); err != nil || hook.Wants(AttemptInfo{AttemptID: id}) {
				t.Fatal("completed run retained the session")
			}
			want := 1
			if state == "passed" {
				want = 0
			}
			if ledger.retracted != want {
				t.Fatalf("retract calls=%d want %d", ledger.retracted, want)
			}
		})
	}
}
