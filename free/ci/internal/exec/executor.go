package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// AttemptResult is the local outcome before durable store finalization.
type AttemptResult struct {
	Attempt           model.Attempt
	ExitCode          int
	Started, Finished time.Time
	Err               error
	// Reason refines the failure class: "resource_limit" when a planned container hit its
	// memory or process limit.
	Reason string
	// Signal names the signal that ended the job (SIGSEGV, SIGKILL, ...); "" when it exited
	// on its own. ExitCode is -1 then. Diagnosis tells a crash from an external kill by it.
	Signal string
	// Refused is true when the Isolator refused the attempt in Prepare: nothing ran, no workspace
	// exists, no hook or secret was touched, so the attempt can be offered again.
	Refused bool
}

// Executor runs each local job through one process or Docker funnel.
type Executor struct {
	Env      EnvPolicy
	Redactor Redactor
	// NewRedactor returns fresh streaming state for each concurrent attempt.
	NewRedactor func() Redactor
	Secrets     SecretSource
	// RegisterSecret, when set, receives every non-empty resolved secret value
	// before the job starts, so the redaction registry (and the artifact and cache
	// scanners that read it) knows the value. An error fails the attempt: a
	// secret that cannot be masked is never handed to a job. It must not keep
	// the value in an error.
	RegisterSecret func(value []byte) error
	// Isolation is required. A missing policy refuses execution before hooks or secrets.
	Isolation    Isolator
	Stale        StaleChecker
	Grace        time.Duration
	redactorOnce sync.Once
	redactorSlot chan struct{}
}

// Run confines a job to a fresh private workspace and removes it on every exit.
func (e *Executor) Run(ctx context.Context, s JobSpec) (result AttemptResult) {
	if e.NewRedactor == nil && e.Redactor != nil {
		// Legacy direct hooks are serialized because their state may span chunks.
		e.redactorOnce.Do(func() { e.redactorSlot = make(chan struct{}, 1); e.redactorSlot <- struct{}{} })
		slot := e.redactorSlot
		select {
		case <-ctx.Done():
			return AttemptResult{Attempt: model.Attempt{ID: s.AttemptID, State: "cancelled", FailureClass: "cancelled"}, Err: ctx.Err()}
		case <-slot:
		}
		defer func() { slot <- struct{}{} }()
		if err := ctx.Err(); err != nil {
			return AttemptResult{Attempt: model.Attempt{ID: s.AttemptID, State: "cancelled", FailureClass: "cancelled"}, Err: err}
		}
	}
	local := Executor{Env: e.Env, Redactor: e.Redactor, NewRedactor: e.NewRedactor, Secrets: e.Secrets, RegisterSecret: e.RegisterSecret, Isolation: e.Isolation, Stale: e.Stale, Grace: e.Grace}
	e = &local
	if e.NewRedactor != nil {
		e.Redactor = e.NewRedactor()
	}
	result.Attempt = model.Attempt{ID: s.AttemptID, State: "failed"}
	result.Started = time.Now()
	defer func() { result.Finished = time.Now() }()
	if !safeID.MatchString(s.AttemptID) || !safeID.MatchString(s.CoordinatorID) || len(s.Command) == 0 {
		result.Err = coded("E612", "invalid attempt, coordinator or command")
		return
	}
	if err := checkMounts(s); err != nil {
		result.Err = err
		return
	}
	if e.Isolation == nil {
		result.Err = coded("E611", "an explicit isolation policy is required")
		result.Attempt.FailureClass, result.Refused = "infra", true
		return
	}
	perr := e.Isolation.Prepare(ctx, &s)
	if s.Release != nil { // set even when Prepare failed late: whatever it created is removed
		release := s.Release
		defer func() { release(context.WithoutCancel(ctx)) }()
	}
	if perr != nil {
		// a node that cannot isolate is not the job's fault
		result.Err, result.Attempt.FailureClass, result.Refused = perr, "infra", true
		return
	}
	if e.Env == nil {
		e.Env = inheritEnvironment{}
	}
	if e.Secrets == nil {
		e.Secrets = emptySecrets{}
	}
	if e.Redactor == nil {
		e.Redactor = passThroughRedactor{}
	}
	if len(s.Masks) > 0 {
		e.Redactor = newValueMask(e.Redactor, s.Masks)
	}
	if e.Grace <= 0 {
		e.Grace = 10 * time.Second
	}
	if s.Timeout <= 0 && s.Job.Timeout != "" {
		var err error
		s.Timeout, err = time.ParseDuration(s.Job.Timeout)
		if err != nil {
			result.Err = coded("E612", "invalid timeout")
			return
		}
	}
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}
	if s.Home == "" {
		s.Home = os.Getenv("NSELF_CI_HOME")
	}
	if s.Home == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			result.Err = err
			return
		}
		s.Home = filepath.Join(home, ".nself", "ci")
	}
	base := filepath.Join(s.Home, "work")
	if err := os.MkdirAll(base, 0700); err != nil {
		result.Err = err
		return
	}
	workspace := filepath.Join(base, s.AttemptID)
	if err := os.Mkdir(workspace, 0700); err != nil {
		result.Err = err
		return
	}
	defer func() {
		if err := os.RemoveAll(workspace); err != nil && result.Err == nil && result.Attempt.State == "passed" {
			// only a clean run is turned into a failure; a failed or cancelled attempt keeps its own class
			result.Err = coded("E610", "workspace cleanup failed: "+err.Error())
			result.Attempt.State = "failed"
			result.Attempt.FailureClass = "infra"
		}
	}()
	pinned, err := os.OpenRoot(workspace) // pinned before the job runs: AfterJob works on this inode
	if err != nil {
		result.Err = err
		return
	}
	defer func() { _ = pinned.Close() }()
	if s.SourceDir != "" {
		if err := copyTree(s.SourceDir, workspace); err != nil {
			result.Err = err
			return
		}
	}
	if e.Stale != nil {
		if err := e.sweepOnStart(ctx, s.CoordinatorID, s.Home); err != nil {
			result.Err = err
			return
		}
	}
	if s.BeforeJob != nil {
		if err := s.BeforeJob(ctx, pinned); err != nil {
			result.Err = err
			result.Attempt.FailureClass = "infra"
			return
		}
	}
	policy := e.Env
	if s.Env != nil {
		policy = s.Env
	}
	env, err := policy.Environment(ctx, s.Job)
	if err != nil {
		result.Err = err
		return
	}
	source := e.Secrets
	if s.Secrets != nil {
		source = s.Secrets
	}
	if s.Plan != nil || s.Job.Isolation == "container" {
		env = containerEnv(env)
	}
	secrets, err := source.Resolve(ctx, s.Job.Secrets)
	if err != nil {
		result.Err = err
		return
	}
	for k, v := range s.Job.Env {
		if !safeID.MatchString(k) || strings.Contains(k, ".") {
			result.Err = coded("E612", "invalid environment key")
			return
		}
		env = append(env, k+"="+v)
	}
	env = append(env, s.NetEnv...)
	var secretEnv []string // secret values travel apart from the env file (P7-TRUST-02: no secret on disk)
	for k, v := range secrets {
		if !safeID.MatchString(k) || strings.Contains(k, ".") || unsafeSecretName(k) {
			result.Err = coded("E612", "invalid or reserved secret key")
			return
		}
		if e.RegisterSecret != nil && v != "" {
			if err := e.RegisterSecret([]byte(v)); err != nil {
				result.Err, result.Attempt.FailureClass = coded("E612", "a job secret could not be registered for masking; the job did not start"), "infra"
				return
			}
		}
		secretEnv = append(secretEnv, k+"="+v)
	}
	if s.Plan != nil {
		result.ExitCode, result.Reason, result.Err = e.runPlanned(ctx, s, workspace, append(env, secretEnv...))
	} else if s.Job.Isolation == "container" {
		result.ExitCode, result.Err = e.runContainer(ctx, s, workspace, env, secretEnv)
	} else {
		result.ExitCode, result.Signal, result.Err = e.runProcess(ctx, s, workspace, append(env, secretEnv...))
	}
	if s.AfterJob != nil && result.Err == nil && result.ExitCode == 0 && ctx.Err() == nil {
		result.Err = s.AfterJob(ctx, pinned)
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.Attempt.FailureClass = "timeout"
	case errors.Is(ctx.Err(), context.Canceled):
		result.Attempt.FailureClass = "cancelled"
		result.Attempt.State = "cancelled"
	case result.Err != nil:
		result.Attempt.FailureClass = "infra"
	case result.ExitCode != 0:
		result.Attempt.FailureClass = "code"
	default:
		result.Attempt.State = "passed"
	}
	return
}
