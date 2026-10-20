package main

// Purpose: the hosted-provider wiring of the V15 coordinator start (D-1327, D-1394): it points the provider
//          runtime and the GitHub-hosted provider at the coordinator's store and the operator's policy, tells the
//          hosted driver the trust class of an attempt, and names the repositories the world describes hosted
//          runners for.
// Inputs:  the open coordinator store, the data dir (the policy resolver's root) and the repository allowlist.
// Outputs: serveHosted installs runtime.UseStore, runtime.UsePolicy, hosted.UsePolicy, hosted.UseStore,
//          hosted.UseAttempts and runtime.DefaultContributor().SetTargets; the returned func removes them.
// Constraints: call once at coordinator start, before the first dispatch. The policy is the operator's own
//          (store rows and user files): the coordinator reads no repository's policy file. An unreadable policy
//          is an error to the provider, never a default that widens trust. An attempt the store does not know
//          keeps hosted's safe default (untrusted). No fork-pull-request fetch ref is set: the store does not keep
//          the pull number, so untrusted work dispatches on the admitted revision and the untrusted ref.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// operatorPolicyTTL bounds how often the world builder (every dispatcher tick) re-resolves the policy.
const operatorPolicyTTL = 10 * time.Second

// serveHosted installs the hosted-provider wiring and returns the func that removes it.
func serveHosted(st *store.Store, home string, allowed []string) (undo func()) {
	return serveHostedWith(st, operatorPolicySource(st, home, time.Now), allowed)
}

// serveHostedWith is serveHosted with the policy source given (tests).
func serveHostedWith(st *store.Store, src func(context.Context) (*policy.Effective, error), allowed []string) (undo func()) {
	runtime.UseStore(st)
	runtime.UsePolicy(src)
	hosted.UsePolicy(src)
	hosted.UseStore(st)
	hosted.UseAttempts(attemptMetaFrom(st))
	targets := hostedTargets(allowed)
	runtime.DefaultContributor().SetTargets(func(context.Context) ([]providers.Query, error) { return targets, nil })
	return func() {
		runtime.DefaultContributor().SetTargets(nil)
		hosted.UseAttempts(nil)
		hosted.UsePolicy(nil)
		runtime.UsePolicy(nil)
	}
}

// hostedTargets are the repositories hosted runners are described for: the coordinator's allowlist, whose
// entries are "owner/name" (the project of a pipeline the coordinator accepted).
func hostedTargets(allowed []string) []providers.Query {
	out := make([]providers.Query, 0, len(allowed))
	for _, r := range allowed {
		if r != "" {
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
