package job

// Purpose: the operator's way out of a stuck at-most-once attempt (Epic D9):
// `nself ci runs resolve <attempt> --idempotency-key <key> --rerun|--abandon`.
// Inputs:  the attempt id, the key the job received (NSELF_CI_IDEMPOTENCY_KEY),
// the action and the actor.
// Outputs: a Result, or an E716 *Error (key_mismatch, not_resolvable,
// already_resolved); store errors pass through.
// Constraints: the key is checked first and in constant time; nothing is
// written on a refusal. Exactly one audit row per action: rerun writes the
// operator-retry row the store's CreateAttempt requires for attempt n+1 (the
// attempt itself is created by run.CreateAttempt, the one chokepoint,
// TestCreateAttemptOnly); abandon moves the attempt to failed through the
// operator transition (its own audit row). Only needs-operator is rerunnable. A
// finalizing release attempt can only be abandoned (the store never retries
// after finalizing, P7-CI-31), and only once its coordinator is gone: while the
// coordinator is alive its publish step may be running, so the abandon is
// refused (coordinator_live) unless Force is set (D-1078). Nothing retries automatically. Known limit: the
// store's OperatorRetry has no "already approved" check and no audit read
// exists outside the store, so a second rerun before attempt n+1 exists writes a
// second approval row (each call is an audited decision; debt row filed).

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Action is what the operator decides.
type Action string

// The two decisions.
const (
	Rerun   Action = "rerun"
	Abandon Action = "abandon"
)

// Request is one resolution.
type Request struct {
	Attempt, Key, Actor, Reason string
	Action                      Action
	Force                       bool   // abandon a finalizing attempt although its coordinator is alive
	Boot                        string // this machine's boot id, for the coordinator liveness test
}

// Result says what was done.
type Result struct {
	Attempt string `json:"attempt"`
	Action  Action `json:"action"`
	State   string `json:"state"`          // the state the attempt was in
	Next    string `json:"next,omitempty"` // the attempt id a rerun authorizes (n+1)
}

// Resolve applies req.
func Resolve(ctx context.Context, st *store.Store, req Request) (Result, error) {
	if req.Action != Rerun && req.Action != Abandon {
		return Result{}, refuse(CodeResolveRefused, NotResolvable, "action must be rerun or abandon")
	}
	at, job, siblings, err := locate(ctx, st, req.Attempt)
	if err != nil {
		return Result{}, err
	}
	if subtle.ConstantTimeCompare([]byte(IdempotencyKey(req.Attempt)), []byte(req.Key)) != 1 {
		return Result{}, refuse(CodeResolveRefused, KeyMismatch, "the idempotency key does not belong to attempt %s", req.Attempt)
	}
	next := fmt.Sprintf("%s.a%d", at.JobID, at.N+1)
	for _, o := range siblings {
		if o.ID == next {
			return Result{}, refuse(CodeResolveRefused, AlreadyResolved, "attempt %s already has a successor (%s)", req.Attempt, o.ID)
		}
	}
	res := Result{Attempt: req.Attempt, Action: req.Action, State: at.State}
	actor := req.Actor
	if actor == "" {
		actor = "operator"
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "resolved by `nself ci runs resolve`"
	}
	stuck := at.State == "needs-operator"
	finalReleaseAttempt := at.State == "finalizing" && job.Kind == "release"
	if !stuck && !finalReleaseAttempt {
		return Result{}, refuse(CodeResolveRefused, NotResolvable, "attempt %s is %s: only needs-operator (or a finalizing release attempt, to abandon) can be resolved", req.Attempt, at.State)
	}
	if req.Action == Abandon && !stuck && !req.Force {
		live, lerr := st.AttemptCoordinatorLive(ctx, at.ID, req.Boot)
		if lerr != nil {
			return Result{}, lerr
		}
		if live {
			return Result{}, refuse(CodeResolveRefused, CoordinatorLive, "attempt %s is finalizing and its coordinator is alive: the publish step may be running; wait for it, or pass --force to abandon anyway", req.Attempt)
		}
	}
	if req.Action == Rerun {
		if !stuck {
			return Result{}, refuse(CodeResolveRefused, NotResolvable, "attempt %s is finalizing: a release attempt is never retried after finalizing; abandon it", req.Attempt)
		}
		err = st.OperatorRetry(ctx, actor, at.ID, reason)
		res.Next = next
	} else {
		err = st.Transition(ctx, at.ID, model.JobState(at.State), "failed",
			store.TransitionDetail{Epoch: at.Epoch, FailureClass: "cancelled", Actor: actor, Reason: "abandoned: " + reason, Operator: true})
	}
	return res, mapConflict(err)
}

// mapConflict turns a lost compare-and-set (a concurrent resolution) into E716.
func mapConflict(err error) error {
	var se *store.Error
	if errors.As(err, &se) && (se.Code == "E605" || se.Code == "E604") {
		return refuse(CodeResolveRefused, AlreadyResolved, "the attempt changed while it was being resolved: %s", se.Message)
	}
	return err
}

// historyPage is the page size of the run history scan (the store's maximum).
const historyPage = 100

// locate finds the attempt, its job and the other attempts of that job.
func locate(ctx context.Context, st *store.Store, id string) (store.AttemptView, store.RunJob, []store.AttemptView, error) {
	var before int64
	for {
		page, err := st.History(ctx, before, historyPage)
		if err != nil {
			return store.AttemptView{}, store.RunJob{}, nil, err
		}
		for _, p := range page.Pipelines {
			if !strings.HasPrefix(id, p.ID+".") {
				continue
			}
			attempts, err := st.PipelineAttempts(ctx, p.ID)
			if err != nil {
				return store.AttemptView{}, store.RunJob{}, nil, err
			}
			for _, a := range attempts {
				if a.ID != id {
					continue
				}
				jobs, err := st.PipelineJobs(ctx, p.ID)
				if err != nil {
					return store.AttemptView{}, store.RunJob{}, nil, err
				}
				var job store.RunJob
				for _, j := range jobs {
					if j.ID == a.JobID {
						job = j
					}
				}
				var siblings []store.AttemptView
				for _, o := range attempts {
					if o.JobID == a.JobID && o.ID != a.ID {
						siblings = append(siblings, o)
					}
				}
				return a, job, siblings, nil
			}
		}
		if len(page.Pipelines) < historyPage {
			break // the last page
		}
		before = page.NextCreatedAt
	}
	return store.AttemptView{}, store.RunJob{}, nil, refuse(CodeResolveRefused, NotResolvable, "no attempt %q in the run history", id)
}
