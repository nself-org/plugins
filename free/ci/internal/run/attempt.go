package run

// Purpose: the only function that creates an attempt (SCHED F14/F15). The
// run loop, P7-CI-40 retry, P7-CI-41 reproduce and P7-CI-52 ad hoc call
// CreateAttempt and nothing else; the one AdmitAndLease call outside the
// store, lease and dispatch packages lives here.
// Inputs:  an AttemptSpec and a context holding the run Session.
// Outputs: an Admitted attempt: leased, with a runner, ready to run.
// Constraints: CreateAttempt blocks until the attempt is admitted, the
// context is done (ctx.Err()), or it is refused (E618, E693, E695, or a
// model.Reason admission refusal); it never leaves a partial attempt: a
// refused attempt is cancelled before the error returns.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// AttemptSpec asks for one attempt of one job.
type AttemptSpec struct {
	Pipeline       *model.Pipeline
	Job            *model.Job
	N              int
	Foreground     bool
	Band, Previous string
}

// Admitted is an attempt that holds a lease.
type Admitted struct {
	AttemptID, LeaseID, RunnerID string
	Local                        bool
}

// DispatchHook places an attempt (P7-SCHED/P7-NODE). It blocks until the
// attempt is admitted, ctx is done (ctx.Err()), or it is refused with E618,
// E693 or E695; it never returns a partial attempt. The attempt row already
// exists; AttemptIDFor names it. LocalAdmit is the local path a hook may use.
type DispatchHook func(ctx context.Context, s AttemptSpec) (Admitted, error)

// Dispatch is nil until the scheduler registers: then admission is local.
var Dispatch DispatchHook

// PlacementRef returns the placement record of an attempt (P7-SCHED-05 sets it).
var PlacementRef func(ctx context.Context, attemptID string) (ref string, ok bool)

// AdmissionRefused is a local refusal that is a reason, not a failure.
type AdmissionRefused struct{ Reason model.Reason }

func (e *AdmissionRefused) Error() string { return "admission refused: " + string(e.Reason) }

// AttemptIDFor is the id CreateAttempt gives the attempt of s.
func AttemptIDFor(ctx context.Context, s AttemptSpec) (string, error) {
	id, _, err := attemptIDs(ctx, s)
	return id, err
}

func attemptIDs(ctx context.Context, s AttemptSpec) (attemptID, jobRow string, err error) {
	sess := SessionFrom(ctx)
	if sess == nil {
		return "", "", fail(CodeRun, "no run session in the context")
	}
	if s.Pipeline == nil || s.Job == nil || s.N < 1 {
		return "", "", fail(CodeRun, "attempt spec needs a pipeline, a job and N >= 1")
	}
	row, ok := sess.jobRow(s.Job)
	if !ok {
		return "", "", fail(CodeRun, "job is not part of this run")
	}
	return fmt.Sprintf("%s.a%d", row, s.N), row, nil
}

// idempotencyKey is set for jobs that must run at most once.
func idempotencyKey(job *model.Job, attemptID string) string {
	if job.Kind != "release" && (job.Idempotent == nil || *job.Idempotent) {
		return ""
	}
	sum := sha256.Sum256([]byte("nself-ci-idempotency\x00" + attemptID))
	return hex.EncodeToString(sum[:])
}

func keyDigest(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// CreateAttempt creates, admits and leases one attempt.
func CreateAttempt(ctx context.Context, s AttemptSpec) (Admitted, error) {
	id, row, err := attemptIDs(ctx, s)
	if err != nil {
		return Admitted{}, err
	}
	sess := SessionFrom(ctx)
	key := idempotencyKey(s.Job, id)
	if err = sess.Store.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: row, CoordinatorID: sess.Coordinator.ID, N: s.N}, keyDigest(key)); err != nil {
		return Admitted{}, err
	}
	var adm Admitted
	if Dispatch != nil {
		adm, err = Dispatch(ctx, s)
	} else {
		adm, err = LocalAdmit(ctx, s)
	}
	if err != nil {
		bg, done := background(ctx)
		defer done()
		if state, epoch, e := sess.Store.AttemptStateEpoch(bg, id); e == nil && state == "queued" {
			_ = sess.Store.Transition(bg, id, "queued", "cancelled", store.TransitionDetail{Epoch: epoch, FailureClass: "cancelled", Reason: "admission refused"})
		}
		return Admitted{}, err
	}
	if adm.AttemptID == "" {
		adm.AttemptID = id
	}
	sess.mu.Lock()
	sess.local[adm.AttemptID] = adm.Local
	sess.mu.Unlock()
	return adm, nil
}

// LocalAdmit leases the queued attempt of s on this host, waiting while the
// host reservation is full. It is the nil-Dispatch path and the one a
// dispatch hook falls back to for a local placement.
func LocalAdmit(ctx context.Context, s AttemptSpec) (Admitted, error) {
	id, _, err := attemptIDs(ctx, s)
	if err != nil {
		return Admitted{}, err
	}
	sess := SessionFrom(ctx)
	mode := ciexec.Background
	if s.Foreground {
		mode = ciexec.Foreground
	}
	for {
		capacity := sess.Capacity()
		dec := ciexec.Admit(*s.Job, mode, capacity, sess.Admission)
		if !dec.Allowed {
			return Admitted{}, &AdmissionRefused{Reason: dec.Reason}
		}
		cpu, mem := 1, 256
		if r := s.Job.Requirements; r != nil {
			cpu, mem = max(cpu, r.CPU), max(mem, r.MemMB)
		}
		state, epoch, err := sess.Store.AttemptStateEpoch(ctx, id)
		if err != nil || state != "queued" {
			return Admitted{}, fail(CodeRun, "attempt %s is not queued", id)
		}
		nonce := make([]byte, 16)
		if _, err = rand.Read(nonce); err != nil {
			return Admitted{}, err
		}
		token := sha256.Sum256(append([]byte(id), nonce...))
		d := store.Demand{AttemptID: id, RunnerID: "local", LeaseID: "lease-" + id, TokenDigest: hex.EncodeToString(token[:]),
			CPU: cpu, MemMB: mem, Epoch: epoch,
			CapacityCPU: max(dec.Slots*cpu, cpu), CapacityMemMB: max(capacity.FreeMemoryMB-capacity.MemoryMB*dec.ReserveMemoryPercent/100, mem)}
		err = sess.Store.AdmitAndLease(ctx, sess.Host, d, sess.Store.ReserveBudget)
		if err == nil {
			return Admitted{AttemptID: id, LeaseID: d.LeaseID, RunnerID: d.RunnerID, Local: true}, nil
		}
		var se *store.Error
		if !errors.As(err, &se) || se.Code != "E604" || !strings.Contains(se.Message, "capacity") {
			return Admitted{}, err
		}
		select {
		case <-ctx.Done():
			return Admitted{}, ctx.Err()
		case <-time.After(sess.Poll):
		}
	}
}
