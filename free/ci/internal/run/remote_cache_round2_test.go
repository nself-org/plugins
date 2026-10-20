package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func remoteCacheRound2Fixture(t *testing.T) (*remoteEnv, string, protocol.Lease) {
	t.Helper()
	t.Setenv("NSELF_CI_CACHE_DISABLE", "")
	t.Setenv("NSELF_CI_ARTIFACT_MAX_FILE", "")
	e := newRemoteEnv(t)
	e.sink.home = t.TempDir()
	e.sink.expect(e.id, testRev)
	t.Cleanup(func() { remoteCaches.Delete(e.id) })
	root := t.TempDir()
	for name, body := range map[string]string{"Dockerfile": "FROM scratch\n", "go.mod": "module example.invalid/app\n\ngo 1.26\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	lease := protocol.Lease{LeaseID: e.lease, AttemptID: e.id, Epoch: uint64(e.epoch), Job: model.Job{Kind: "build", Isolation: "container", Container: &model.ContainerConfig{Image: "example.invalid/build@sha256:" + strings.Repeat("a", 64)}}}
	return e, root, lease
}

func TestRemoteCacheCarrierOrderingWiredIntoBinding(t *testing.T) {
	for _, kind := range []string{"ssh", "agent", "future-carrier", ""} {
		t.Run("carrier="+kind, func(t *testing.T) {
			e, root, lease := remoteCacheRound2Fixture(t)
			sink := prepareCache(context.Background(), e.sink, &lease, root, "node", false, "linux/amd64", kind)
			bound, ok := sink.(cacheLeaseSink)
			if !ok {
				t.Fatalf("no cache binding: %T", sink)
			}
			start := time.Now()
			// No refs: only an unordered carrier must retain the grace. The
			// store may refuse commit outside finalizing; timing precedes that.
			_ = bound.l.Finalize(context.Background(), 150*time.Millisecond)
			elapsed := time.Since(start)
			if kind == "ssh" || kind == "agent" {
				if elapsed >= 125*time.Millisecond {
					t.Fatalf("ordered %s waited %v", kind, elapsed)
				}
			} else if elapsed < 140*time.Millisecond {
				t.Fatalf("unknown %s lost grace: %v", kind, elapsed)
			}
		})
	}
}

func TestRemoteCacheKeysUseOnlyLeaseEnvironment(t *testing.T) {
	e, root, lease := remoteCacheRound2Fixture(t)
	key := func(hostFlags, jobFlags string) string {
		t.Helper()
		t.Setenv("GOFLAGS", hostFlags)
		lease.Job.Env = map[string]string{}
		if jobFlags != "" {
			lease.Job.Env["GOFLAGS"] = jobFlags
		}
		prepareCache(context.Background(), e.sink, &lease, root, "node", false, "linux/amd64", "ssh")
		if lease.Cache != nil {
			for _, s := range lease.Cache.Save {
				if s.Spec == "go" {
					return s.Key
				}
			}
		}
		t.Fatal("go save not planned")
		return ""
	}
	if key("-tags=host-one", "") != key("-tags=host-two", "") {
		t.Fatal("coordinator environment changed remote key")
	}
	if key("", "-tags=job-one") == key("", "-tags=job-two") {
		t.Fatal("controlled job environment did not change remote key")
	}
}

func TestRemoteCacheUnsupportedIsolationHasNoPlan(t *testing.T) {
	for _, level := range []string{"vm", "sandbox", "hosted"} {
		t.Run(level, func(t *testing.T) {
			e, root, lease := remoteCacheRound2Fixture(t)
			lease.Job.Isolation = model.Isolation(level)
			// Unsupported executors must skip cache setup entirely, including
			// database reads which could produce irrelevant cache warnings.
			if err := e.st.Close(); err != nil {
				t.Fatal(err)
			}
			oldWarn := cacheOn.warn
			warnings := 0
			cacheOn.warn = func(string) { warnings++ }
			t.Cleanup(func() { cacheOn.warn = oldWarn })
			if got := prepareCache(context.Background(), e.sink, &lease, root, "node", false, "linux/amd64", "ssh"); got != e.sink || lease.Cache != nil {
				t.Fatalf("unsupported isolation got cache: %T %+v", got, lease.Cache)
			}
			if warnings != 0 {
				t.Fatal("unsupported isolation attempted cache setup")
			}
		})
	}
}

func TestRemoteCacheSetupFailuresWarn(t *testing.T) {
	for _, fail := range []string{"facts", "workdir", "detect", "paths", "attest", "policy"} {
		t.Run(fail, func(t *testing.T) {
			e, root, lease := remoteCacheRound2Fixture(t)
			oldWarn, oldAttest := cacheOn.warn, CacheAttest
			var warnings []string
			cacheOn.warn = func(s string) { warnings = append(warnings, s) }
			t.Cleanup(func() { cacheOn.warn, CacheAttest = oldWarn, oldAttest })
			switch fail {
			case "facts":
				lease.AttemptID = "missing-attempt"
			case "workdir":
				lease.Job.Workdir = "../escape"
			case "detect":
				lease.Job.Workdir = "missing"
			case "paths":
				lease.Job.Cache = &model.JobCacheConfig{Paths: []string{"../escape"}}
			case "attest":
				CacheAttest = func(context.Context, AttemptInfo, store.PipelineRow) (CacheAttestation, error) {
					return CacheAttestation{}, errors.New("attest unavailable")
				}
			case "policy":
				t.Setenv("NSELF_CI_ARTIFACT_MAX_FILE", "invalid")
			}
			if got := prepareCache(context.Background(), e.sink, &lease, root, "node", false, "linux/amd64", "ssh"); got != e.sink || lease.Cache != nil || len(warnings) == 0 {
				t.Fatalf("setup failure not warned/dropped: sink=%T plan=%+v warnings=%v", got, lease.Cache, warnings)
			}
		})
	}
}
