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

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/services"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Mount is a host path made visible to a job (cache restore).
type Mount = ciexec.Mount

// AttemptInfo describes the attempt a hook is called for.
type AttemptInfo struct {
	// SourceClass is the classified source trust, before any ad-hoc admission override.
	SourceClass                                model.TrustClass
	RunID, JobID, JobName, AttemptID, Revision string
	N                                          int
	Job                                        model.Job
	IdempotencyKey                             string
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
	Container   bool
	SecretsInfo // project root and effective secrets allowlist (P7-TRUST-02)
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
	Store          string // state.db of the coordinator's home; "" = the default store
	Config         map[string]any
	Run            RunValues // per-run values (the flags of one invocation): only ever tighten, except the preset; nil = none
	// Class is the class the run is admitted at; it picks the default policy scope, so an untrusted
	// source gets the untrusted floor (D-1436). "" = the local operator's own checkout (owner).
	Class model.TrustClass
	// Visibility is "private" only when the repository is known to be private; "" and anything else is
	// "unknown", which keeps hosted execution off.
	Visibility string
}

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
	Name           string `json:"name"`
	State          string `json:"state"`
	FailureClass   string `json:"failure_class,omitempty"`
	AttemptID      string `json:"attempt_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// RemoteResolver finds runs that live on another machine; it is consulted
// before the local store by `ci runs`, `ci evidence` and `ci logs`.
type RemoteResolver interface {
	Resolve(ctx context.Context, ref string) (RunView, bool, error)
}

// ServicesRequest asks for the services one attempt needs.
type ServicesRequest struct {
	RunID string
	// AttemptID is the attempt the services belong to. The hook keys its provisions by it, so two jobs of one
	// run that need the same services each get their own and tear down only their own.
	AttemptID string
	Names     []string
	// Specs are Names resolved against the run's .nself/ci.yaml, read from the repository root rather than the
	// process's working directory. Empty: the hook resolves Names itself.
	Specs []services.Spec
	// Isolation and Network are the level and network scope the attempt runs under, for hooks that attach the
	// job to the services' network.
	Isolation model.Isolation
	Network   model.NetworkScope
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

// RecordedTrustClassifier separates admission of an operator-authored command
// from the source class whose isolation the ad-hoc handler already enforces.
// RecordedTrust may only lower the admitted class, never raise it.
type RecordedTrustClassifier interface {
	RecordedTrust() model.TrustClass
}

// EvidenceStamper adds facts to the base document that must survive a builder error.
// It must not add input digests or a current binding to an incomplete document.
type EvidenceStamper interface {
	Stamp(model.Evidence) model.Evidence
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
	NewRedactor   func() ciexec.Redactor                                        // fresh per-attempt state; every output chunk passes it (P7-TRUST-19)
	NewSecrets    func(AttemptInfo, func() ciexec.Redactor) ciexec.SecretSource // per-attempt secret source (P7-TRUST-02); nil = Registered's
	Evidence      EvidenceBuilder
	Logs          LogHook // stores each attempt's redacted output (P7-CI-39)
	AttemptStarts []AttemptStartHook
	AfterRuns     []AfterRunHook
	Outcomes      []OutcomeHook
	// Workspaces and AttemptFinalizes are the per-attempt seams of
	// hooks_attempt_end.go (P7-CACHE-03).
	Workspaces       []WorkspaceHook
	AttemptFinalizes []AttemptFinalizeHook
	Auto             AutoHooks // PublishGate, PublishDetail, Offload, Race, Notify (hooks_auto.go)
}
