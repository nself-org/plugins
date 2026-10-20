package main

// Purpose: the hosted-provider wiring of the coordinator start (D-1327, D-1394): after serveHostedWith the
//          world builder describes hosted runners for the allowlisted repositories only, the operator policy
//          reaches the provider keys, and the hosted driver reads an attempt's trust class from the store;
//          the undo returns every default.
// Inputs:  a real store, a recording provider registered once for the process, an injected policy source.
// Outputs: pass or fail.
// Constraints: no network; the provider is the fake with its Describe replaced.

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const probeProviderID = "serve-hosted-probe"

// probeProvider records the queries the world builder describes it with.
type probeProvider struct {
	*fake.Fake
	mu      sync.Mutex
	queries []providers.Query
}

func (*probeProvider) ID() string { return probeProviderID }

func (p *probeProvider) Describe(_ context.Context, q providers.Query) ([]model.Capability, error) {
	p.mu.Lock()
	p.queries = append(p.queries, q)
	p.mu.Unlock()
	ptr := func(s string) *string { return &s }
	slots := 2
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity.ID, c.Identity.Name = probeProviderID+":"+q.Repo, probeProviderID+":"+q.Repo
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Availability.Slots.Value = &slots
	c.Economics.Class = model.Fact[string]{Value: ptr("included"), Confidence: "known", Source: "assigned"}
	return []model.Capability{c}, nil
}

func (p *probeProvider) seen() []providers.Query {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]providers.Query(nil), p.queries...)
}

var (
	hostProbe     = &probeProvider{}
	hostProbeOnce sync.Once
)

func useProbeProvider(t *testing.T) *probeProvider {
	t.Helper()
	hostProbeOnce.Do(func() {
		hostProbe.Fake = fake.New(fake.Options{})
		if err := providers.Register(hostProbe); err != nil {
			t.Fatal(err)
		}
		if err := runtime.RegisterCredential(probeProviderID, func(context.Context) rest.AuthFunc {
			return func(context.Context) (string, error) { return "synthetic-test-token", nil }
		}); err != nil {
			t.Fatal(err)
		}
	})
	hostProbe.mu.Lock()
	hostProbe.queries = nil
	hostProbe.mu.Unlock()
	runtime.DefaultContributor().ForgetCredentials()
	t.Setenv("NSELF_V15", "1")
	return hostProbe
}

func serveTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return time.Unix(100, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func seedAttempt(t *testing.T, st *store.Store, trust string) {
	t.Helper()
	ctx := context.Background()
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: "pipe", Project: "o/r", Revision: "rev", Trigger: "pull_request", SourceTrust: trust, PrivacyZone: "hosted-allowed", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.JobRow{ID: "job", PipelineID: "pipe", Name: "job", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: "att", JobID: "job", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
}

func hostedFacts(t *testing.T, attempt string) string {
	t.Helper()
	node := model.Capability{}
	node.Identity.Labels = []string{hosted.LabelRepo + "o/r"}
	f, err := hosted.Facts(context.Background(), node, protocol.Lease{AttemptID: attempt})
	if err != nil {
		t.Fatal(err)
	}
	return f.Trust
}

func TestHostedTargets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed []string
		want    []providers.Query
	}{
		{"every allowlisted repository", []string{"o/r", "o/s"}, []providers.Query{{Project: "o/r", Repo: "o/r"}, {Project: "o/s", Repo: "o/s"}}},
		{"an empty entry is skipped", []string{"", "o/r"}, []providers.Query{{Project: "o/r", Repo: "o/r"}}},
		{"a repository named twice (flag and environment) is described once", []string{"o/r", "O/R", "o/r"}, []providers.Query{{Project: "o/r", Repo: "o/r"}}},
		{"no allowlist describes nothing", nil, []providers.Query{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostedTargets(tc.allowed); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("targets %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestServeHostedDescribesOnlyGitHubHosted: another registered provider is never asked about the
// allowlisted "owner/name" (it could read it as a project of its own forge).
func TestServeHostedDescribesOnlyGitHubHosted(t *testing.T) {
	p := useProbeProvider(t)
	st := serveTestStore(t)
	// an unreadable policy leaves github-hosted without a credential (E700), so the pass sends no request
	undo := serveHostedWith(st, func(context.Context) (*policy.Effective, error) { return nil, errors.New("no policy in this test") }, []string{"o/r"})
	t.Cleanup(undo)
	var w sched.World
	if err := runtime.DefaultContributor().Fill(context.Background(), &w); err != nil {
		t.Fatal(err)
	}
	if got := p.seen(); len(got) != 0 {
		t.Fatalf("a provider other than github-hosted was described: %+v", got)
	}
	for _, r := range w.Hosted {
		if r.Provider == probeProviderID {
			t.Fatalf("a runner of the probe provider is in the world: %s", r.ID)
		}
	}
}

// TestServeHostedAttemptTrust: the trust class of a leased attempt is the pipeline's, an unknown attempt
// keeps the safe default, and without the wiring everything is untrusted.
func TestServeHostedAttemptTrust(t *testing.T) {
	useProbeProvider(t)
	st := serveTestStore(t)
	seedAttempt(t, st, "owner")
	if got := hostedFacts(t, "att"); got != "untrusted" {
		t.Fatalf("before the wiring: trust %q, want untrusted", got)
	}
	undo := serveHostedWith(st, func(context.Context) (*policy.Effective, error) { return nil, nil }, []string{"o/r"})
	if got := hostedFacts(t, "att"); got != "owner" {
		t.Fatalf("known attempt: trust %q, want owner", got)
	}
	if got := hostedFacts(t, "missing"); got != "untrusted" {
		t.Fatalf("unknown attempt: trust %q, want untrusted", got)
	}
	undo()
	if got := hostedFacts(t, "att"); got != "untrusted" {
		t.Fatalf("after undo: trust %q, want untrusted", got)
	}
}

// TestServeHostedPolicyReachesProviders: an unreadable operator policy is an error to the provider keys
// (never a silent default), and the undo restores the defaults.
func TestServeHostedPolicyReachesProviders(t *testing.T) {
	useProbeProvider(t)
	st := serveTestStore(t)
	boom := errors.New("policy unreadable")
	undo := serveHostedWith(st, func(context.Context) (*policy.Effective, error) { return nil, boom }, nil)
	if _, err := hosted.CurrentSettings(context.Background()); err == nil {
		t.Fatal("hosted settings ignored the operator policy source")
	}
	if _, err := runtime.CurrentSettings(context.Background()); err == nil {
		t.Fatal("runtime settings ignored the operator policy source")
	}
	undo()
	if _, err := hosted.CurrentSettings(context.Background()); err != nil {
		t.Fatalf("after undo: %v", err)
	}
	if _, err := runtime.CurrentSettings(context.Background()); err != nil {
		t.Fatalf("after undo: %v", err)
	}
}

// TestOperatorPolicySource: the coordinator resolves the operator policy from its store without a
// repository, and caches it for operatorPolicyTTL.
func TestOperatorPolicySource(t *testing.T) {
	st := serveTestStore(t)
	now := time.Unix(1000, 0)
	src := operatorPolicySource(st, t.TempDir(), func() time.Time { return now })
	first, err := src(context.Background())
	if err != nil || first == nil {
		t.Fatalf("resolve: %v %v", first, err)
	}
	now = now.Add(operatorPolicyTTL / 2)
	if again, _ := src(context.Background()); again != first {
		t.Fatal("a second call inside the TTL re-resolved the policy")
	}
	now = now.Add(operatorPolicyTTL)
	if later, err := src(context.Background()); err != nil || later == first {
		t.Fatalf("after the TTL the policy must be resolved again: %v", err)
	}
}
