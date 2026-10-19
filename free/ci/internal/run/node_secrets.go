package run

// Purpose: send the secret values of a job to a node (D-1101, ADR 0025 §6). The lease names the refs
// (never a value); the ssh driver then asks NodeSecretSource for the values of that one lease and puts
// them in a single frame, in memory only.
// Inputs:  the delivery's node and job (leaseSecretRefs), the lease and the run scope in its context
// (NodeSecretSource).
// Outputs: the lease's value-free refs; the lease's values as protocol.SecretValue.
// Constraints: only a PERSONAL coordinator sends values (NSELF_CI_COORDINATOR_MODE=team never does) and
// only to an operator-owned, uid-separated node (secrets_eligible); any other delivery of a secret job
// fails before the driver runs, so a job never starts on a node without its secrets. Values come from
// the same Source a local attempt uses, with the node's facts, and are registered with the redactor
// before they are returned. No legacy environment source: a node is never the operator's own machine.

import (
	"context"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// nodeScope is what a delivery tells the secret source about itself, through the context.
type nodeScope struct {
	// home is the CI home the run uses; root is the project checkout; coordinator labels the cli calls.
	home, root, coordinator string
	// node is the node the lease goes to.
	node model.Capability
}

type nodeScopeKey struct{}

func withNodeScope(ctx context.Context, s nodeScope) context.Context {
	return context.WithValue(ctx, nodeScopeKey{}, s)
}

// NodeSecretsAllowed reports whether this coordinator may send secret values to a node: a PERSONAL one.
func NodeSecretsAllowed() bool { return os.Getenv("NSELF_CI_COORDINATOR_MODE") != "team" }

// nodeTakesSecrets is the coordinator's half of the node rule: operator-owned and uid-separated.
func nodeTakesSecrets(n model.Capability) bool {
	o := n.Identity.Ownership.Value
	return n.SecretsEligible && o != nil && *o == "operator"
}

// leaseSecretRefs returns the value-free refs the lease of job carries to n, or the reason it may not go.
func leaseSecretRefs(n model.Capability, job model.Job) ([]protocol.SecretRef, error) {
	out := []protocol.SecretRef{}
	if len(job.Secrets) == 0 {
		return out, nil
	}
	if !NodeSecretsAllowed() {
		return nil, fail(CodeRun, "this coordinator sends no secret value to a node; a job with secrets runs where the coordinator runs")
	}
	if !nodeTakesSecrets(n) {
		return nil, fail(CodeRun, "node %s is not an operator-owned, uid-separated node (secrets_eligible); it is sent no secret refs", n.Identity.ID)
	}
	refs, err := secrets.ParseAll(job.Secrets)
	if err != nil {
		return nil, err
	}
	for _, r := range refs {
		out = append(out, protocol.SecretRef{Ref: r.Raw, Class: r.Class})
	}
	return out, nil
}

// nodeFacts are the eligibility facts of a job run by a PERSONAL coordinator on node n. The job is
// `internal`, the weakest class that reaches a dispatch (the trust gate in Coordinator.Run); the policy
// allowlist and the node's declared classes were checked when the job was placed.
func nodeFacts(n model.Capability, job model.Job) secrets.Facts {
	d := sched.DeclarationFromCapability(n, false, trust.Personal)
	iso := d.Isolation
	if iso == "" {
		iso = "process"
	}
	return secrets.Facts{
		Job:    trust.JobFacts{Trust: "internal", PersonalOwnRun: true, Release: job.Kind == "release", Mode: trust.Personal},
		Runner: trust.RunnerFacts{Ownership: d.Ownership, Dedicated: trust.IsDedicatedTrusted(d), Hosted: d.Hosted, UIDSeparation: d.UIDSeparation, Isolation: iso},
	}
}

// NodeSecretSource is the ssh driver's per-lease SecretSource: it resolves the values of the lease's refs on
// this machine. The context must come from a delivery (transportAdapter.Deliver).
func NodeSecretSource(ctx context.Context, l protocol.Lease) ([]protocol.SecretValue, error) {
	sc, ok := ctx.Value(nodeScopeKey{}).(nodeScope)
	if !ok {
		return nil, fail(CodeRun, "lease %s has no delivery scope; no secret value was resolved", l.LeaseID)
	}
	if !NodeSecretsAllowed() || !nodeTakesSecrets(sc.node) {
		return nil, fail(CodeRun, "lease %s may not be sent secret values; none were resolved", l.LeaseID)
	}
	secretsMu.Lock()
	mode := secretsRuntime.Mode
	secretsMu.Unlock()
	if mode != secrets.ModePersonalLocal {
		return nil, fail(CodeRun, "this process is not a PERSONAL coordinator; no secret value was resolved")
	}
	raw := make([]string, len(l.Secrets))
	for i, r := range l.Secrets {
		raw[i] = r.Ref
	}
	refs, err := secrets.ParseAll(raw)
	if err != nil {
		return nil, err
	}
	facts := nodeFacts(sc.node, l.Job)
	a := AttemptInfo{AttemptID: l.AttemptID, Job: l.Job, Home: sc.home, SecretsInfo: SecretsInfo{Root: sc.root, CoordinatorID: sc.coordinator}}
	src := buildSecretSource(a, Registered.NewRedactor, &facts)
	if z, ok := src.(interface{ Zero() }); ok {
		defer z.Zero()
	}
	env, err := src.Resolve(ctx, raw)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.SecretValue, 0, len(refs))
	for _, r := range refs {
		out = append(out, protocol.SecretValue{Ref: r.Raw, Value: env[r.EnvName()]})
	}
	return out, nil
}
