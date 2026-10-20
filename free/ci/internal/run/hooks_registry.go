package run

import (
	"sort"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

// Registered is the registry other packages write from init().
var Registered Hooks

// Register helpers mirror the Hooks fields; a second registration of a
// single-slot hook replaces the first.
func RegisterRemote(r RemoteResolver)     { Registered.Remote = append(Registered.Remote, r) }
func RegisterTrust(c TrustClassifier)     { Registered.Trust = c }
func RegisterPolicy(p PolicyResolver)     { Registered.Policy = p }
func RegisterFinalizer(f Finalizer)       { Registered.Finalizers = append(Registered.Finalizers, f) }
func RegisterCache(c CacheHook)           { Registered.Cache = c }
func RegisterServices(s ServicesHook)     { Registered.Services = s }
func RegisterAdhoc(a AdhocHandler)        { Registered.Adhoc = a }
func RegisterImage(i ContainerImageHook)  { Registered.Image = i }
func RegisterPublisher(p Publisher)       { Registered.Publisher = p }
func RegisterIsolation(i ciexec.Isolator) { Registered.Isolation = i }
func RegisterEvidence(b EvidenceBuilder)  { Registered.Evidence = b }
func RegisterLogs(h LogHook)              { Registered.Logs = h }
func RegisterAttemptStart(h AttemptStartHook) {
	Registered.AttemptStarts = append(Registered.AttemptStarts, h)
}
func RegisterAfterRun(h AfterRunHook) { Registered.AfterRuns = append(Registered.AfterRuns, h) }

// RegisterRedactor sets the per-attempt Redactor factory (single slot).
func RegisterRedactor(f func() ciexec.Redactor) { Registered.NewRedactor = f }

// Resolved fills every nil single-slot field from Registered and appends the
// registered lists, so a test can add a fake on top of the registry.
func (h Hooks) Resolved() Hooks {
	r := Registered
	out := h
	if out.Trust == nil {
		out.Trust = r.Trust
	}
	if out.Policy == nil {
		out.Policy = r.Policy
	}
	if out.Cache == nil {
		out.Cache = r.Cache
	}
	if out.Services == nil {
		out.Services = r.Services
	}
	if out.Adhoc == nil {
		out.Adhoc = r.Adhoc
	}
	if out.Image == nil {
		out.Image = r.Image
	}
	if out.Publisher == nil {
		out.Publisher = r.Publisher
	}
	if out.Isolation == nil {
		out.Isolation = r.Isolation
	}
	if out.NewRedactor == nil {
		out.NewRedactor = r.NewRedactor
	}
	if out.Evidence == nil {
		out.Evidence = r.Evidence
	}
	if out.Logs == nil {
		out.Logs = r.Logs
	}
	out.Remote = append(append([]RemoteResolver(nil), h.Remote...), r.Remote...)
	out.Finalizers = append(append([]Finalizer(nil), h.Finalizers...), r.Finalizers...)
	out.AttemptStarts = append(append([]AttemptStartHook(nil), h.AttemptStarts...), r.AttemptStarts...)
	out.AfterRuns = append(append([]AfterRunHook(nil), h.AfterRuns...), r.AfterRuns...)
	out.Outcomes = append(append([]OutcomeHook(nil), h.Outcomes...), r.Outcomes...)
	out.Workspaces = append(append([]WorkspaceHook(nil), h.Workspaces...), r.Workspaces...)
	out.AttemptFinalizes = append(append([]AttemptFinalizeHook(nil), h.AttemptFinalizes...), r.AttemptFinalizes...)
	out.Auto = h.Auto.resolved(r.Auto)
	sort.SliceStable(out.Finalizers, func(i, j int) bool { return out.Finalizers[i].Name() < out.Finalizers[j].Name() })
	return out
}
