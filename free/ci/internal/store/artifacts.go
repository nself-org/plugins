package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// DeclareArtifacts records immutable names and expected digests before execution.
// A declaration's Name must carry sha256:<hex>, so a later commit is verifiable.
func (s *Store) DeclareArtifacts(ctx context.Context, attemptID string, decls []model.ArtifactDecl) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var state string
		if err := tx.QueryRowContext(ctx, "SELECT state FROM attempt WHERE id=?", attemptID).Scan(&state); err != nil {
			return err
		}
		if state != "queued" && state != "leased" {
			return coded("E604", "artifacts must be declared before running")
		}
		for _, d := range decls {
			name := d.Name
			if name == "" {
				name = d.Path
			}
			parts := strings.Split(name, "@sha256:")
			if len(parts) != 2 || len(parts[1]) != 64 {
				return coded("E604", "artifact name requires @sha256 digest")
			}
			_, err := tx.ExecContext(ctx, "INSERT INTO declared_artifact(attempt_id,name,path,expected_sha256) VALUES (?,?,?,?)", attemptID, name, d.Path, parts[1])
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// MarkArtifactCommitted fences stale workers and checks the declared digest.
func (s *Store) MarkArtifactCommitted(ctx context.Context, attemptID string, epoch int64, name, sha256 string, size int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var state, expected string
		var current int64
		err := tx.QueryRowContext(ctx, `SELECT a.state,a.epoch,d.expected_sha256 FROM attempt a JOIN declared_artifact d ON d.attempt_id=a.id WHERE a.id=? AND d.name=?`, attemptID, name).Scan(&state, &current, &expected)
		if err != nil {
			return coded("E604", "undeclared artifact")
		}
		if current != epoch {
			return coded("E605", "stale artifact epoch")
		}
		if state != "running" && state != "finalizing" {
			return coded("E604", "attempt cannot commit artifacts")
		}
		if sha256 != expected || size < 0 {
			return coded("E604", "artifact digest or size invalid")
		}
		_, err = tx.ExecContext(ctx, "UPDATE declared_artifact SET committed_sha256=?,size=?,committed=1 WHERE attempt_id=? AND name=?", sha256, size, attemptID, name)
		return err
	})
}
