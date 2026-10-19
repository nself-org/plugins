package secrets

// Purpose: the exec.SecretSource for one attempt (Epic D4, D5). It parses the job's refs, checks them
// against the trust facts and the effective policy, resolves them on the executing node only, registers
// every value with the redactor and hands the values to the executor as environment.
// Inputs:  a Config built per attempt (mode, resolver, redaction registry, facts, node facts).
// Outputs: Resolve (exec.SecretSource), Refs (value-free lease refs), Sources (ref -> evidence source),
// Zero (attempt end).
// Constraints: nothing is resolved unless the refs parse (E674), the mode and node allow it (E677), the
// installed executor Redactor is the registry redactor (E676) and the cli answers the probe (E675). The
// order is fixed so an ineligible attempt makes zero resolver calls. All values are registered with the
// redactor before any is returned; a Register error returns nothing. A TEAM coordinator Source never
// resolves. No error string, audit event or source label contains a value.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Mode is where the Source runs.
type Mode string

const (
	// ModePersonalLocal is the operator's own machine running its own coordinator.
	ModePersonalLocal Mode = "personal-local"
	// ModeAgentNode is an executing node with its own age identity.
	ModeAgentNode Mode = "agent-node"
	// ModeTeamCoordinator passes references only.
	ModeTeamCoordinator Mode = "team-coordinator"
)

// Registrar is the redaction registry (redact.Redactor.Register).
type Registrar interface{ Register(v []byte) error }

// Event is a value-free audit record the Source emits.
type Event struct {
	Kind, Attempt, Ref, Source string
}

// EventLegacyEnv is the audit kind of a value taken from the legacy environment.
const EventLegacyEnv = "secret.legacy_env"

// Config is everything a Source needs.
type Config struct {
	Mode     Mode
	Attempt  string
	Resolver ValueResolver
	Registry Registrar
	// RedactorInstalled reports whether the executor's installed Redactor is the TRUST-19 redactor.
	RedactorInstalled func() bool
	Facts             Facts
	// NodeID and NodeEligible are the registry facts of an agent node (secrets_eligible).
	NodeID       string
	NodeEligible bool
	// AgeKeyPath is the node's own age identity; an agent node without one resolves nothing.
	AgeKeyPath string
	// Legacy, when set, is the personal-node compat source for release/ANDROID_KEYSTORE_*.
	Legacy *LegacyEnv
	// Audit records a value-free event; an error fails the attempt (the compat source is never used unaudited).
	Audit func(Event) error
}

// LeaseRef is what a coordinator sends a node instead of a value.
type LeaseRef struct {
	Ref   string            `json:"ref"`
	Class model.SecretClass `json:"class"`
}

// Source implements exec.SecretSource.
type Source struct {
	cfg     Config
	mu      sync.Mutex
	bufs    [][]byte
	sources map[string]string
}

// New returns a Source for one attempt.
func New(cfg Config) *Source { return &Source{cfg: cfg, sources: map[string]string{}} }

// Refs validates refs and returns them as lease refs. It never resolves; a coordinator in any mode uses it.
func (s *Source) Refs(raw []string) ([]LeaseRef, error) {
	refs, err := ParseAll(raw)
	if err != nil {
		return nil, err
	}
	if reasons := Check(refs, s.cfg.Facts); len(reasons) > 0 {
		return nil, errNotEligible(reasonStrings(reasons), "secret refs refused: %v", reasonStrings(reasons))
	}
	out := make([]LeaseRef, len(refs))
	for i, r := range refs {
		out[i] = LeaseRef{Ref: r.Raw, Class: r.Class}
	}
	return out, nil
}

// legacyPermitted: only the operator's own personal node, never an agent or a coordinator for a team, and
// never without an audit sink (every use of the compat source is audited).
func (s *Source) legacyPermitted() bool {
	return s.cfg.Legacy != nil && s.cfg.Audit != nil && s.cfg.Mode == ModePersonalLocal && s.cfg.Facts.Runner.PersonalOwnNode && s.cfg.Facts.Runner.Ownership == "operator"
}

// Resolve implements exec.SecretSource. On any error nothing is returned and nothing stays held.
func (s *Source) Resolve(ctx context.Context, raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	refs, err := ParseAll(raw)
	if err != nil {
		return nil, err
	}
	switch s.cfg.Mode {
	case ModePersonalLocal:
	case ModeAgentNode:
		if !s.cfg.NodeEligible {
			return nil, errNotEligible([]string{ReasonNodeNotEligible}, "node %s is not secrets-eligible (%s)", echo(s.cfg.NodeID), ReasonNodeNotEligible)
		}
	default:
		return nil, errNotEligible([]string{ReasonRefsOnly}, "this coordinator passes secret refs only and holds no value (%s)", ReasonRefsOnly)
	}
	if reasons := Check(refs, s.cfg.Facts); len(reasons) > 0 {
		return nil, errNotEligible(reasonStrings(reasons), "secret refs refused: %v", reasonStrings(reasons))
	}
	if s.cfg.RedactorInstalled == nil || !s.cfg.RedactorInstalled() || s.cfg.Registry == nil || s.cfg.Resolver == nil {
		return nil, errResolve("the executor redactor is not the registry redactor; nothing was resolved")
	}
	if s.cfg.Mode == ModeAgentNode && s.cfg.AgeKeyPath == "" {
		return nil, errResolve("this node has no age identity; nothing was resolved")
	}
	if err := s.cfg.Resolver.Probe(ctx); err != nil {
		return nil, err
	}
	vals, err := s.resolve(ctx, refs)
	if err != nil {
		return nil, err
	}
	return s.admit(vals)
}

// resolve asks the cli for all refs, and per ref when the compat source may fill a gap.
func (s *Source) resolve(ctx context.Context, refs []Ref) ([]Value, error) {
	vals, err := s.cfg.Resolver.Resolve(ctx, s.cfg.Attempt, refs)
	if err == nil {
		return vals, nil
	}
	if !s.legacyPermitted() || causeOf(err) != "E541" {
		return nil, err
	}
	var out []Value
	drop := func() {
		for _, v := range out {
			zero(v.Bytes)
		}
	}
	for _, r := range refs {
		got, e := s.cfg.Resolver.Resolve(ctx, s.cfg.Attempt, []Ref{r})
		if e == nil {
			out = append(out, got...)
			continue
		}
		if causeOf(e) != "E541" {
			drop()
			return nil, e
		}
		b, ok := s.cfg.Legacy.Lookup(r)
		if !ok {
			drop()
			return nil, e
		}
		out = append(out, Value{Ref: r, Bytes: b, Source: LegacySource})
		if err := s.cfg.Audit(Event{Kind: EventLegacyEnv, Attempt: s.cfg.Attempt, Ref: r.Raw, Source: LegacySource}); err != nil {
			drop()
			return nil, errResolve("the legacy environment source could not be audited, so it was not used")
		}
	}
	return out, nil
}

func causeOf(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Cause
	}
	return ""
}

// admit registers every value with the redactor, then takes ownership of the buffers.
func (s *Source) admit(vals []Value) (map[string]string, error) {
	for _, v := range vals {
		if err := s.cfg.Registry.Register(v.Bytes); err != nil {
			for _, w := range vals {
				zero(w.Bytes)
			}
			return nil, errResolve("%s was not registered with the redactor, so the attempt does not start", echo(v.Ref.Raw))
		}
	}
	out := make(map[string]string, len(vals))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range vals {
		out[v.Ref.Name] = string(v.Bytes)
		s.bufs = append(s.bufs, v.Bytes)
		s.sources[v.Ref.Raw] = v.Source
	}
	return out, nil
}

// Sources maps each resolved ref to its evidence source label (`age:<file>` or `legacy-env`).
func (s *Source) Sources() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.sources))
	for k, v := range s.sources {
		out[k] = v
	}
	return out
}

// Zero overwrites every buffer the Source holds. It is idempotent and runs at attempt end on every path.
func (s *Source) Zero() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.bufs {
		zero(b)
	}
	s.bufs = nil
}

// Held reports how many value buffers are live (for tests and the leak check).
func (s *Source) Held() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.bufs) }

// FactsFor derives the runner facts a personal-local Source uses for the operator's own node.
func FactsFor(trustClass model.TrustClass, runner trust.RunnerFacts, policy trust.Effective) Facts {
	allowed, has := AllowlistOf(policy)
	return Facts{Job: trust.JobFacts{Trust: trustClass}, Runner: runner, Allowed: allowed, HasPolicy: has}
}
