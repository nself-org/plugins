package authz

// Purpose: who may touch which cache namespace, and the write capability that
// ends with the attempt (ADR 0025 §7, R-SEC-19; research §6: "revoke write at
// job end"). P7-CACHE's stores call CacheAccess for every restore and
// CheckWrite inside the commit transaction.
// Inputs:  Job (namespace recomputed from stored facts, attempt state, result,
// capability) and the namespace being touched.
// Outputs: Mode, CheckWrite's *Denied (E684), Denied, Reason.
// Constraints: pure; no I/O. Call order for P7-CACHE: CacheAccess before every
// restore, then SnapshotEligible(entry) before serving a ROSnapshot entry (the
// mode alone does not make an entry servable), CheckWrite inside the commit
// transaction. Default is none. A job reads or writes only its own
// namespace; a lower class may read (never write) the snapshot of a higher
// class of the same project.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Mode is what a job may do to a namespace.
type Mode string

const (
	// ModeRWOwn is read-write on the job's own namespace.
	ModeRWOwn Mode = "rw-own"
	// ModeROSnapshot is read-only restore of a trusted snapshot (mount read-only).
	ModeROSnapshot Mode = "ro-snapshot"
	// ModeNone is no access.
	ModeNone Mode = "none"
)

// AttemptState is the attempt lifecycle point the store read just now.
type AttemptState string

const (
	// StateRunning is a started attempt: restore only.
	StateRunning AttemptState = "running"
	// StateFinalizing is the only state in which a cache save commits.
	StateFinalizing AttemptState = "finalizing"
	// StateEnded is any other state; unknown values are treated the same way.
	StateEnded AttemptState = "ended"
)

// Capability is the (attempt, epoch) write capability. No bearer token exists:
// the store compares it with the attempt's current lease inside the commit.
type Capability struct {
	Attempt string
	Epoch   int64
}

// Job is what the store knows about the attempt, read from the store (never
// from the job): Namespace is CacheNamespace recomputed from the stored job
// facts.
type Job struct {
	Namespace  Namespace
	State      AttemptState
	Passed     bool
	Capability Capability
}

// live reports whether the attempt can still touch caches at all.
func (j Job) live() bool { return j.State == StateRunning || j.State == StateFinalizing }

// IsSnapshotSource reports whether ns may be the source of a read-only
// snapshot: a protected namespace of an internal or owner class.
func IsSnapshotSource(ns Namespace) bool {
	return ns.Valid() && ns.Protection == Protected && trust.RankTrust(ns.Trust) >= trust.RankTrust("internal")
}

// CacheAccess decides what job may do to target. A finished or unknown attempt
// gets none. Own namespace: read-write. Another namespace: read-only snapshot
// when it is a snapshot source of the same project and of a strictly higher
// class than the job's own; otherwise none. An invalid namespace gets none.
func CacheAccess(job Job, target Namespace) Mode {
	own := job.Namespace
	switch {
	case !job.live() || !own.Valid() || !target.Valid():
		return ModeNone
	case own.Equal(target):
		return ModeRWOwn
	case own.Project == target.Project && IsSnapshotSource(target) && trust.RankTrust(own.Trust) < trust.RankTrust(target.Trust):
		return ModeROSnapshot
	}
	return ModeNone
}

// Reason is a stable key for a refusal; consumers branch on it.
type Reason string

const (
	ReasonNamespace  Reason = "cache.namespace"
	ReasonCapability Reason = "cache.capability"
	ReasonEpoch      Reason = "cache.epoch"
	ReasonState      Reason = "cache.attempt_state"
	ReasonResult     Reason = "cache.result"
	ReasonMode       Reason = "cache.mode"
)

// Denied is the error form of a refused cache write; it carries code E684.
type Denied struct {
	Reason Reason
	Detail string
}

func (d *Denied) Error() string {
	msg := CodeCacheWrite + ": cache write denied: " + string(d.Reason)
	if d.Detail != "" {
		msg += fmt.Sprintf(" (%s)", d.Detail)
	}
	return msg
}

// CheckWrite is the hook the store calls inside the commit transaction. claimed
// is the namespace the caller asked to write; current is the attempt's
// capability as just re-read from the store. It returns nil only for a
// non-empty matching (attempt, epoch), state finalizing, a recorded pass, and
// ModeRWOwn on claimed. The caller's claimed value is only compared with the
// recomputed job.Namespace.
func CheckWrite(job Job, claimed Namespace, current Capability) error {
	switch {
	case job.Capability.Attempt == "" || job.Capability.Attempt != current.Attempt:
		return &Denied{Reason: ReasonCapability, Detail: "attempt does not hold the write capability"}
	case job.Capability.Epoch != current.Epoch:
		return &Denied{Reason: ReasonEpoch, Detail: "lease epoch moved on"}
	case job.State != StateFinalizing:
		return &Denied{Reason: ReasonState, Detail: "attempt is not finalizing"}
	case !job.Passed:
		return &Denied{Reason: ReasonResult, Detail: "attempt has no recorded pass"}
	case !claimed.Equal(job.Namespace):
		return &Denied{Reason: ReasonNamespace, Detail: "claimed namespace is not the job's own"}
	case CacheAccess(job, claimed) != ModeRWOwn:
		return &Denied{Reason: ReasonMode, Detail: "no read-write access"}
	}
	return nil
}
