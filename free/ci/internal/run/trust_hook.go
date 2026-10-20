package run

// Purpose: the real TrustClassifier (P7-TRUST-13): it gathers the git facts of the checkout through
// the exec command runner, classifies them with internal/trust/source, applies the machine-invoked
// cap, honours `--trust` and writes one audit row when `--trust owner` overrides the verdict. It
// also classifies a forge trigger (ClassifyForge), re-fetching the facts first. It is the only file
// of the package that may import providers.
// Constraints: a TrustRequest or forge facts in, a class, E625 or E687 out; registered from init(); git runs through Capture (report.Capture); no revision or no git
//          repository is the operator's own directory, as before this hook existed. The class is
//          min(classified, trust.machine_invoked_cap) when NSELF_INVOKED_BY is set; `--trust` may lower
//          the class, only `--trust owner` may raise it, and the audit row is written first: no row, no
//          run. A fact that cannot be gathered or re-fetched stays at its untrusted value.

import (
	"cmp"
	"context"
	"errors"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func init() { RegisterTrust(SourceTrust{}) }

// SourceTrust is the registered classifier. The zero value uses report.Capture, the default state
// store, the environment and providers.RefetchFacts.
type SourceTrust struct {
	Capture   Capture
	OpenStore func() (*store.Store, error)
	Getenv    func(string) string
	Refetch   func(context.Context, model.TriggerFacts) (model.TriggerFacts, error)
}

// Classify implements TrustClassifier for a local run.
func (h SourceTrust) Classify(ctx context.Context, r TrustRequest) (model.TrustClass, error) {
	res, err := h.Decide(ctx, r)
	return res.Class, err
}

// Decide is Classify with the reasons: the class a run gets and why.
func (h SourceTrust) Decide(ctx context.Context, r TrustRequest) (source.Result, error) {
	var res source.Result
	switch {
	case r.Revision == "":
		res = h.noRevision(ctx, r.Root)
	case !fullSHA.MatchString(r.Revision):
		res = source.Result{Class: "untrusted", Reasons: []string{source.ReasonLocalBadRevision}}
	default:
		res = source.ClassifyLocal(h.gather(ctx, r.Root, r.Revision))
	}
	capClass, machine, err := h.machineCap(ctx, r.Root)
	if err != nil {
		return res, err
	}
	if machine {
		res = source.ApplyCap(res, capClass)
	}
	req := model.TrustClass(r.Requested)
	if req == "" {
		return res, nil
	}
	if machine && trust.RankTrust(req) > trust.RankTrust(capClass) {
		return res, &roles.Error{Reason: "machine_cap", Msg: "--trust " + r.Requested + " is above trust.machine_invoked_cap " + string(capClass)}
	}
	if req == "owner" && res.Class != "owner" {
		if err := h.auditOverride(ctx, r, res); err != nil {
			return res, err
		}
		res.Class, res.Reasons = "owner", append(res.Reasons, source.ReasonLocalOverride)
		return res, nil
	}
	if trust.RankTrust(req) <= trust.RankTrust(res.Class) {
		res.Class = req
	}
	return res, nil // a request above the verdict is not granted: the coordinator names the mismatch
}

// ClassifyForge classifies a forge trigger: the facts are re-fetched from the API first, so a
// payload claim never stands in for a fact the operator's token can read. Facts that cannot be
// re-fetched (the fetch failed, or no registered provider can) leave the run untrusted.
func (h SourceTrust) ClassifyForge(ctx context.Context, root string, f model.TriggerFacts) (source.Result, error) {
	refetch, verifiable := h.Refetch, true
	if refetch == nil {
		p, ok := providers.Lookup(f.Source)
		_, can := p.(providers.TrustFacts)
		refetch, verifiable = providers.RefetchFacts, ok && can
	}
	fresh, ferr := refetch(ctx, f)
	if ferr == nil {
		f = fresh
	}
	res := source.Classify(f)
	if ferr != nil || !verifiable {
		res = source.Unverified(res)
	}
	limit, machine, err := h.machineCap(ctx, root)
	if err != nil {
		return res, err
	}
	if machine {
		res = source.ApplyCap(res, limit)
	}
	return res, nil
}

func (h SourceTrust) getenv(k string) string {
	if h.Getenv != nil {
		return h.Getenv(k)
	}
	return os.Getenv(k)
}

func (h SourceTrust) capture() Capture {
	if h.Capture != nil {
		return h.Capture
	}
	return report.Capture
}

func (h SourceTrust) openStore() (*store.Store, error) {
	if h.OpenStore != nil {
		return h.OpenStore()
	}
	return OpenStore("")
}

// machineCap is the effective trust.machine_invoked_cap when the run is machine-invoked (Epic D27).
func (h SourceTrust) machineCap(ctx context.Context, root string) (model.TrustClass, bool, error) {
	if h.getenv("NSELF_INVOKED_BY") == "" {
		return "", false, nil
	}
	st, err := h.openStore()
	if err != nil {
		return "", true, fail(CodeTrust, "the machine-invoked trust cap could not be read: %v", err)
	}
	defer func() { _ = st.Close() }()
	root = cmp.Or(root, ".")
	res := policy.Resolver{Store: st, Run: policy.Runner(h.capture())}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, res.Run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		return "", true, fail(CodeTrust, "the machine-invoked trust cap could not be resolved: %v", err)
	}
	v, _ := got.Values["trust.machine_invoked_cap"].(string)
	return trust.DecodeTrust(cmp.Or(v, "owner")), true, nil
}

// auditOverride writes the one audit row of a `--trust owner` that raised the class.
func (h SourceTrust) auditOverride(ctx context.Context, r TrustRequest, res source.Result) error {
	st, err := h.openStore()
	if err != nil {
		return fail(CodeTrust, "--trust owner needs an audit row and the store could not be opened: %v", err)
	}
	defer func() { _ = st.Close() }()
	ev := audit.Event{Actor: policy.LocalSubject, Action: "trust.override", Target: "revision:" + r.Revision,
		Detail: "requested=owner classified=" + string(res.Class) + " reasons=" + strings.Join(res.Reasons, ",")}
	if err := (&audit.Log{St: st}).Append(ctx, ev); err != nil {
		return fail(CodeTrust, "--trust owner needs an audit row and it could not be written: %v", err)
	}
	return nil
}

// providerRefusal returns the provider error when cause is a refusal a retry cannot fix (P7-HOST-11).
// It lives here because the layering test lets only this file of the package name providers.
func providerRefusal(cause error) (*providers.Error, bool) {
	var pe *providers.Error
	if !errors.As(cause, &pe) {
		return nil, false
	}
	switch pe.Code {
	case "E700", "E701", "E702":
		return pe, pe.Class == providers.Auth || pe.Class == providers.Config
	}
	return nil, false
}
