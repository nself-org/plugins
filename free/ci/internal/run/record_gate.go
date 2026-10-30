package run

// Purpose: compose record-pool admission and publication checks with attest.Publish.
// Inputs: the session, protected project policy and durable attempt lease holders.
// Outputs: exclusions before delivery and E809 publication refusals with evidence.
// Constraints: V15 only; a missing fact never authorizes a status of record.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/autoworld"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/pools/record"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// RecordFacts are trusted facts used by the publication gate.
type RecordFacts struct {
	Record      bool
	Policy      trust.Effective
	Runners     []pools.Runner
	ReportOwner string
	Role        report.Role
}

// RecordGate checks all contributing attempts, independently of placement.
type RecordGate struct {
	Facts func(context.Context, PublishRequest) (RecordFacts, error)
	// Contexts checks the context selected by the status path against protection.
	Contexts *report.ResultContexts
}

func recordRefused(detail string) error {
	return fmt.Errorf("%s: %s: %s", record.Code, record.Reason, detail)
}

// Gate vetoes dev runs, missing facts, foreign reporting ownership and any
// attempt outside the effective record pools. It never changes the publisher.
func (g RecordGate) Gate(ctx context.Context, r PublishRequest, status report.Status) error {
	if !compat.V15() {
		return nil
	}
	if g.Facts == nil {
		return recordRefused("pool facts unavailable")
	}
	f, err := g.Facts(ctx, r)
	if err != nil {
		return recordRefused("pool facts unreadable")
	}
	if !f.Record {
		return recordRefused("dev run")
	}
	if f.ReportOwner == report.OwnerTeam && f.Role != report.RoleTeam {
		return recordRefused(report.SuppressedReason)
	}
	if len(f.Runners) == 0 {
		return recordRefused("no contributing runner facts")
	}
	for _, runner := range f.Runners {
		if !record.Allows(record.FromPolicy(f.Policy), runner) {
			return recordRefused("a contributing attempt is outside pools.record")
		}
	}
	if g.Contexts != nil && r.Owner != "" && r.Repo != "" {
		want := g.Contexts.ResolveResultContext(ctx, r.Owner, r.Repo, "")
		if status.ContextName() != want {
			return fmt.Errorf("result context must be %s; nothing posted", want)
		}
	}
	return nil
}

type recordHooks struct{ sessions sync.Map }

func init() {
	h := &recordHooks{}
	prev := Dispatch
	Dispatch = func(ctx context.Context, spec AttemptSpec) (Admitted, error) {
		if compat.V15() {
			if s := SessionFrom(ctx); s != nil {
				h.sessions.Store(s.RunID, s)
			}
		}
		return prev(ctx, spec)
	}
	RegisterPublishGate(RecordGate{Facts: h.facts, Contexts: &report.ResultContexts{Read: (report.GH{}).RequiredPinned}})
	RegisterAfterRun(h)
	RegisterFinalizer(recordCleanup{h})
	if err := autoworld.RegisterExclusion("pools.record", recordWorldExclusions); err != nil {
		panic(err)
	}
}

func recordWorldExclusions(ctx context.Context, w sched.World) (map[string][]model.PlacementReasonDetail, error) {
	if !compat.V15() {
		return nil, nil
	}
	s := SessionFrom(ctx)
	if s == nil {
		return nil, nil // plans are wired by the command composition root
	}
	c := pools.Config{Labels: s.Store, Team: func() bool { return report.ProcessRole() == report.RoleTeam }, Dev: func() bool { return !s.Record }}
	if !s.Record {
		return pools.ExclusionProvider(c)(ctx, w)
	}
	return record.ExclusionProvider(c, func(context.Context) bool { return s.Record })(ctx, w)
}

func (h *recordHooks) facts(ctx context.Context, r PublishRequest) (RecordFacts, error) {
	v, ok := h.sessions.Load(r.Run.RunID)
	if !ok {
		return RecordFacts{}, recordRefused("session unavailable")
	}
	s := v.(*Session)
	f := RecordFacts{Record: s.Record, Role: report.ProcessRole()}
	if !s.Record {
		return f, nil
	}
	if s.Store == nil {
		return f, recordRefused("store unavailable")
	}
	row, ok, err := s.Store.PipelineByID(ctx, s.RunID)
	if err != nil || !ok {
		return f, recordRefused("pipeline unavailable")
	}
	resolver := policy.Resolver{Store: s.Store, Run: policy.Runner(report.Capture)}
	pol, err := resolver.Resolve(ctx, policy.Repo{Root: row.Project, Project: policy.ProjectIDGitCached(ctx, resolver.Run, row.Project), Class: model.TrustClass(row.SourceTrust), Visibility: "unknown"}, policy.Revision{Dir: row.Project, Run: s.PolicyRun})
	if err != nil {
		return f, err
	}
	f.Policy = pol.Effective
	if owner, _, ok := f.Policy.Value("providers.report_owner"); ok {
		f.ReportOwner, _ = owner.(string)
	}
	f.Runners, err = recordRunners(ctx, s.Store, s.RunID, f.Role)
	return f, err
}

func recordRunners(ctx context.Context, st *store.Store, runID string, role report.Role) ([]pools.Runner, error) {
	attempts, err := st.PipelineAttempts(ctx, runID)
	if err != nil {
		return nil, err
	}
	var runners []pools.Runner
	for _, a := range attempts {
		if a.State == "queued" {
			continue
		}
		facts, ok, err := (store.AttestLedger{S: st}).AttemptFacts(ctx, a.ID)
		if err != nil || !ok {
			return nil, recordRefused("attempt holder unavailable")
		}
		r := pools.Runner{ID: facts.RunnerID, Source: pools.SourceRegistry}
		if role == report.RoleTeam {
			r.Source = pools.SourceCoordinator
		}
		if r.ID == "local" {
			r.Source = pools.SourceLocal
		} else {
			n, err := st.NodeGet(ctx, r.ID)
			var cap model.Capability
			if err != nil || json.Unmarshal(n.Capability, &cap) != nil {
				return nil, recordRefused("runner fact unavailable")
			}
			if cap.Location.Kind.Value != nil && *cap.Location.Kind.Value == "hosted" {
				r.Source = pools.SourceProvider
			}
		}
		set, err := st.NodeLabels(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		r.Assigned = labels.Strings(set)
		runners = append(runners, r)
	}
	return runners, nil
}

// BeforeStatus records refusals before the post is considered.
func (*recordHooks) BeforeStatus() {}

func (h *recordHooks) AfterRun(ctx context.Context, r RunInfo) error {
	if !compat.V15() {
		return nil
	}
	err := (RecordGate{Facts: h.facts}).Gate(ctx, PublishRequest{Run: r}, report.Status{})
	if err == nil {
		return nil
	}
	if v, ok := h.sessions.Load(r.RunID); ok {
		s := v.(*Session)
		if s.Store != nil {
			ev := r.Evidence
			ev.Errors = append(ev.Errors, model.EvidenceError{Code: record.Code, Detail: record.Reason})
			body, e := json.Marshal(ev)
			if e == nil {
				e = s.Store.RecordRunEvidence(ctx, "ev-record-"+r.RunID, r.RunID, r.Revision, ev.InputDigests.Config, body)
			}
			if e != nil {
				return fmt.Errorf("%w; record refusal evidence could not be stored", err)
			}
		}
	}
	return err
}

type recordCleanup struct{ h *recordHooks }

func (recordCleanup) Name() string { return "record-pools" }
func (c recordCleanup) Finalize(_ context.Context, r RunInfo) error {
	c.h.sessions.Delete(r.RunID)
	return nil
}
