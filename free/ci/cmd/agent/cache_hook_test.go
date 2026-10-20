package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/authz"
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
