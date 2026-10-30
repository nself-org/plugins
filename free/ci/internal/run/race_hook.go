package run

// Purpose: the run side of race mode (P7-AUTO-14), without run importing
// internal/race: sibling attempts of one job are made by CreateAttempt (the
// only attempt creator), cancelled through the durable lease cancel intent
// (the node kills the attempt's process group, NODE-23), and read back.
// Inputs:  a context holding the run Session and an AttemptSpec template.
// Outputs: RaceSibling values carrying the lease epoch that fences a cancel.
// Constraints: a release or idempotent:false job is never raced (CACHE-05
// at-most-once): Create refuses it before anything is persisted. Cancel is
// idempotent: a repeat, or a cancel that lost to the result commit (E605), is
// not an error. Nothing here posts a status.

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// RaceSibling is one admitted sibling attempt and the epoch its lease holds.
type RaceSibling struct {
	Admitted
	N     int
	Epoch int64
}

// RaceAttempts makes and stops the siblings of one job of the session in the context.
type RaceAttempts struct {
	// Spec is the template of every sibling: Pipeline, Job, Foreground and Band; N is set per sibling.
	Spec AttemptSpec
	// Pin carries the runner a sibling is meant for into the dispatch hook's
	// placement request (nil: the context is unchanged).
	Pin func(ctx context.Context, runner string) context.Context
}

// Raceable says job may have sibling attempts: never a release or idempotent:false job.
func Raceable(job *model.Job) bool { return job != nil && retryable(*job) }

// Create makes sibling n through CreateAttempt and reads its lease epoch.
func (r RaceAttempts) Create(ctx context.Context, n int, runner string) (RaceSibling, error) {
	if !Raceable(r.Spec.Job) {
		return RaceSibling{}, fail(CodeRun, "a release or idempotent:false job is never raced")
	}
	s := r.Spec
	s.N = n
	if r.Pin != nil {
		ctx = r.Pin(ctx, runner)
	}
	adm, err := CreateAttempt(ctx, s)
	if err != nil {
		return RaceSibling{}, err
	}
	_, epoch, err := SessionFrom(ctx).Store.AttemptStateEpoch(ctx, adm.AttemptID)
	if err != nil {
		return RaceSibling{}, err
	}
	return RaceSibling{Admitted: adm, N: n, Epoch: epoch}, nil
}

// Cancel writes the lease cancel intent of a sibling, fenced by its epoch.
func (r RaceAttempts) Cancel(ctx context.Context, attemptID string, epoch int64, reason string) error {
	sess := SessionFrom(ctx)
	if sess == nil {
		return fail(CodeRun, "no run session in the context")
	}
	err := retryBusy(func() error { return sess.Store.RequestCancel(ctx, attemptID, "race: "+reason, epoch) })
	if err != nil && CodeOf(err) == codeStoreRef {
		return nil // already requested, the epoch moved or the result is committed: nothing left to stop
	}
	return err
}

// State reads a sibling's state and failure class.
func (r RaceAttempts) State(ctx context.Context, attemptID string) (state, class string, err error) {
	sess := SessionFrom(ctx)
	if sess == nil {
		return "", "", errors.New("no run session in the context")
	}
	attempts, err := sess.Store.PipelineAttempts(ctx, sess.RunID)
	if err != nil {
		return "", "", err
	}
	for _, a := range attempts {
		if a.ID == attemptID {
			return string(a.State), a.FailureClass, nil
		}
	}
	return "", "", fail(CodeRun, "attempt %s is not part of this run", attemptID)
}
