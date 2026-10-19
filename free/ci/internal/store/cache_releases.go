package store

// Purpose: the release records of P7-CACHE-18 (Epic CACHE D15): one row per
// run that was published, awaits a signature on the operator's machine, or
// failed to publish.
// Inputs: ReleaseRow values written by internal/release/publish and the run hook.
// Outputs: ReleaseRow values; the state machine below.
// Constraints: expand-only migration set cache-releases with no foreign key;
// the run id is the primary key, so a repeated publication is one row; a
// published row never changes (trigger), and a second write of the same
// manifest digest and key is a no-op, so recovery is idempotent; failed and
// awaiting-signature rows move to any state; the reason is a short printable
// text (never a payload, a path or a secret); SQL stays in this file.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed migrations/cache-releases/*.sql
var cacheReleasesMigrations embed.FS

func init() {
	if err := Register("cache-releases", cacheReleasesMigrations); err != nil {
		panic(err)
	}
}

// Release states.
const (
	ReleasePublished = "published"
	ReleaseAwaiting  = "awaiting-signature"
	ReleaseFailed    = "failed"
)

// maxReleaseReason bounds the stored reason.
const maxReleaseReason = 512

// ReleaseRow is the record of one run's release.
type ReleaseRow struct {
	RunID, Project, Revision string
	State                    string
	ManifestSHA256, KeyID    string // set when published
	Reason                   string // the failure or the wait, short and printable
	CreatedAt, UpdatedAt     int64
}

func checkRelease(r ReleaseRow) error {
	if r.RunID == "" || r.Project == "" || r.Revision == "" {
		return coded("E607", "a release record needs a run, a project and a revision")
	}
	switch r.State {
	case ReleasePublished:
		if len(r.ManifestSHA256) != 64 || r.KeyID == "" {
			return coded("E607", "a published release record needs the manifest digest and the key id")
		}
	case ReleaseAwaiting, ReleaseFailed:
		if r.ManifestSHA256 != "" || r.KeyID != "" {
			return coded("E607", "only a published release record carries a manifest digest and a key id")
		}
	default:
		return coded("E607", "unknown release state")
	}
	if !utf8.ValidString(r.Reason) || strings.ContainsFunc(r.Reason, unicode.IsControl) {
		return coded("E607", "a release reason is printable text")
	}
	return nil
}

// RecordRelease writes the release row of a run. A published row accepts only
// the same manifest digest and key again.
func (s *Store) RecordRelease(ctx context.Context, r ReleaseRow) error {
	if len(r.Reason) > maxReleaseReason {
		r.Reason = strings.ToValidUTF8(r.Reason[:maxReleaseReason], "")
	}
	if err := checkRelease(r); err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		var old ReleaseRow
		err := tx.QueryRowContext(ctx, `SELECT project,revision,state,manifest_sha256,key_id FROM cache_release WHERE run_id=?`, r.RunID).
			Scan(&old.Project, &old.Revision, &old.State, &old.ManifestSHA256, &old.KeyID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			_, err = tx.ExecContext(ctx, `INSERT INTO cache_release(run_id,project,revision,state,manifest_sha256,key_id,reason,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
				r.RunID, r.Project, r.Revision, r.State, r.ManifestSHA256, r.KeyID, r.Reason, now, now)
			return err
		case err != nil:
			return err
		case old.Project != r.Project || old.Revision != r.Revision:
			return coded("E607", "the release record belongs to another project or revision")
		case old.State == ReleasePublished:
			if r.State == ReleasePublished && r.ManifestSHA256 == old.ManifestSHA256 && r.KeyID == old.KeyID {
				return nil
			}
			return coded("E607", "a published release record does not change")
		}
		_, err = tx.ExecContext(ctx, `UPDATE cache_release SET state=?,manifest_sha256=?,key_id=?,reason=?,updated_at=? WHERE run_id=?`,
			r.State, r.ManifestSHA256, r.KeyID, r.Reason, now, r.RunID)
		return err
	})
}

// ReleaseFor returns the release row of a run; ok is false when there is none.
func (s *Store) ReleaseFor(ctx context.Context, runID string) (ReleaseRow, bool, error) {
	r := ReleaseRow{RunID: runID}
	err := s.readers.QueryRowContext(ctx, `SELECT project,revision,state,manifest_sha256,key_id,reason,created_at,updated_at FROM cache_release WHERE run_id=?`, runID).
		Scan(&r.Project, &r.Revision, &r.State, &r.ManifestSHA256, &r.KeyID, &r.Reason, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ReleaseRow{}, false, nil
	}
	return r, err == nil, err
}

// AttemptCoordinator returns the coordinator id the attempt was created under
// (the builder identity of the release provenance); "" when the attempt is unknown.
func (s *Store) AttemptCoordinator(ctx context.Context, attemptID string) (string, error) {
	var id sql.NullString
	err := s.readers.QueryRowContext(ctx, `SELECT coordinator_id FROM attempt WHERE id=?`, attemptID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id.String, err
}
