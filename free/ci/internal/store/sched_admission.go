package store

// Purpose: the store half of the admission chokepoint (P7-SCHED-04, Epic D7/D8).
// AdmitPlacement is the one transaction that creates a lease for a dispatcher
// placement: it re-reads the active-attempt counters and the host reservation,
// runs the registered admission steps, moves the attempt queued -> leased, and
// writes the lease, the host reservation and the placement identity together
// or not at all. AdmitAndLease (P7-CI-31) stays for its own callers; this one
// adds in-transaction counters, tx-capable steps and the optional fencing epoch
// that Engine.Grant's LeaseGrant issues, because neither of those can join
// another transaction.
// Inputs:  host, a Demand, an AdmitRequest and steps in the order to run.
// Outputs: the lease fence (0 for a local lease) or a coded error.
// Constraints: counters count attempts in leased, running or finalizing; the
// attempt being admitted is queued and is never counted; every SQL statement of
// admission lives in this file; steps see an AdmitView whose unexported tx lets
// later store files (budget, approval) write inside the same transaction.

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

//go:embed migrations/sched-admission/*.sql
var schedAdmissionMigrations embed.FS

func init() {
	if err := Register("sched-admission", schedAdmissionMigrations); err != nil {
		panic(err)
	}
}

// AdmitRequest names the placement an admission commits. Pipeline, job and
// project are re-read from the attempt row, never trusted from the caller.
type AdmitRequest struct {
	User, Team, Provider, WorldDigest string
	// NodeID names the registry node of a remote placement; its row is re-read in
	// the transaction, so a node revoked or drained since the world was built
	// never gets a lease. Empty for a local or provider-hosted runner.
	NodeID string
	// Project is the project id the node must still authorize (the job's
	// Project in the scheduler); required with NodeID, empty is refused.
	Project string
	// Incarnation > 0 issues a fenced remote lease (LeaseEpoch); 0 is a local lease.
	Incarnation int64
}

// AdmitCounts are the active-attempt counters of the eight limit levels.
type AdmitCounts struct{ Pipeline, Job, Runner, Provider, Project, User, Team, Global int }

// AdmitView is what a step sees inside the admission transaction.
type AdmitView struct {
	Host               string
	Demand             Demand
	Request            AdmitRequest
	PipelineID         string
	JobName            string
	Project            string
	UsedCPU, UsedMemMB int
	Counts             AdmitCounts
	tx                 *sql.Tx
}

// AdmitStep refuses an admission by returning an error; the transaction rolls back.
type AdmitStep func(context.Context, AdmitView) error

const activeStates = `('leased','running','finalizing')`

// hostCapacityMessage is the message of the E604 a full host reservation raises.
const hostCapacityMessage = "host capacity exhausted"

// NodeUnavailableError is the refusal of a remote placement whose node row says
// the node cannot take a lease now (offline, draining, maintenance, revoked or gone).
type NodeUnavailableError struct{ State string }

func (e *NodeUnavailableError) Error() string { return "E604: node is " + e.State }

// IsHostCapacity reports whether err is AdmitPlacement's full-host refusal: the
// placement is fine and the attempt should wait, not fail.
func IsHostCapacity(err error) bool {
	var se *Error
	return errors.As(err, &se) && se.Code == "E604" && se.Message == hostCapacityMessage
}

// AdmitPlacement admits one queued attempt; see the file comment.
func (s *Store) AdmitPlacement(ctx context.Context, host string, d Demand, r AdmitRequest, steps ...AdmitStep) (int64, error) {
	if host == "" || d.AttemptID == "" || d.LeaseID == "" || d.RunnerID == "" || d.CPU < 1 || d.MemMB < 1 || d.CapacityCPU < 1 || d.CapacityMemMB < 1 {
		return 0, coded("E604", "invalid demand")
	}
	var fence int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		v := AdmitView{Host: host, Demand: d, Request: r, tx: tx}
		if err := tx.QueryRowContext(ctx, `SELECT j.pipeline_id,j.name,p.project FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.id=?`, d.AttemptID).Scan(&v.PipelineID, &v.JobName, &v.Project); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return coded("E604", "unknown attempt")
			}
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT coalesce(sum(cpu),0),coalesce(sum(mem_mb),0) FROM host_reservation WHERE host=?", host).Scan(&v.UsedCPU, &v.UsedMemMB); err != nil {
			return err
		}
		if v.UsedCPU+d.CPU > d.CapacityCPU || v.UsedMemMB+d.MemMB > d.CapacityMemMB {
			return coded("E604", hostCapacityMessage)
		}
		if r.NodeID != "" {
			var state, lifecycle string
			if err := tx.QueryRowContext(ctx, "SELECT state,lifecycle FROM node WHERE id=?", r.NodeID).Scan(&state, &lifecycle); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return &NodeUnavailableError{State: "unknown"}
				}
				return err
			}
			if state != "online" {
				return &NodeUnavailableError{State: state}
			}
			var lc model.CapabilityLifecycle
			if err := json.Unmarshal([]byte(lifecycle), &lc); err != nil {
				return coded("E607", "node lifecycle is unreadable")
			}
			// r.Project is the scheduler's project id (the id a node authorizes); the pipeline row's
			// project is a different identifier (the repository root), so it cannot stand in for it.
			if lc.Discovered || lc.RevokedAt != nil || r.Project == "" || !slices.Contains(lc.AuthorizedProjects, r.Project) {
				return &NodeUnavailableError{State: "unauthorized"}
			}
		}
		counts, err := admitCounts(ctx, tx, v)
		if err != nil {
			return err
		}
		v.Counts = counts
		for _, step := range steps {
			if err := step(ctx, v); err != nil {
				return err
			}
		}
		return s.commitAdmission(ctx, tx, v, &fence)
	})
	if err != nil {
		return 0, err
	}
	return fence, nil
}

func (s *Store) commitAdmission(ctx context.Context, tx *sql.Tx, v AdmitView, fence *int64) error {
	d := v.Demand
	if v.Request.Incarnation > 0 {
		var previous int64
		if err := tx.QueryRowContext(ctx, "SELECT lease_epoch FROM attempt WHERE id=?", d.AttemptID).Scan(&previous); err != nil {
			return err
		}
		if *fence = LeaseEpoch(v.Request.Incarnation, previous); *fence == 0 {
			return coded("E607", "fencing epoch exhausted")
		}
	}
	if err := s.apply(ctx, tx, d.AttemptID, "queued", "leased", TransitionDetail{Epoch: d.Epoch}); err != nil {
		return err
	}
	ttl := d.TTL
	if ttl <= 0 {
		ttl = s.ttl
	}
	now := s.now().UnixNano()
	if *fence > 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE attempt SET lease_epoch=? WHERE id=?", *fence, d.AttemptID); err != nil {
			return err
		}
	}
	if err := insertLease(ctx, tx, v.Host, d, *fence, ttl, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO sched_admitted(attempt_id,pipeline_id,project,user_id,team_id,provider,runner_id,world_digest,admitted_at) VALUES (?,?,?,?,?,?,?,?,?)", d.AttemptID, v.PipelineID, v.Project, v.Request.User, v.Request.Team, v.Request.Provider, d.RunnerID, v.Request.WorldDigest, now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM sched_refusal WHERE attempt_id=?", d.AttemptID)
	return err
}

// admitCounts reads all eight counters in one pass over the active attempts.
// An active attempt with no recorded identity (a lease made before this set
// existed, or through the nil-hook local path) counts against every user, team
// and provider bucket, so an unknown identity can only make a limit stricter.
func admitCounts(ctx context.Context, tx *sql.Tx, v AdmitView) (AdmitCounts, error) {
	var c AdmitCounts
	err := tx.QueryRowContext(ctx, `SELECT count(*),
 coalesce(sum(j.pipeline_id=?),0), coalesce(sum(j.pipeline_id=? AND j.name=?),0), coalesce(sum(l.runner_id=?),0), coalesce(sum(p.project=?),0),
 coalesce(sum(?!='' AND (sa.attempt_id IS NULL OR sa.provider=?)),0), coalesce(sum(sa.attempt_id IS NULL OR sa.user_id=?),0), coalesce(sum(sa.attempt_id IS NULL OR sa.team_id=?),0)
 FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id
 LEFT JOIN lease l ON l.attempt_id=a.id LEFT JOIN sched_admitted sa ON sa.attempt_id=a.id
 WHERE a.state IN `+activeStates,
		v.PipelineID, v.PipelineID, v.JobName, v.Demand.RunnerID, v.Project, v.Request.Provider, v.Request.Provider, v.Request.User, v.Request.Team).
		Scan(&c.Global, &c.Pipeline, &c.Job, &c.Runner, &c.Project, &c.Provider, &c.User, &c.Team)
	return c, err
}

// CancelIntent reports whether an operator asked to cancel the attempt; such an
// attempt can never be admitted, whatever its state still says.
func (s *Store) CancelIntent(ctx context.Context, attemptID string) (bool, error) {
	var n int
	err := s.readers.QueryRowContext(ctx, "SELECT count(*) FROM cancel_intent WHERE attempt_id=?", attemptID).Scan(&n)
	return n > 0, err
}

// RecordRefusal keeps the latest reason an attempt stayed queued, for `ci explain`.
func (s *Store) RecordRefusal(ctx context.Context, attemptID, reason, detail string) error {
	if attemptID == "" || reason == "" {
		return coded("E604", "refusal needs an attempt and a reason")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO sched_refusal(attempt_id,reason,detail,at) VALUES (?,?,?,?) ON CONFLICT(attempt_id) DO UPDATE SET reason=excluded.reason,detail=excluded.detail,at=excluded.at", attemptID, reason, detail, s.now().UnixNano())
		return err
	})
}

// Refusal returns the recorded reason, if the attempt was refused and not yet admitted.
func (s *Store) Refusal(ctx context.Context, attemptID string) (reason, detail string, ok bool, err error) {
	err = s.readers.QueryRowContext(ctx, "SELECT reason,detail FROM sched_refusal WHERE attempt_id=?", attemptID).Scan(&reason, &detail)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return reason, detail, err == nil, err
}

// AdmittedIdentity returns the placement identity recorded at admission.
func (s *Store) AdmittedIdentity(ctx context.Context, attemptID string) (user, team, provider, runner, digest string, ok bool, err error) {
	err = s.readers.QueryRowContext(ctx, "SELECT user_id,team_id,provider,runner_id,world_digest FROM sched_admitted WHERE attempt_id=?", attemptID).Scan(&user, &team, &provider, &runner, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", "", false, nil
	}
	return user, team, provider, runner, digest, err == nil, err
}

// SetPreviousRunner records the runner a queued retry attempt must avoid when another runner can
// take it (retry anti-affinity, the same fact LeaseFail records for its own retries).
func (s *Store) SetPreviousRunner(ctx context.Context, attemptID, runnerID string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE attempt SET previous_runner_id=? WHERE id=? AND state='queued'", runnerID, attemptID)
		return err
	})
}
