package run

// Purpose: the extension points later Tickets register. Every hook is an
// interface with a no-op default; tests plug fakes into Coordinator.Hooks.
// Inputs:  registrations from init() of other packages (Register*), or a
// Hooks value on a Coordinator.
// Outputs: a Hooks snapshot the run uses.
// Constraints: registrations happen at init, before any run; a run reads
// the snapshot once and never mutates it. CACHE delta 3 fixes the shapes of
// CacheHook, AttemptStart and AfterRun; Finalizers run in name order.

import (
	"context"
	"sort"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Mount is a host path made visible to a job (cache restore).
type Mount = ciexec.Mount

// AttemptInfo describes the attempt a hook is called for.
type AttemptInfo struct {
	RunID, JobID, JobName, AttemptID, Revision string
	N                                          int
	// Check is the catalogue check the job was planned from ("" for a configured job).
	Check          string
	Job            model.Job
	IdempotencyKey string
	// Home is the run's state directory as given in Options ("" = NSELF_CI_HOME or ~/.nself/ci).
	Home string
	// Store is the run's store; hooks read and write their own tables through it.
	Store *store.Store
	// Epoch is the attempt's lease epoch when it started running (0 when the
	// caller did not set it). Hooks that write fenced rows use it, so a stale
	// callback cannot adopt a later incarnation's epoch.
	Epoch int64
	// Container is the run's --container option: every job runs in the toolchain
	// container although Job still names no container (prepare applies it after
	// the cache hook). The cache hook plans container-level mounts for it.
	Container bool
}

// Outcome is how an attempt ended.
type Outcome struct {
	State        model.JobState
	FailureClass model.FailureClass
	ExitCode     int
}

// CacheSetup is what a CacheHook adds to a job.
type CacheSetup struct {
	Env    map[string]string
	Mounts []Mount
}

// CacheHook restores a cache before the job and saves it after.
type CacheHook interface {
	Before(ctx context.Context, a AttemptInfo) (CacheSetup, error)
	After(ctx context.Context, a AttemptInfo, o Outcome) error
}

// AttemptStartHook runs before the job starts (artifact declarations). An
// error fails the job with the error's code.
type AttemptStartHook interface {
	AttemptStart(ctx context.Context, a AttemptInfo) error
}

// RunInfo describes a finished run to AfterRun hooks and Finalizers.
type RunInfo struct {
	RunID, Revision, EvidenceDigest string
	Result                          model.Result
	Evidence                        model.Evidence
	Jobs                            []JobOutcome
	Dirty                           bool
}

// AfterRunHook runs after the pipeline evidence is recorded.
type AfterRunHook interface {
	AfterRun(ctx context.Context, r RunInfo) error
}

// Finalizer closes a run; finalizers run in Name order.
type Finalizer interface {
	Name() string
	Finalize(ctx context.Context, r RunInfo) error
}

// TrustRequest asks the classifier how far the source is trusted.
type TrustRequest struct {
	Root, Revision, Requested string
}

// TrustClassifier decides `--trust` (owner needs the real one, TRUST delta 2).
type TrustClassifier interface {
	Classify(ctx context.Context, r TrustRequest) (model.TrustClass, error)
}

// PolicyRequest is the input of the policy resolver, called once per pipeline.
type PolicyRequest struct {
	Root, Pipeline string
	Config         map[string]any
}

// PolicyResult carries the digest stored on the pipeline row and in evidence.
type PolicyResult struct{ Digest string }

// PolicyResolver resolves the effective policy (P7-TRUST-27 registers it).
type PolicyResolver interface {
	Resolve(ctx context.Context, r PolicyRequest) (PolicyResult, error)
}

// RunView is the read model `ci runs` prints.
type RunView struct {
	ID       string    `json:"id"`
	Revision string    `json:"revision"`
	Status   string    `json:"status"`
	Jobs     []JobView `json:"jobs,omitempty"`
}

// JobView is one job of a RunView.
type JobView struct {
	Name         string `json:"name"`
	State        string `json:"state"`
	FailureClass string `json:"failure_class,omitempty"`
}

// RemoteResolver finds runs that live on another machine; it is consulted
// before the local store by `ci runs`, `ci evidence` and `ci logs`.
type RemoteResolver interface {
	Resolve(ctx context.Context, ref string) (RunView, bool, error)
}

// ServicesRequest asks for the services a job needs.
type ServicesRequest struct {
	RunID string
	Names []string
}

// ServicesSetup is what started services add to a job.
type ServicesSetup struct{ Env map[string]string }

// ServicesHook starts disposable services (P7-CI-42).
type ServicesHook interface {
	Up(ctx context.Context, r ServicesRequest) (ServicesSetup, error)
	Down(ctx context.Context, r ServicesRequest) error
}

// AdhocHandler runs `nself ci -- <cmd...>` (P7-CI-52).
type AdhocHandler interface {
	Adhoc(ctx context.Context, o Options, argv []string) (exit int, err error)
}

// ContainerImageHook names the pinned toolchain image for --container (P7-CI-43).
type ContainerImageHook interface {
	Image(ctx context.Context, a AttemptInfo) (string, error)
}

// PublishRequest is what the Publisher turns into a status.
type PublishRequest struct {
	Owner, Repo string
	Run         RunInfo
	Projection  model.ProjectionResult
}

// Publisher builds the status that is posted (P7-TRUST registers
// attest.Publish). The default is the model projection plus the evidence digest.
type Publisher interface {
	Publish(ctx context.Context, r PublishRequest) (report.Status, error)
}

// EvidenceFacts is what an EvidenceBuilder sees of a finished run.
type EvidenceFacts struct {
	Root, RunID string
	Rev         Revision
	Store       *store.Store
	Git         Capture // runs git through the exec funnel
	Jobs        []JobOutcome
	Job         string // --job: only that job (and what it needs) ran
}

// EvidenceBuilder completes the run evidence (P7-CI-38). Begin runs before any
// job starts and returns a token (the input digests and the binding as the run
// began); Complete receives that token with the base document and returns the
// final one. A Complete error leaves the base document, which carries no input
// digests and so never reads as current.
type EvidenceBuilder interface {
	Begin(ctx context.Context, root string, rev Revision, git Capture) any
	Complete(ctx context.Context, begin any, f EvidenceFacts, ev model.Evidence) (model.Evidence, error)
}

// Hooks is a snapshot of every hook. A nil field means "no-op default".
type Hooks struct {
	Remote        []RemoteResolver
	Trust         TrustClassifier
	Policy        PolicyResolver
	Finalizers    []Finalizer
	Cache         CacheHook
	Services      ServicesHook
	Adhoc         AdhocHandler
	Image         ContainerImageHook
	Publisher     Publisher
	Isolation     ciexec.Isolator
	NewRedactor   func() ciexec.Redactor // fresh per-attempt state; every output chunk passes it (P7-TRUST-19)
	Evidence      EvidenceBuilder
	Logs          LogHook // stores each attempt's redacted output (P7-CI-39)
	AttemptStarts []AttemptStartHook
	AfterRuns     []AfterRunHook
	Outcomes      []OutcomeHook
	// Workspaces and AttemptFinalizes are the per-attempt seams of
	// hooks_attempt_end.go (P7-CACHE-03).
	Workspaces       []WorkspaceHook
	AttemptFinalizes []AttemptFinalizeHook
}

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
	sort.SliceStable(out.Finalizers, func(i, j int) bool { return out.Finalizers[i].Name() < out.Finalizers[j].Name() })
	return out
}

// guard calls a hook and turns a panic into an E627 error, so one bad hook
// cannot kill the coordinator and orphan the other jobs.
func guard(name string, f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fail(CodeOptions, "%s panicked: %v", name, r)
		}
	}()
	return f()
}
