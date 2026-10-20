package run

// Purpose: install the secret Source (P7-TRUST-02) as the executor's per-attempt SecretSource, so a job
// that declares `secrets:` gets its values as environment on this machine and nowhere else.
// Inputs:  the attempt (AttemptInfo) and the Redactor factory the executor actually uses; ConfigureSecrets
// supplies what only the embedding process knows (an agent node's identity and eligibility, the effective
// policy, a different CLI binary).
// Outputs: a *secrets.Source per attempt that has secrets; the executor calls Zero() when the attempt ends.
// Constraints: the default is the local coordinator on the operator's own machine: it only runs owner and
// internal sources (the trust gate in Coordinator.Run), and the job facts carry the class the coordinator
// gave the source (AttemptInfo.Source); a class outside owner and internal resolves no secret. Values are registered with Redactions (the registry the installed redactor
// reads); if the installed redactor is not a registry filter the Source resolves nothing (E676).

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// SecretsRuntime is the process-level configuration of the secret Source.
type SecretsRuntime struct {
	Mode secrets.Mode
	// Runner runs the nself cli (default: the exec funnel).
	Runner secrets.Runner
	// Binary is the cli program ("" = nself).
	Binary string
	// Facts returns the eligibility facts of an attempt (default: the local operator facts).
	Facts func(AttemptInfo) secrets.Facts
	// NodeID, NodeEligible and AgeKeyPath describe an agent node.
	NodeID       string
	NodeEligible bool
	AgeKeyPath   string
	// Legacy is the personal-node compat source (default: the process environment).
	Legacy *secrets.LegacyEnv
	// Audit receives value-free events such as legacy-env use.
	Audit func(secrets.Event) error
}

var (
	secretsMu      sync.Mutex
	secretsRuntime = SecretsRuntime{Mode: secrets.ModePersonalLocal}
	secretsProbe   = &secrets.ProbeState{}
)

// ConfigureSecrets replaces the secret Source configuration (call before the first run).
func ConfigureSecrets(rt SecretsRuntime) {
	secretsMu.Lock()
	secretsRuntime = rt
	secretsProbe = &secrets.ProbeState{}
	secretsMu.Unlock()
}

func init() { RegisterSecrets(newSecretSource) }

// SecretsInfo is what an attempt (AttemptInfo) and a policy result (PolicyResult) carry for the Source.
type SecretsInfo struct {
	// Root is the project root the run was started for (Options.Root; "" = the working directory).
	Root string
	// CoordinatorID is the live coordinator: the resolver call carries it, so the stale-process sweep of a
	// concurrent attempt does not take it for a leftover.
	CoordinatorID string
	// SecretClasses is the effective policy's `trust.secrets` allowlist; HasSecretClasses is false when the policy has no such key.
	SecretClasses    []string
	HasSecretClasses bool
}

// RegisterSecrets sets the per-attempt secret source factory (single slot).
func RegisterSecrets(f func(AttemptInfo, func() ciexec.Redactor) ciexec.SecretSource) {
	Registered.NewSecrets = f
}

// secretsFactory is the hook's factory, else the registered one.
func (h Hooks) secretsFactory() func(AttemptInfo, func() ciexec.Redactor) ciexec.SecretSource {
	if h.NewSecrets != nil {
		return h.NewSecrets
	}
	return Registered.NewSecrets
}

// localFacts are the facts of a job run by the local coordinator on the operator's own machine. The job
// trust is the class the coordinator gave the source ("internal" when the attempt carries none). The local
// declaration accepts owner and internal only, so a source outside them resolves no secret at all (D-1325).
func localFacts(a AttemptInfo) secrets.Facts {
	iso := a.Job.Isolation
	if iso == "" {
		iso = "process"
	}
	accepts := []model.TrustClass{"owner", "internal"}
	decl := trust.Declaration{Accepts: accepts, Ownership: "operator", Source: "assigned",
		PersonalOwnNode: true, PersonalOwnRun: true, ReleaseJob: a.Job.Kind == "release"}
	runner := trust.RunnerFacts{Ownership: "operator", PersonalOwnNode: true, Dedicated: trust.IsDedicatedTrusted(decl), Isolation: iso}
	class, allowed, has := a.Source, a.SecretClasses, a.HasSecretClasses
	if class == "" {
		class = "internal"
	}
	if !slices.Contains(accepts, class) {
		allowed, has = nil, true // an empty allowlist: every secret class is refused (SecretsNotInPolicy)
	}
	return secrets.Facts{
		Job:    trust.JobFacts{Trust: class, PersonalOwnRun: true, Release: a.Job.Kind == "release", Mode: trust.Personal},
		Runner: runner, Allowed: allowed, HasPolicy: has,
	}
}

// redactorInstalled reports whether newRedactor builds the registry redactor (P7-TRUST-19).
func redactorInstalled(newRedactor func() ciexec.Redactor) func() bool {
	return func() bool {
		if newRedactor == nil {
			return false
		}
		f, ok := newRedactor().(*redact.Filter)
		return ok && f.Reads(Redactions)
	}
}

func newSecretSource(a AttemptInfo, newRedactor func() ciexec.Redactor) ciexec.SecretSource {
	secretsMu.Lock()
	rt, probe := secretsRuntime, secretsProbe
	secretsMu.Unlock()
	facts := localFacts
	if rt.Facts != nil {
		facts = rt.Facts
	} else if rt.Mode != secrets.ModePersonalLocal {
		facts = func(AttemptInfo) secrets.Facts { return secrets.Facts{} } // no operator facts for a node or a coordinator
	}
	runner := rt.Runner
	if runner == nil {
		runner = &secrets.ExecRunner{Home: a.Home, Attempt: a.AttemptID, CoordinatorID: coordinatorOf(a)}
	}
	legacy := rt.Legacy
	if legacy == nil && rt.Mode == secrets.ModePersonalLocal {
		legacy = &secrets.LegacyEnv{Getenv: os.Getenv}
	}
	return secrets.New(secrets.Config{
		Mode:    rt.Mode,
		Attempt: a.AttemptID,
		Resolver: &secrets.Resolver{Runner: runner, Binary: rt.Binary, Project: projectOf(a),
			AgeKeyPath: rt.AgeKeyPath, Shared: probe},
		Registry: Redactions, RedactorInstalled: redactorInstalled(newRedactor),
		Facts: facts(a), NodeID: rt.NodeID, NodeEligible: rt.NodeEligible, AgeKeyPath: rt.AgeKeyPath,
		Legacy: legacy, Audit: auditFn(rt, a),
	})
}

// projectOf is the project root passed to the cli: the checkout the attempt belongs to.
func projectOf(a AttemptInfo) string {
	root := a.Root
	if root == "" {
		root = "."
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}

// auditFn is the Source's audit sink: the configured one, else a value-free row in the run's own store.
func auditFn(rt SecretsRuntime, a AttemptInfo) func(secrets.Event) error {
	if rt.Audit != nil {
		return rt.Audit
	}
	path := ""
	if a.Home != "" {
		path = filepath.Join(a.Home, "state.db")
	}
	return func(e secrets.Event) error {
		st, err := OpenStore(path)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()
		return st.AppendAudit(context.Background(), "secrets", e.Kind, e.Ref, "attempt="+e.Attempt+";source="+e.Source)
	}
}

// coordinatorOf is the coordinator id the resolver's executor call carries. The live id is
// "<boot>:<pid>:<start>"; the executor accepts only safe ids, so it is mapped the way jobs map it
// (safeID): without that every resolver call under a real coordinator failed with E612.
func coordinatorOf(a AttemptInfo) string {
	if a.CoordinatorID != "" {
		return safeID(a.CoordinatorID)
	}
	return "secrets"
}
