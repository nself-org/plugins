package store

import (
	"context"
	"database/sql"
)

type AttemptView struct {
	ID, JobID, State, FailureClass string
	N                              int
	Epoch                          int64
}
type EvidenceView struct {
	ID, PipelineID, AttemptID, Revision, InputDigest string
	Body                                             []byte
	CreatedAt                                        int64
}
type HistoryPage struct {
	Pipelines     []PipelineRow
	NextCreatedAt int64
}
type FlakyCandidate struct {
	JobID, InputDigest string
	DistinctResults    int
}

// PipelinesByRevision returns newest first.
func (s *Store) PipelinesByRevision(ctx context.Context, revision string) ([]PipelineRow, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status FROM pipeline WHERE revision=? ORDER BY created_at DESC`, revision)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanPipelines(rows)
}

func scanPipelines(rows *sql.Rows) ([]PipelineRow, error) {
	var out []PipelineRow
	for rows.Next() {
		var p PipelineRow
		err := rows.Scan(&p.ID, &p.Project, &p.Revision, &p.Trigger, &p.SourceTrust, &p.PrivacyZone, &p.PolicyDigest, &p.InputDigest, &p.SelectionMode, &p.SelectionReason, &p.SelectionDigest, &p.Status)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) AttemptsByJob(ctx context.Context, jobID string) ([]AttemptView, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT id,job_id,n,state,epoch,coalesce(failure_class,'') FROM attempt WHERE job_id=? ORDER BY n", jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AttemptView
	for rows.Next() {
		var a AttemptView
		if err = rows.Scan(&a.ID, &a.JobID, &a.N, &a.State, &a.Epoch, &a.FailureClass); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// History uses a stable created-at cursor and bounded page size.
func (s *Store) History(ctx context.Context, before int64, limit int) (HistoryPage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if before == 0 {
		before = 1<<63 - 1
	}
	rows, err := s.readers.QueryContext(ctx, `SELECT id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at FROM pipeline WHERE created_at<? ORDER BY created_at DESC LIMIT ?`, before, limit)
	if err != nil {
		return HistoryPage{}, err
	}
	defer func() { _ = rows.Close() }()
	var page HistoryPage
	for rows.Next() {
		var p PipelineRow
		var created int64
		err = rows.Scan(&p.ID, &p.Project, &p.Revision, &p.Trigger, &p.SourceTrust, &p.PrivacyZone, &p.PolicyDigest, &p.InputDigest, &p.SelectionMode, &p.SelectionReason, &p.SelectionDigest, &p.Status, &created)
		if err != nil {
			return page, err
		}
		page.Pipelines = append(page.Pipelines, p)
		page.NextCreatedAt = created
	}
	return page, rows.Err()
}

func (s *Store) FlakyCandidates(ctx context.Context) ([]FlakyCandidate, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT j.id,j.input_digest,count(DISTINCT a.state) FROM job j JOIN attempt a ON a.job_id=j.id WHERE a.state IN ('passed','failed') GROUP BY j.id,j.input_digest HAVING count(DISTINCT a.state)>1 ORDER BY j.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []FlakyCandidate
	for rows.Next() {
		var f FlakyCandidate
		if err = rows.Scan(&f.JobID, &f.InputDigest, &f.DistinctResults); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) EvidenceByRevision(ctx context.Context, revision, inputDigest string) ([]EvidenceView, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT id,pipeline_id,coalesce(attempt_id,''),revision,input_digest,body,created_at FROM evidence WHERE revision=? AND input_digest=? ORDER BY created_at DESC`, revision, inputDigest)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []EvidenceView
	for rows.Next() {
		var e EvidenceView
		if err = rows.Scan(&e.ID, &e.PipelineID, &e.AttemptID, &e.Revision, &e.InputDigest, &e.Body, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
