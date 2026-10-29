package main

// Purpose: the hosted-provider wiring of the V15 coordinator start (D-1327, D-1394): it points the provider
//          runtime and the GitHub-hosted provider at the coordinator's store and the operator's policy, tells the
//          hosted driver the trust class of an attempt, and names the repositories the world describes hosted
//          runners for.
// Inputs:  the open coordinator store, the data dir (the policy resolver's root) and the repository allowlist.
// Outputs: serveHosted installs serveGitLab's wiring (D-1346), runtime.UseStore, runtime.UsePolicy, hosted.UsePolicy, hosted.UseStore,
//          hosted.UseAttempts and the contributor targets (GitHub-hosted only, each allowlisted repository
//          once, lookups cached for hostedDescribeTTL); the returned func removes them.
// Constraints: call once at coordinator start, before the first dispatch. The policy is the operator's own
//          (store rows and user files): the coordinator reads no repository's policy file. An unreadable policy
//          is an error to the provider, never a default that widens trust. An attempt the store does not know
//          keeps hosted's safe default (untrusted). No fork-pull-request fetch ref is set: the store does not keep
//          the pull number, so untrusted work dispatches on the admitted revision and the untrusted ref.

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// operatorPolicyTTL bounds how often the world builder (every dispatcher tick) re-resolves the policy.
const operatorPolicyTTL = 10 * time.Second

// hostedDescribeTTL bounds how often a repository's hosted runners are looked up (one API request each).
const hostedDescribeTTL = 60 * time.Second

// serveHosted installs the hosted-provider wiring and returns the func that removes it.
func serveHosted(st *store.Store, home string, allowed []string) (undo func()) {
	return serveHostedWith(st, operatorPolicySource(st, home, time.Now), allowed)
}

// serveHostedWith is serveHosted with the policy source given (tests).
func serveHostedWith(st *store.Store, src func(context.Context) (*policy.Effective, error), allowed []string) (undo func()) {
	oldSender := report.Default
	sender := *oldSender
	sender.Store = st
	report.Default = &sender
	health.UseStore(st)
	runtime.UseStore(st)
	runtime.UsePolicy(src)
	hosted.UsePolicy(src)
	hosted.UseStore(st)
	hosted.UseAttempts(attemptMetaFrom(st))
	undoGitLab := serveGitLab(st)
	targets := hostedTargets(allowed)
	c := runtime.DefaultContributor()
	// only the GitHub-hosted provider is described, and only for GitHub repositories: another provider
	// would read the same "owner/name" as a project of its own forge
	c.SetTargetsFor(func(_ context.Context, provider string) ([]providers.Query, error) {
		if provider != hosted.ProviderID {
			return nil, nil
		}
		return targets, nil
	})
	c.SetDescribeTTL(hostedDescribeTTL)
	return func() {
		health.Default().Wait()
		health.UseStore(nil)
		report.Default = oldSender
		c.SetTargetsFor(nil)
		c.SetDescribeTTL(0)
		undoGitLab()
		hosted.UseAttempts(nil)
		hosted.UsePolicy(nil)
		runtime.UsePolicy(nil)
		hosted.ReleaseStore()
		runtime.ReleaseStore()
	}
}

// hostedTargets are the repositories hosted runners are described for: the coordinator's allowlist, whose
// entries are "owner/name" (the project of a pipeline the coordinator accepted). Each repository once,
// whatever its case (a second Describe of the same repository would collide on its runner ids).
func hostedTargets(allowed []string) []providers.Query {
	out := make([]providers.Query, 0, len(allowed))
	seen := map[string]bool{}
	for _, r := range allowed {
		if k := strings.ToLower(r); r != "" && !seen[k] {
			seen[k] = true
			out = append(out, providers.Query{Project: r, Repo: r})
		}
	}
	return out
}

// attemptMetaFrom reads the trust class of an attempt's pipeline from the coordinator store.
func attemptMetaFrom(st *store.Store) hosted.Attempts {
	return func(ctx context.Context, attemptID string) (hosted.AttemptMeta, bool, error) {
		trust, ok, err := st.AttemptSourceTrust(ctx, attemptID)
		if err != nil || !ok {
			return hosted.AttemptMeta{}, false, err
		}
		return hosted.AttemptMeta{Trust: trust}, true, nil
	}
}

// operatorPolicySource resolves the operator's effective policy from the store, cached for operatorPolicyTTL
// (an error is not cached, so a transient failure is retried on the next call).
func operatorPolicySource(st *store.Store, home string, now func() time.Time) func(context.Context) (*policy.Effective, error) {
	var (
		mu  sync.Mutex
		eff *policy.Effective
		at  time.Time
	)
	return func(ctx context.Context) (*policy.Effective, error) {
		mu.Lock()
		defer mu.Unlock()
		if eff != nil && now().Sub(at) < operatorPolicyTTL {
			return eff, nil
		}
		cfg := pdoctor.Load(ctx, home, report.Capture, st)
		if cfg.Effective == nil {
			return nil, cfg.Err
		}
		eff, at = cfg.Effective, now()
		return eff, nil
	}
}

// providerRepoFor is the repository whose GitHub-hosted runners a run may use: the project of a pipeline
// whose code comes from github.com (the same sources coordinator.SafeCloneURL pins to it). A pipeline
// from another forge gets none, whatever its owner/name: the same path on GitHub may be someone else's
// public repository, and the job spec would land in its Actions logs.
func providerRepoFor(source, project string) string {
	switch source {
	case "github", "manual", "":
		return project
	}
	return ""
}
