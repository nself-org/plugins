package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func cacheHookLease(access, isolation string) protocol.Lease {
	ns := authz.Namespace{Project: "project", Trust: "owner", Protection: authz.Protected}
	return protocol.Lease{AttemptID: "cache-attempt", LeaseID: "cache-lease", Epoch: 1, Job: model.Job{Isolation: model.Isolation(isolation)}, Cache: &protocol.LeaseCache{
		Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: access,
	}}
}

func TestCacheHookReadOnlyMounts(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	l := cacheHookLease("ro", "container")
	l.Cache.Restore = []protocol.CacheRestore{{Spec: "go", Key: strings.Repeat("a", 64), Dir: "go"}}
	l.Cache.Restore[0].Blobs = []protocol.CacheBlob{seedCacheBlob(t, l.Cache.Namespace, "mod")}
	setup, err := cacheBefore(context.Background(), l, session.JobRoot{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer cacheAfter(context.Background(), l, session.JobRoot{}, session.Outcome{State: "cancelled"}, nil)
	if len(setup.Mounts) != 2 {
		t.Fatalf("mounts=%+v", setup.Mounts)
	}
	for _, mount := range setup.Mounts {
		if !mount.ReadOnly {
			t.Fatal("read-only lease has writable mount", mount)
		}
	}
	if setup.Env["GOMODCACHE"] == "" || setup.Env["GOCACHE"] == "" {
		t.Fatal("missing tool environment", setup.Env)
	}
	if _, err := cacheBefore(context.Background(), cacheHookLease("ro", "process"), session.JobRoot{Dir: t.TempDir()}); err == nil {
		t.Fatal("read-only process lease accepted")
	}
}

func seedCacheBlob(t *testing.T, namespace, path string) protocol.CacheBlob {
	t.Helper()
	var data bytes.Buffer
	tw := tar.NewWriter(&data)
	if err := tw.WriteHeader(&tar.Header{Name: "payload", Size: 4, Mode: 0644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data.Bytes()))
	state, err := stateDir()
	if err != nil {
		t.Fatal(err)
	}
	copies, err := remote.NewReadThrough(filepath.Join(state, "cache-readthrough"), 0)
	if err != nil {
		t.Fatal(err)
	}
	f, err := copies.Open(context.Background(), namespace, digest, int64(data.Len()), func(_ context.Context, w io.Writer) error { _, err := w.Write(data.Bytes()); return err })
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return protocol.CacheBlob{Path: path, SHA256: digest, Size: int64(data.Len())}
}

func TestCacheHookMissingRestoreBindsNothing(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	for _, access := range []string{"ro", "rw"} {
		for _, blobs := range []bool{false, true} {
			l := cacheHookLease(access, "container")
			l.Cache.Restore = []protocol.CacheRestore{{Spec: "go", Key: strings.Repeat("a", 64), Dir: "go"}}
			if blobs {
				l.Cache.Restore[0].Blobs = []protocol.CacheBlob{{Path: "mod", SHA256: strings.Repeat("b", 64), Size: 2}}
			}
			setup, err := cacheBefore(context.Background(), l, session.JobRoot{Dir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			if len(setup.Mounts) != 0 || len(setup.Env) != 0 {
				t.Fatalf("miss bound empty cache: %+v", setup)
			}
			_ = cacheAfter(context.Background(), l, session.JobRoot{}, session.Outcome{}, nil)
		}
	}
}

func TestCacheHookPartialRestoreBindsNothing(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	l := cacheHookLease("ro", "container")
	l.Cache.Restore = []protocol.CacheRestore{{Spec: "go", Key: strings.Repeat("a", 64), Dir: "go", Blobs: []protocol.CacheBlob{seedCacheBlob(t, l.Cache.Namespace, "mod"), {Path: "build", SHA256: strings.Repeat("b", 64), Size: 2}}}}
	root := session.JobRoot{Dir: t.TempDir()}
	setup, err := cacheBefore(context.Background(), l, root)
	if err != nil {
		t.Fatal(err)
	}
	defer cacheAfter(context.Background(), l, root, session.Outcome{}, nil)
	if len(setup.Mounts) != 0 || len(setup.Env) != 0 {
		t.Fatalf("partial restore bound: %+v", setup)
	}
	if _, err := os.Stat(filepath.Join(root.Dir, "cache-work", "go")); !os.IsNotExist(err) {
		t.Fatalf("partial restore retained: %v", err)
	}
}

func TestCacheHookSetupFailureIsBestEffort(t *testing.T) {
	for _, failure := range []string{"invalid-cap", "cache-work-exists", "bad-state"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
			root := session.JobRoot{Dir: t.TempDir()}
			switch failure {
			case "invalid-cap":
				t.Setenv("NSELF_CI_NODE_CACHE_MAX_BYTES", "broken")
			case "cache-work-exists":
				if err := os.Mkdir(filepath.Join(root.Dir, "cache-work"), 0755); err != nil {
					t.Fatal(err)
				}
			case "bad-state":
				p := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(p, nil, 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("NSELF_CI_AGENT_HOME", p)
			}
			l := cacheHookLease("rw", "container")
			l.Cache.Save = []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod", "go/build"}}}
			setup, err := cacheBefore(context.Background(), l, root)
			if err != nil || len(setup.Env) != 0 || len(setup.Mounts) != 0 {
				t.Fatalf("setup=%+v err=%v", setup, err)
			}
		})
	}
}

func TestCacheBeforeJobFailureDropsEnvironment(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	l := cacheHookLease("rw", "container")
	l.Cache.Save = []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod", "go/build"}}}
	setup, err := cacheBefore(context.Background(), l, session.JobRoot{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer cacheAfter(context.Background(), l, session.JobRoot{}, session.Outcome{}, nil)
	ws, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := ws.Mkdir(specs.WorkspaceCacheName, 0755); err != nil {
		t.Fatal(err)
	}
	if err := cacheBeforeJob(execSpec{AttemptID: l.AttemptID}, setup.Env)(context.Background(), ws); err != nil {
		t.Fatal(err)
	}
	if len(setup.Env) != 0 {
		t.Fatalf("failed cache still in environment: %+v", setup.Env)
	}
	if agentAfterJob(execSpec{AttemptID: l.AttemptID}, jobInputs{}) != nil {
		t.Fatal("failed workspace still collects")
	}
}

func TestCacheAfterJobAbsentWithoutCollector(t *testing.T) {
	if agentAfterJob(execSpec{AttemptID: "no-cache-or-artifacts"}, jobInputs{}) != nil {
		t.Fatal("no-op collector installed")
	}
}

func TestCacheStageFailureKeepsUsableBindings(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	l := cacheHookLease("rw", "process")
	l.Cache.Save = []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod", "go/build"}}}
	root := session.JobRoot{Dir: t.TempDir()}
	if err := os.Mkdir(filepath.Join(root.Dir, "cache-stage"), 0700); err != nil {
		t.Fatal(err)
	}
	setup, err := cacheBefore(context.Background(), l, root)
	if err != nil {
		t.Fatal(err)
	}
	defer cacheAfter(context.Background(), l, root, session.Outcome{}, nil)
	if setup.Env["GOCACHE"] == "" || setup.Env["GOMODCACHE"] == "" {
		t.Fatalf("usable binding lost: %+v", setup)
	}
	if agentAfterJob(execSpec{AttemptID: l.AttemptID}, jobInputs{}) != nil {
		t.Fatal("failed stage still collects")
	}
}

func TestCacheHookReplacementClosesRoots(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	l := cacheHookLease("rw", "process")
	l.Cache.Save = []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod", "go/build"}}}
	oldRoot := session.JobRoot{Dir: t.TempDir()}
	if _, err := cacheBefore(context.Background(), l, oldRoot); err != nil {
		t.Fatal(err)
	}
	v, _ := nodeCaches.Load(l.AttemptID)
	old := v.(*nodeCache)
	if _, err := cacheBefore(context.Background(), l, session.JobRoot{Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	defer cacheAfter(context.Background(), l, session.JobRoot{}, session.Outcome{}, nil)
	if _, err := old.base.Stat("."); err == nil {
		t.Fatal("replaced root left open")
	}
	if _, err := os.Stat(filepath.Join(oldRoot.Dir, "cache-stage")); !os.IsNotExist(err) {
		t.Fatalf("replaced stage retained: %v", err)
	}
}

type deadlineCacheStage struct {
	t      *testing.T
	called bool
}

type refusedCacheStage struct{ deadlineCacheStage }

func (s *refusedCacheStage) Collect(context.Context, *os.Root, authz.ValueScanner) error {
	return fmt.Errorf("E711 secret_match: registered value in cache")
}

func TestCacheCollectionRefusalReachesJobLog(t *testing.T) {
	base, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l := cacheHookLease("rw", "process")
	nodeCaches.Store(l.AttemptID, &nodeCache{base: base, stage: &refusedCacheStage{}})
	defer cacheAfter(context.Background(), l, session.JobRoot{}, session.Outcome{}, nil)
	var output bytes.Buffer
	hook := agentAfterJob(execSpec{AttemptID: l.AttemptID}, jobInputs{Output: func(b []byte) { _, _ = output.Write(b) }})
	if err := hook(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "E711 secret_match") {
		t.Fatalf("refusal absent from job log: %q", output.String())
	}
}

func (s *deadlineCacheStage) Collect(context.Context, *os.Root, authz.ValueScanner) error { return nil }
func (s *deadlineCacheStage) Close() error                                                { return nil }
func (s *deadlineCacheStage) Ship(ctx context.Context, _ <-chan protocol.Offset, _ remote.CacheUploader) error {
	s.called = true
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 90*time.Second || time.Until(deadline) < 89*time.Second {
		s.t.Errorf("upload deadline=%v present=%v", deadline, ok)
	}
	return context.DeadlineExceeded
}

type cacheTestUploader struct{ session.Uploader }

func (cacheTestUploader) Offsets() <-chan protocol.Offset { return nil }
func TestCacheShipHasOwnDeadline(t *testing.T) {
	l := cacheHookLease("rw", "process")
	stage := &deadlineCacheStage{t: t}
	nodeCaches.Store(l.AttemptID, &nodeCache{stage: stage})
	if err := cacheAfter(context.Background(), l, session.JobRoot{}, session.Outcome{State: "passed"}, cacheTestUploader{}); err != nil {
		t.Fatal(err)
	}
	if !stage.called {
		t.Fatal("Ship not called")
	}
}

func TestCacheHookFailedAndCancelledNeverSend(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	for _, state := range []string{"failed", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			l := cacheHookLease("rw", "process")
			l.Cache.Save = []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod", "go/build"}}}
			root := session.JobRoot{Dir: t.TempDir()}
			setup, err := cacheBefore(context.Background(), l, root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(setup.Env["GOMODCACHE"], "payload"), []byte("synthetic data"), 0600); err != nil {
				t.Fatal(err)
			}
			// Even a collected clean set cannot be sent for a failed outcome.
			if err := agentAfterJob(execSpec{AttemptID: l.AttemptID}, jobInputs{})(context.Background(), nil); err != nil {
				t.Fatal(err)
			}
			if err := cacheAfter(context.Background(), l, root, session.Outcome{State: state}, nil); err != nil {
				t.Fatal(err)
			}
			if _, ok := nodeCaches.Load(l.AttemptID); ok {
				t.Fatal("finished attempt retained")
			}
			if _, err := os.Stat(filepath.Join(root.Dir, "cache-stage")); !os.IsNotExist(err) {
				t.Fatal("stage retained", err)
			}
		})
	}
}

func TestCacheHookInvalidAccessRefused(t *testing.T) {
	if _, err := cacheBefore(context.Background(), cacheHookLease("rw-own", "process"), session.JobRoot{Dir: t.TempDir()}); err == nil {
		t.Fatal("invalid wire access accepted")
	}
}
