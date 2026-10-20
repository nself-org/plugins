package store

// Purpose: the run approval rows (Epic D18, P7-TRUST-24): the source facts the classification hook
// records for a run (author, head repo, whether it needs approval) and the approval bound to the
// run's head SHA, config digest and head repo.
// Inputs:  run ids, ApprovalRunFacts, ApproveInput, and a void check the caller supplies (the rule
//          lives in trust/source; the store never judges a binding).
// Outputs: ApprovalState (pipeline columns, facts and approval joined), voided approvals, pending runs.
// Constraints: expand-only migration set trust-approvals, no foreign key into another set; validity is
//          computed from the rows (a mismatch is never valid, written or not); a void is written, with
//          its audit row, in one transaction by SweepApprovals and RecordApprovalFacts, never inside an
//          admission transaction (a refusal rolls it back); one approval row per run, replaced by a
//          fresh approval; an approval and a void each write exactly one audit row.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
)

//go:embed migrations/trust-approvals/*.sql
var trustApprovalMigrations embed.FS

func init() {
	if err := Register("trust-approvals", trustApprovalMigrations); err != nil {
		panic(err)
	}
}

// ErrApprovalUnknownRun and ErrApprovalStale are the refusals ApproveRun raises besides store errors.
var (
	ErrApprovalUnknownRun = errors.New("store: approval for an unknown run")
	ErrApprovalStale      = errors.New("store: the run moved since the approver looked")
)

// ApprovalBinding is what an approval is bound to; trust/source.Binding has the same fields.
type ApprovalBinding struct{ Run, HeadSHA, ConfigDigest, HeadRepo string }

// ApprovalRunFacts are the source facts recorded for a run.
type ApprovalRunFacts struct {
	Run, Author, HeadRepo string
	RequiresApproval      bool
}

// ApprovalRow is a recorded approval; VoidedAt is zero while it stands.
type ApprovalRow struct {
	ApprovalBinding
	Approver, VoidReason string
	Spend                bool
	ApprovedAt, VoidedAt int64
}

// ApprovalState is everything an approval decision reads about one run.
type ApprovalState struct {
	Run, Project, Revision, ConfigDigest, SourceTrust string
	Finished                                          bool
	// Facts is false when no facts row was recorded; Author, HeadRepo and Requires are then empty.
	Facts            bool
	Author, HeadRepo string
	Requires         bool
	HasApproval      bool
	Approval         ApprovalRow
}

// Current is the binding the run has now: the pipeline row's revision and input digest and the
// recorded head repo.
func (st ApprovalState) Current() ApprovalBinding {
	return ApprovalBinding{Run: st.Run, HeadSHA: st.Revision, ConfigDigest: st.ConfigDigest, HeadRepo: st.HeadRepo}
}

// VoidCheck returns the field names in which cur differs from approved (empty when still valid).
type VoidCheck func(approved, cur ApprovalBinding) []string

type approvalQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const approvalStateSQL = `SELECT p.id,p.project,p.revision,p.input_digest,p.source_trust,p.finished_at IS NOT NULL,
 f.run IS NOT NULL,coalesce(f.author,''),coalesce(f.head_repo,''),coalesce(f.requires_approval,0),
 a.run IS NOT NULL,coalesce(a.head_sha,''),coalesce(a.config_digest,''),coalesce(a.head_repo,''),coalesce(a.approver,''),
 coalesce(a.spend,0),coalesce(a.approved_at,0),coalesce(a.voided_at,0),coalesce(a.void_reason,'')
 FROM pipeline p LEFT JOIN trust_run_facts f ON f.run=p.id LEFT JOIN trust_approvals a ON a.run=p.id WHERE p.id=?`

func readApprovalState(ctx context.Context, q approvalQuerier, run string) (ApprovalState, bool, error) {
	var st ApprovalState
	a := &st.Approval
	err := q.QueryRowContext(ctx, approvalStateSQL, run).Scan(&st.Run, &st.Project, &st.Revision, &st.ConfigDigest, &st.SourceTrust, &st.Finished,
		&st.Facts, &st.Author, &st.HeadRepo, &st.Requires, &st.HasApproval, &a.HeadSHA, &a.ConfigDigest, &a.HeadRepo, &a.Approver, &a.Spend, &a.ApprovedAt, &a.VoidedAt, &a.VoidReason)
	if errors.Is(err, sql.ErrNoRows) {
		return ApprovalState{}, false, nil
	}
	a.Run = st.Run
	return st, err == nil, err
}

// ApprovalState reads one run's approval facts.
func (s *Store) ApprovalState(ctx context.Context, run string) (ApprovalState, bool, error) {
	return readApprovalState(ctx, s.readers, run)
}

// ApprovalState is the in-transaction read the admission step uses: no network, no second connection.
func (v AdmitView) ApprovalState(ctx context.Context) (ApprovalState, bool, error) {
	return readApprovalState(ctx, v.tx, v.PipelineID)
}

// ApproveInput is one approval. ExpectSHA, when set, is the head SHA the approver looked at; BindAuthor
// says ExpectAuthor is the author the non-author check ran against, so a facts rewrite between the check
// and the write is refused. Check voids the standing approval of the run first (one audit row) when the
// run moved since it was given.
type ApproveInput struct {
	Run, Approver, ExpectSHA, ExpectAuthor string
	BindAuthor, Spend                      bool
	Check                                  VoidCheck
}

// ApproveRun records an approval bound to the run's current binding and writes its audit row in the
// same transaction. It returns ErrApprovalUnknownRun for a run that does not exist and
// ErrApprovalStale when the run's head SHA is not ExpectSHA.
func (s *Store) ApproveRun(ctx context.Context, in ApproveInput) (ApprovalRow, error) {
	if in.Run == "" || in.Approver == "" || in.Check == nil {
		return ApprovalRow{}, coded("E607", "approval needs a run, an approver and a void check")
	}
	var row ApprovalRow
	err := s.write(ctx, func(tx *sql.Tx) error {
		st, ok, err := readApprovalState(ctx, tx, in.Run)
		switch {
		case err != nil:
			return err
		case !ok:
			return ErrApprovalUnknownRun
		case in.ExpectSHA != "" && !strings.EqualFold(in.ExpectSHA, st.Revision):
			return ErrApprovalStale
		case in.BindAuthor && in.ExpectAuthor != st.Author:
			return ErrApprovalStale
		}
		if _, err = s.voidStale(ctx, tx, in.Run, in.Check); err != nil {
			return err
		}
		cur := st.Current()
		now := s.now().UnixNano()
		if _, err = tx.ExecContext(ctx, `INSERT INTO trust_approvals(run,head_sha,config_digest,head_repo,approver,spend,approved_at,voided_at,void_reason) VALUES (?,?,?,?,?,?,?,NULL,'')
 ON CONFLICT(run) DO UPDATE SET head_sha=excluded.head_sha,config_digest=excluded.config_digest,head_repo=excluded.head_repo,approver=excluded.approver,spend=excluded.spend,approved_at=excluded.approved_at,voided_at=NULL,void_reason=''`,
			in.Run, cur.HeadSHA, cur.ConfigDigest, cur.HeadRepo, in.Approver, in.Spend, now); err != nil {
			return err
		}
		row = ApprovalRow{ApprovalBinding: cur, Approver: in.Approver, Spend: in.Spend, ApprovedAt: now}
		return s.appendAudit(ctx, tx, in.Approver, "approval.grant", in.Run, fmt.Sprintf("sha=%s config=%s head_repo=%q spend=%t", auditSafe(cur.HeadSHA), auditSafe(cur.ConfigDigest), auditSafe(cur.HeadRepo), in.Spend))
	})
	return row, err
}

// auditSafe makes a stored value fit an audit detail: appendAudit refuses a detail holding "secret=", so a
// head repo or digest a forge chose must not be able to make a void or an approval unwritable.
func auditSafe(v string) string { return strings.ReplaceAll(v, "=", "%3D") }

// RecordApprovalFacts upserts the source facts of a run and voids an approval the new facts no longer
// match, in one transaction.
func (s *Store) RecordApprovalFacts(ctx context.Context, f ApprovalRunFacts, check VoidCheck) ([]ApprovalRow, error) {
	if f.Run == "" || check == nil {
		return nil, coded("E607", "facts need a run and a void check")
	}
	var voided []ApprovalRow
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO trust_run_facts(run,author,head_repo,requires_approval,recorded_at) VALUES (?,?,?,?,?)
 ON CONFLICT(run) DO UPDATE SET author=excluded.author,head_repo=excluded.head_repo,requires_approval=excluded.requires_approval,recorded_at=excluded.recorded_at`,
			f.Run, f.Author, f.HeadRepo, f.RequiresApproval, s.now().UnixNano()); err != nil {
			return err
		}
		var err error
		voided, err = s.voidStale(ctx, tx, f.Run, check)
		return err
	})
	return voided, err
}

// SweepApprovals voids every standing approval of run ("" for all runs) that no longer matches its
// run, one audit row each, and returns them.
func (s *Store) SweepApprovals(ctx context.Context, run string, check VoidCheck) ([]ApprovalRow, error) {
	if check == nil {
		return nil, coded("E607", "sweep needs a void check")
	}
	var voided []ApprovalRow
	err := s.write(ctx, func(tx *sql.Tx) error {
		var err error
		voided, err = s.voidStale(ctx, tx, run, check)
		return err
	})
	return voided, err
}

func (s *Store) voidStale(ctx context.Context, tx *sql.Tx, run string, check VoidCheck) ([]ApprovalRow, error) {
	rows, err := tx.QueryContext(ctx, "SELECT run FROM trust_approvals WHERE voided_at IS NULL AND (?='' OR run=?) ORDER BY run", run, run)
	if err != nil {
		return nil, err
	}
	var runs []string
	for rows.Next() {
		var r string
		if err = rows.Scan(&r); err != nil {
			_ = rows.Close()
			return nil, err
		}
		runs = append(runs, r)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	var out []ApprovalRow
	for _, r := range runs {
		st, ok, err := readApprovalState(ctx, tx, r)
		if err != nil {
			return nil, err
		}
		if !ok { // the run was pruned: the approval has nothing left to bind to
			st = ApprovalState{Run: r, Approval: ApprovalRow{}}
			if err = tx.QueryRowContext(ctx, "SELECT head_sha,config_digest,head_repo,approver,spend,approved_at FROM trust_approvals WHERE run=?", r).Scan(&st.Approval.HeadSHA, &st.Approval.ConfigDigest, &st.Approval.HeadRepo, &st.Approval.Approver, &st.Approval.Spend, &st.Approval.ApprovedAt); err != nil {
				return nil, err
			}
			st.Approval.Run, st.HasApproval = r, true
		}
		a := st.Approval
		reasons := check(a.ApprovalBinding, st.Current())
		if len(reasons) == 0 {
			continue
		}
		a.VoidReason, a.VoidedAt = strings.Join(reasons, ","), s.now().UnixNano()
		if _, err = tx.ExecContext(ctx, "UPDATE trust_approvals SET voided_at=?,void_reason=? WHERE run=? AND voided_at IS NULL", a.VoidedAt, a.VoidReason, r); err != nil {
			return nil, err
		}
		cur := st.Current()
		if err = s.appendAudit(ctx, tx, "system:approval", "approval.void", r, fmt.Sprintf("reasons=%s approved_sha=%s now_sha=%s approved_config=%s now_config=%s approved_head_repo=%q now_head_repo=%q approver=%s",
			a.VoidReason, auditSafe(a.HeadSHA), auditSafe(cur.HeadSHA), auditSafe(a.ConfigDigest), auditSafe(cur.ConfigDigest), auditSafe(a.HeadRepo), auditSafe(cur.HeadRepo), auditSafe(a.Approver))); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// PendingApprovals lists the unfinished runs the facts or the source trust hold for approval, whose
// approval is missing or no longer valid, oldest first. needs says whether a state requires approval
// (the rule lives in trust/source).
func (s *Store) PendingApprovals(ctx context.Context, needs func(ApprovalState) bool, check VoidCheck) ([]ApprovalState, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT id FROM pipeline WHERE finished_at IS NULL ORDER BY created_at, id")
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var out []ApprovalState
	for _, id := range ids {
		st, ok, err := s.ApprovalState(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok || !needs(st) {
			continue
		}
		if st.HasApproval && st.Approval.VoidedAt == 0 && len(check(st.Approval.ApprovalBinding, st.Current())) == 0 {
			continue
		}
		out = append(out, st)
	}
	return out, nil
}
