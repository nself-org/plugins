package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// JobSpec is the local execution request assembled by the coordinator.
type JobSpec struct {
	AttemptID, CoordinatorID string
	Job                      model.Job
	Command                  []string
	SourceDir                string
	Mounts                   []Mount
	Home                     string
	Timeout                  time.Duration
	Network                  model.NetworkScope
	CPU                      float64
	MemoryMB                 int
	Output                   func([]byte)
	// Slot is the execution slot the attempt holds; isolation profiles derive the job UID from it.
	Slot int
	// Plan, when an Isolator sets it, builds the container command instead of the baseline flags.
	Plan ContainerPlanner
}

// AttemptResult is the local outcome before durable store finalization.
type AttemptResult struct {
	Attempt           model.Attempt
	ExitCode          int
	Started, Finished time.Time
	Err               error
	// Reason refines the failure class: "resource_limit" when a planned container hit its
	// memory or process limit.
	Reason string
}

// Executor runs each local job through one process or Docker funnel.
type Executor struct {
	Env      EnvPolicy
	Redactor Redactor
	// NewRedactor returns fresh streaming state for each concurrent attempt.
	NewRedactor  func() Redactor
	Secrets      SecretSource
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
	local := Executor{Env: e.Env, Redactor: e.Redactor, NewRedactor: e.NewRedactor, Secrets: e.Secrets, Isolation: e.Isolation, Stale: e.Stale, Grace: e.Grace}
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
		e.Isolation = baselineIsolator{}
	}
	if err := e.Isolation.Prepare(ctx, &s); err != nil {
		// a node that cannot isolate is not the job's fault
		result.Err, result.Attempt.FailureClass = err, "infra"
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
	if s.SourceDir != "" {
		if err := copyTree(s.SourceDir, workspace); err != nil {
			result.Err = err
			return
		}
	}
	if e.Stale != nil {
		if err := e.sweep(ctx, s.CoordinatorID, s.Home); err != nil {
			result.Err = err
			return
		}
	}
	env, err := e.Env.Environment(ctx, s.Job)
	if err != nil {
		result.Err = err
		return
	}
	secrets, err := e.Secrets.Resolve(ctx, s.Job.Secrets)
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
	for k, v := range secrets {
		if !safeID.MatchString(k) || strings.Contains(k, ".") {
			result.Err = coded("E612", "invalid secret key")
			return
		}
		env = append(env, k+"="+v)
	}
	if s.Plan != nil {
		result.ExitCode, result.Reason, result.Err = e.runPlanned(ctx, s, workspace, env)
	} else if s.Job.Isolation == "container" {
		result.ExitCode, result.Err = e.runContainer(ctx, s, workspace, env)
	} else {
		result.ExitCode, result.Err = e.runProcess(ctx, s, workspace, env)
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

// copySymlink recreates a symlink whose relative target stays inside the job
// source. An absolute target, or one that climbs out of the source, is refused:
// the job must not reach files outside its workspace through a link.
func copySymlink(path, rel, target string) error {
	link, err := os.Readlink(path)
	if err != nil {
		return err
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(rel), link))
	if filepath.IsAbs(link) || resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return fmt.Errorf("symlink in job source: %s", rel)
	}
	return os.Symlink(link, target)
}

func copyTree(source, dest string) error {
	root, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		if info.Mode()&os.ModeSymlink != 0 {
			return copySymlink(path, rel, target)
		}
		if info.IsDir() {
			return os.Mkdir(target, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file in job source: %s", rel)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()&0700)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, err = out.ReadFrom(in)
		inputCloseErr := in.Close()
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return closeErr
	})
}
