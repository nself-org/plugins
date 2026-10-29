package store

// Purpose: the read-only queries behind `nself ci runs stats` (P7-SCHED-20, Epic D23): one read
// transaction over the rows the scheduling, cache, lease, node and outbox sets already keep.
// Inputs:  a StatsQuery (clock, window, optional exact project).
// Outputs: StatsRows, the raw rows internal/stats aggregates; no aggregation happens here.
// Constraints: read transactions only; every statement is index-assisted (the query-plan test in
//          internal/stats explains each StatsStatements entry); a set whose table is absent
//          reports it in Missing instead of failing; times: telemetry ts is milliseconds, every
//          other table nanoseconds; the project filter is an index range, never a table scan.

import (
	"context"
	"database/sql"
	"time"
)

// StatsQuery selects the window. Project "" reads every project.
type StatsQuery struct {
	Now     time.Time
	Since   time.Duration
	Project string
}

// StatOutcome is one finished attempt with its job and trigger.
type StatOutcome struct {
	Attempt, Job, Project, JobKey, Runner, RunnerClass, Result, FailureClass, Trigger string
	N                                                                                 int
	WaitMs, RuntimeMs, ColdStartMs                                                    int64
}

// StatGroup is one aggregated row: a project, a key (result, confidence or state), a count and a sum.
type StatGroup struct {
	Project, Key string
	N, Sum       int64
}

// StatLease is one lease with the state of its attempt and its project.
type StatLease struct {
	Project, AttemptState string
	ExpiresAt, Epoch      int64
}

// StatNode is one node with its capability document (empty when none).
type StatNode struct {
	ID, Name, State, Doc string
	UpdatedAt            int64
}

// StatsRows is everything the aggregation reads.
type StatsRows struct {
	Outcomes  []StatOutcome
	Queued    []StatGroup // Key = trigger
	Cache     []StatGroup // Key = result, Sum = bytes
	Ledger    []StatGroup // Key = confidence, Sum = micro
	CAS       []StatGroup // Sum = bytes
	Artifacts []StatGroup // Sum = bytes
	Leases    []StatLease
	Nodes     []StatNode
	// OutboxPending counts unposted entries; OutboxOldest is the creation time (ns) of the oldest, 0 for none.
	OutboxPending, OutboxOldest int64
	Incarnation                 int64
	Path                        string
	Missing                     map[string]bool
}

// StatsStmt is one statement with the arguments a query binds.
type StatsStmt struct {
	Name, SQL string
	Args      []any
}

// statsTables are the tables whose set may be absent; the rest are core.
var statsTables = []string{"schedtel_outcome", "ledger", "cache_use", "cache_entry", "cache_artifact", "outbox", "node"}

// StatsStatements returns the read statements for q, in the order StatsRead runs them.
func StatsStatements(q StatsQuery) []StatsStmt {
	lo, hi := "", "\U0010ffff"
	if q.Project != "" {
		lo, hi = q.Project, q.Project
	}
	cut := q.Now.Add(-q.Since)
	ms, ns := cut.UnixMilli(), cut.UnixNano()
	return []StatsStmt{
		{"outcomes", `SELECT o.attempt_id,a.job_id,a.n,o.project,o.job_key,o.runner,o.runner_class,o.result,o.failure_class,p.trigger,o.wait_ms,o.runtime_ms,o.cold_start_ms
FROM schedtel_outcome o JOIN attempt a ON a.id=o.attempt_id JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id
WHERE o.project BETWEEN ? AND ? AND o.ts>=?`, []any{lo, hi, ms}},
		{"queued", `SELECT p.project,p.trigger,count(*) FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.state='queued' GROUP BY p.project,p.trigger`, nil},
		{"cache", `SELECT coalesce(c.project,''),u.result,count(*),sum(u.bytes) FROM cache_use u LEFT JOIN cache_attempt c ON c.attempt_id=u.attempt_id WHERE u.ts>=? GROUP BY 1,2`, []any{ns}},
		{"ledger", `SELECT project,confidence,count(*),sum(coalesce(act_micro,est_micro)) FROM ledger WHERE provider>='' AND admitted_at>=? GROUP BY project,confidence`, []any{ns}},
		{"cas", `SELECT coalesce(c.project,''),count(*),sum(e.bytes) FROM cache_entry e LEFT JOIN cache_attempt c ON c.attempt_id=e.attempt_id WHERE e.namespace>=x'' GROUP BY 1`, nil},
		{"artifacts", `SELECT project,count(*),sum(size) FROM cache_artifact WHERE project>='' AND state='committed' GROUP BY project`, nil},
		{"leases", `SELECT coalesce(p.project,''),a.state,l.expires_at,l.epoch FROM lease l JOIN attempt a ON a.id=l.attempt_id JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE l.expires_at>=0`, nil},
		{"nodes", `SELECT n.id,n.name,n.state,n.updated_at,coalesce(c.doc_json,'') FROM node n LEFT JOIN capability_snapshot c ON c.node_id=n.id WHERE n.id>=''`, nil},
		{"outbox", `SELECT count(*),coalesce(min(created_at),0) FROM outbox WHERE state IN ('pending','posting','failed')`, nil},
		{"incarnation", `SELECT coalesce(max(incarnation),0) FROM coordinator WHERE id>=''`, nil},
	}
}

// StatsRead runs every statement in one read transaction. A statement whose table is absent is skipped.
func (s *Store) StatsRead(ctx context.Context, q StatsQuery) (StatsRows, error) {
	out := StatsRows{Path: s.path, Missing: map[string]bool{}}
	tx, err := s.readers.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, coded("E607", err.Error())
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range statsTables {
		var n int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, t).Scan(&n); err != nil {
			return out, coded("E607", err.Error())
		}
		out.Missing[t] = n == 0
	}
	skip := map[string]bool{"outcomes": out.Missing["schedtel_outcome"], "cache": out.Missing["cache_use"], "ledger": out.Missing["ledger"],
		"cas": out.Missing["cache_entry"], "artifacts": out.Missing["cache_artifact"], "nodes": out.Missing["node"], "outbox": out.Missing["outbox"]}
	for _, st := range StatsStatements(q) {
		if skip[st.Name] {
			continue
		}
		rows, err := tx.QueryContext(ctx, st.SQL, st.Args...)
		if err != nil {
			return out, coded("E607", err.Error())
		}
		err = scanStats(rows, st.Name, &out)
		_ = rows.Close()
		if err == nil {
			err = rows.Err()
		}
		if err != nil {
			return out, coded("E607", err.Error())
		}
	}
	return out, nil
}

func scanStats(rows *sql.Rows, name string, out *StatsRows) error {
	for rows.Next() {
		var err error
		switch name {
		case "outcomes":
			var o StatOutcome
			err = rows.Scan(&o.Attempt, &o.Job, &o.N, &o.Project, &o.JobKey, &o.Runner, &o.RunnerClass, &o.Result, &o.FailureClass, &o.Trigger, &o.WaitMs, &o.RuntimeMs, &o.ColdStartMs)
			out.Outcomes = append(out.Outcomes, o)
		case "queued":
			var g StatGroup
			err = rows.Scan(&g.Project, &g.Key, &g.N)
			out.Queued = append(out.Queued, g)
		case "cache", "ledger":
			var g StatGroup
			err = rows.Scan(&g.Project, &g.Key, &g.N, &g.Sum)
			if name == "cache" {
				out.Cache = append(out.Cache, g)
			} else {
				out.Ledger = append(out.Ledger, g)
			}
		case "cas", "artifacts":
			var g StatGroup
			err = rows.Scan(&g.Project, &g.N, &g.Sum)
			if name == "cas" {
				out.CAS = append(out.CAS, g)
			} else {
				out.Artifacts = append(out.Artifacts, g)
			}
		case "leases":
			var l StatLease
			err = rows.Scan(&l.Project, &l.AttemptState, &l.ExpiresAt, &l.Epoch)
			out.Leases = append(out.Leases, l)
		case "nodes":
			var n StatNode
			err = rows.Scan(&n.ID, &n.Name, &n.State, &n.UpdatedAt, &n.Doc)
			out.Nodes = append(out.Nodes, n)
		case "outbox":
			err = rows.Scan(&out.OutboxPending, &out.OutboxOldest)
		case "incarnation":
			err = rows.Scan(&out.Incarnation)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
