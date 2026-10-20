package runnerjob

// Purpose: the transport preparer that mints a runner credential after admission (Epic D6, NODE-02 ask).
// It runs at lease delivery, so the forge registration exists only for a job that was admitted and
// placed, and the credential lives in the lease for the delivery and nowhere else.
// Inputs:  the node capability the lease goes to, the lease (completed in place) and its job.
// Outputs: lease.RunnerCredential {kind, value, expires_at}; a ledger row with the registration handle.
// Constraints: the preparer re-checks what placement already proved, because it is the last step before
//          a credential exists: remote only, container isolation sandboxed-container, no secrets, a
//          digest-pinned image, a Linux node that offers at least that isolation and the restricted
//          network scope. Any miss is E708 and nothing is minted; a demand that is not live (finished, or expired while it waited and its slot is taken) is E703 and nothing is minted either. A lease that already carries a
//          credential is refused (nobody else may set one). If the ledger write fails after the mint the
//          registration is removed again at once. A second delivery of the same attempt removes the
//          earlier registration first (the credential is not stored, so it cannot be reused) and keeps the
//          demand's slot while it mints again. A job that carries the runner markers but no credential kind
//          (a pipeline read back from the store without Restore) is refused, never run as an ordinary job. The value
//          never appears in an error, a log line or the ledger.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func refuse(reason string) error {
	return providers.NewError(providers.Config, "E708", reason, 0, time.Time{})
}

// archOf reads linux/<arch> from the job matrix.
func archOf(j model.Job) string {
	if j.Matrix == nil {
		return ""
	}
	p, _ := j.Matrix.Platform.(string)
	if arch, ok := strings.CutPrefix(p, "linux/"); ok && (arch == "amd64" || arch == "arm64") {
		return arch
	}
	return ""
}

func fact[T any](f model.Fact[T]) (T, bool) {
	var zero T
	if f.Value == nil {
		return zero, false
	}
	return *f.Value, true
}

// checkPlacement is the last check before a credential is minted.
func checkPlacement(node model.Capability, j model.Job) error {
	switch {
	case !j.RemoteOnly:
		return refuse("runner.remote_only")
	case j.Isolation != Isolation:
		return refuse("isolation.below_minimum")
	case len(j.Secrets) > 0:
		return refuse("runner.secrets_not_allowed")
	case j.Container == nil || !imagePattern.MatchString(j.Container.Image):
		return refuse("runner.image_not_pinned")
	case archOf(j) == "":
		return refuse("runner.platform")
	case j.Env[EnvProvider] == "" || !repoPattern.MatchString(j.Env[EnvRepo]) || !demandPattern.MatchString(j.Env[EnvDemand]):
		return refuse("runner.demand_missing")
	}
	if os, ok := fact(node.Platform.OS); !ok || os != "linux" {
		return refuse("runner.node_not_linux")
	}
	if iso, ok := fact(node.Trust.Isolation); !ok || trust.RankIsolation(iso) < trust.RankIsolation(Isolation) {
		return refuse("isolation.below_minimum")
	}
	if g, ok := fact(node.Tools.GVisor); !ok || !g {
		return refuse("isolation.no_gvisor")
	}
	scopes, _ := fact(node.Trust.Network)
	for _, s := range scopes {
		if s == "restricted" {
			return nil
		}
	}
	return refuse("network.restricted_unavailable")
}

func prepare(ctx context.Context, node model.Capability, l *protocol.Lease, job model.Job) error {
	if job.RunnerCredential == "" {
		if job.Env[EnvProvider] != "" && job.Env[EnvDemand] != "" { // a runner job read back from the store without Restore: never run it as an ordinary job
			return refuse("runner.restore_missing")
		}
		return nil
	}
	// compat.V15(P7-HOST-19): a lease never carries a forge-runner credential (v1.4) -> the credential is minted after admission
	if !compat.V15() {
		return refuse("compat.v14")
	}
	led := CurrentLedger()
	m := state.minterFor(job.RunnerCredential)
	switch {
	case led == nil || m == nil:
		return providers.NewError(providers.Config, "E700", "runner.minter_unavailable", 0, time.Time{})
	case l.RunnerCredential != nil:
		return refuse("runner.credential_preset")
	case m.Provider() != job.Env[EnvProvider]:
		return refuse("runner.provider_mismatch")
	}
	if err := checkPlacement(node, job); err != nil {
		return err
	}
	repo, demand := job.Env[EnvRepo], job.Env[EnvDemand]
	if ok, err := led.Confirm(ctx, m.Provider(), repo, demand); err != nil {
		return fail("ledger.read")
	} else if !ok { // the demand ended, expired with the cap taken since, or is unknown: no registration for it
		return providers.NewError(providers.Quota, "E703", "runner.demand_not_live", 0, time.Time{})
	}
	if prev, ok, err := led.Runner(ctx, l.AttemptID); err != nil {
		return fail("ledger.read")
	} else if ok && prev.State == stateMinted {
		if err = m.Deregister(ctx, prev.Repo, prev.Handle); err != nil {
			return err
		}
		if err = led.Release(ctx, prev); err != nil { // the demand keeps its slot until the new registration exists
			return fail("ledger.write")
		}
	}
	reg, err := m.Mint(ctx, Request{Repo: repo, Demand: demand, AttemptID: l.AttemptID, Arch: archOf(job)})
	if err != nil {
		return err
	}
	row := Runner{Attempt: l.AttemptID, Provider: m.Provider(), Repo: repo, Demand: demand, Handle: reg.Handle}
	if err = led.RecordRunner(ctx, row); err != nil {
		if derr := m.Deregister(context.WithoutCancel(ctx), repo, reg.Handle); derr != nil {
			return fmt.Errorf("%w; the registration could not be removed: %v", fail("ledger.write"), derr)
		}
		return fail("ledger.write")
	}
	l.RunnerCredential = &protocol.RunnerCredential{Kind: job.RunnerCredential, Value: reg.Credential, ExpiresAt: reg.ExpiresAt.UTC().Format(time.RFC3339)}
	return nil
}
