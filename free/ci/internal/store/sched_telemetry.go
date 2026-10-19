package store

// Purpose: the store half of scheduling telemetry (P7-SCHED-05, Epic D16): one
// decision record per attempt, one outcome per finished attempt, and the EWMA
// estimator rows folded from outcomes. Migration set sched-telemetry.
// Inputs:  rows built by internal/schedtel (this file has no scheduling logic).
// Outputs: reads for `ci runs decisions`, `ci explain` and the world contributor.
// Constraints: retention (90 days or 20 000 rows per project, whichever keeps
// more) runs in the transaction that inserts, for the project it inserts into;
// the estimator fold runs inside the outcome transaction; attempt, job and
// pipeline are core tables read here, with no foreign key from this set.

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
)

//go:embed migrations/sched-telemetry/*.sql
var schedTelemetryMigrations embed.FS

func init() {
	if err := Register("sched-telemetry", schedTelemetryMigrations); err != nil {
		panic(err)
	}
}

// Retention bounds: a row is kept while it is younger than TelKeepMs or among
// the newest TelKeepRows of its project.
const (
	TelKeepMs   = int64(90 * 24 * 3600 * 1000)
	TelKeepRows = 20000
)

// TelDecision is the stored placement decision of one attempt (ID is the attempt id).
type TelDecision struct {
	ID, Run, Job, JobName, Project, WorldDigest, Chosen, RunnerClass, Outcome string
	Attempt                                                                   int
	CostMicro, WaitMs, TS                                                     int64
	Doc                                                                       json.RawMessage
}

// TelOutcome is what one finished attempt measured; JobKey is the estimator key.
type TelOutcome struct {
	AttemptID, Project, JobKey, Runner, RunnerClass, Result, FailureClass string
	WaitMs, RuntimeMs, ColdStartMs, CostMicro, TS                         int64
}

// TelEstimator is the EWMA state of one (project, job key, runner class).
type TelEstimator struct {
	Project, JobKey, RunnerClass string
	RuntimeMs, N, ColdStartMs    int64
	ColdN, Updated               int64
}

// TelAttempt is what the core tables say about an attempt.
type TelAttempt struct {
	ID, Run, Job, JobName, Project, Revision, Trigger, PolicyDigest, State string
	N                                                                      int
}

// TelAttempt reads an attempt with its job and pipeline.
func (s *Store) TelAttempt(ctx context.Context, id string) (TelAttempt, bool, error) {
	rows, err := s.telAttempts(ctx, "a.id=?", id)
	if err != nil || len(rows) == 0 {
		return TelAttempt{}, false, err
	}
	return rows[0], true, nil
}

// TelQueued lists the attempts waiting for a placement.
func (s *Store) TelQueued(ctx context.Context) ([]TelAttempt, error) {
	return s.telAttempts(ctx, "a.state=?", "queued")
}

func (s *Store) telAttempts(ctx context.Context, where string, arg any) ([]TelAttempt, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT a.id,p.id,j.id,j.name,p.project,p.revision,p.trigger,p.policy_digest,a.state,a.n FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE `+where+` ORDER BY a.id`, arg)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []TelAttempt
	for rows.Next() {
		var a TelAttempt
		if err = rows.Scan(&a.ID, &a.Run, &a.Job, &a.JobName, &a.Project, &a.Revision, &a.Trigger, &a.PolicyDigest, &a.State, &a.N); err != nil {
			return nil, coded("E607", err.Error())
		}
		out = append(out, a)
	}
	if err = rows.Err(); err != nil {
		return nil, coded("E607", err.Error())
	}
	return out, nil
}

// TelAttemptEnd returns when an attempt ended (UnixNano), false while it has not.
func (s *Store) TelAttemptEnd(ctx context.Context, id string) (int64, bool, error) {
	var end sql.NullInt64
	err := s.readers.QueryRowContext(ctx, `SELECT ended_at FROM attempt WHERE id=?`, id).Scan(&end)
	if err != nil && err != sql.ErrNoRows {
		return 0, false, coded("E607", err.Error())
	}
	return end.Int64, end.Valid, nil
}

// TelAttemptStart returns when an attempt began running (UnixNano), false while
// it has not (or it is a row written before started_at was recorded).
func (s *Store) TelAttemptStart(ctx context.Context, id string) (int64, bool, error) {
	var start sql.NullInt64
	err := s.readers.QueryRowContext(ctx, `SELECT started_at FROM attempt WHERE id=?`, id).Scan(&start)
	if err != nil && err != sql.ErrNoRows {
		return 0, false, coded("E607", err.Error())
	}
	return start.Int64, start.Valid, nil
}

// TelPutDecision stores (or replaces) the decision of one attempt and applies retention.
func (s *Store) TelPutDecision(ctx context.Context, d TelDecision) error {
	return s.telWrite(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO schedtel_decision (id,run,job,job_name,project,attempt,world_digest,chosen,runner_class,outcome,cost_micro,wait_ms,doc_json,ts) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			d.ID, d.Run, d.Job, d.JobName, d.Project, d.Attempt, d.WorldDigest, d.Chosen, d.RunnerClass, d.Outcome, d.CostMicro, d.WaitMs, string(d.Doc), d.TS); err != nil {
			return err
		}
		return telPrune(ctx, tx, "schedtel_decision", d.Project, d.TS, TelKeepMs, TelKeepRows)
	})
}

const telDecisionCols = `id,run,job,job_name,project,attempt,world_digest,chosen,runner_class,outcome,cost_micro,wait_ms,doc_json,ts`

// TelDecisions lists the decisions of a run (or, with run "", the newest limit overall), oldest first.
func (s *Store) TelDecisions(ctx context.Context, run string, limit int) ([]TelDecision, error) {
	if run != "" {
		return s.telDecisionRows(ctx, `SELECT `+telDecisionCols+` FROM schedtel_decision WHERE run=? ORDER BY ts,id`, run)
	}
	return s.telDecisionRows(ctx, `SELECT * FROM (SELECT `+telDecisionCols+` FROM schedtel_decision ORDER BY ts DESC,id DESC LIMIT ?) ORDER BY ts,id`, limit)
}

// TelDecision reads one decision by attempt id.
func (s *Store) TelDecision(ctx context.Context, id string) (TelDecision, bool, error) {
	rows, err := s.telDecisionRows(ctx, `SELECT `+telDecisionCols+` FROM schedtel_decision WHERE id=?`, id)
	if err != nil || len(rows) == 0 {
		return TelDecision{}, false, err
	}
	return rows[0], true, nil
}

func (s *Store) telDecisionRows(ctx context.Context, q string, args ...any) ([]TelDecision, error) {
	rows, err := s.readers.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []TelDecision
	for rows.Next() {
		var d TelDecision
		var doc string
		if err = rows.Scan(&d.ID, &d.Run, &d.Job, &d.JobName, &d.Project, &d.Attempt, &d.WorldDigest, &d.Chosen, &d.RunnerClass, &d.Outcome, &d.CostMicro, &d.WaitMs, &doc, &d.TS); err != nil {
			return nil, coded("E607", err.Error())
		}
		d.Doc = json.RawMessage(doc)
		out = append(out, d)
	}
	if err = rows.Err(); err != nil {
		return nil, coded("E607", err.Error())
	}
	return out, nil
}

// TelRecordOutcome stores the outcome of one attempt, folds it into its
// estimator with fold (given the current row and whether one exists) and
// applies retention, all in one transaction.
func (s *Store) TelRecordOutcome(ctx context.Context, o TelOutcome, fold func(cur TelEstimator, found bool) TelEstimator) error {
	return s.telWrite(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO schedtel_outcome (attempt_id,project,job_key,runner,runner_class,wait_ms,runtime_ms,cold_start_ms,cost_micro,result,failure_class,ts) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			o.AttemptID, o.Project, o.JobKey, o.Runner, o.RunnerClass, o.WaitMs, o.RuntimeMs, o.ColdStartMs, o.CostMicro, o.Result, o.FailureClass, o.TS); err != nil {
			return err
		}
		if fold != nil {
			cur := TelEstimator{Project: o.Project, JobKey: o.JobKey, RunnerClass: o.RunnerClass}
			err := tx.QueryRowContext(ctx, `SELECT runtime_ms,n,cold_start_ms,cold_n,updated FROM schedtel_estimator WHERE project=? AND job_key=? AND runner_class=?`,
				o.Project, o.JobKey, o.RunnerClass).Scan(&cur.RuntimeMs, &cur.N, &cur.ColdStartMs, &cur.ColdN, &cur.Updated)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			next := fold(cur, err == nil)
			if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO schedtel_estimator (project,job_key,runner_class,runtime_ms,n,cold_start_ms,cold_n,updated) VALUES (?,?,?,?,?,?,?,?)`,
				o.Project, o.JobKey, o.RunnerClass, next.RuntimeMs, next.N, next.ColdStartMs, next.ColdN, next.Updated); err != nil {
				return err
			}
		}
		return telPrune(ctx, tx, "schedtel_outcome", o.Project, o.TS, TelKeepMs, TelKeepRows)
	})
}

// TelEstimators lists every estimator row.
func (s *Store) TelEstimators(ctx context.Context) ([]TelEstimator, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT project,job_key,runner_class,runtime_ms,n,cold_start_ms,cold_n,updated FROM schedtel_estimator ORDER BY project,job_key,runner_class`)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []TelEstimator
	for rows.Next() {
		var e TelEstimator
		if err = rows.Scan(&e.Project, &e.JobKey, &e.RunnerClass, &e.RuntimeMs, &e.N, &e.ColdStartMs, &e.ColdN, &e.Updated); err != nil {
			return nil, coded("E607", err.Error())
		}
		out = append(out, e)
	}
	if err = rows.Err(); err != nil {
		return nil, coded("E607", err.Error())
	}
	return out, nil
}

// TelRunnerResults returns the failure class ("" for a pass) of the newest
// limit outcomes on one runner, newest first.
func (s *Store) TelRunnerResults(ctx context.Context, runner string, limit int) ([]string, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT failure_class FROM schedtel_outcome WHERE runner=? ORDER BY ts DESC,attempt_id DESC LIMIT ?`, runner, limit)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var c string
		if err = rows.Scan(&c); err != nil {
			return nil, coded("E607", err.Error())
		}
		out = append(out, c)
	}
	if err = rows.Err(); err != nil {
		return nil, coded("E607", err.Error())
	}
	return out, nil
}

func (s *Store) telWrite(ctx context.Context, fn func(*sql.Tx) error) error {
	if err := s.write(ctx, fn); err != nil {
		return coded("E607", err.Error())
	}
	return nil
}

// telPrune deletes the rows of project that are older than keepMs and outside
// its newest keepRows (table is a constant of this file, never caller input).
func telPrune(ctx context.Context, tx *sql.Tx, table, project string, now, keepMs int64, keepRows int) error {
	key := "id"
	if table == "schedtel_outcome" {
		key = "attempt_id"
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE project=? AND ts<? AND `+key+` NOT IN (SELECT `+key+` FROM `+table+` WHERE project=? ORDER BY ts DESC,`+key+` DESC LIMIT ?)`,
		project, now-keepMs, project, keepRows)
	return err
}
