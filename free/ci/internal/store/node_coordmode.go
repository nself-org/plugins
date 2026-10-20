package store

// Purpose: the coordinator mode row (P7-NODE-20, Epic D21): `personal` (default at first start) or `team`
// (set only by `nself ci serve --team`). One row in migration set `node-coordmode`.
// Inputs:  the --team request and the actor name of the serve that makes it.
// Outputs: the mode a serve runs in, and one audit row when a serve switches the mode to team.
// Constraints: fail closed. A read that fails, finds no row or finds any value but exactly `personal` reports
//          team. The mode never goes from team back to personal on an existing data dir: no store call
//          writes it, and a trigger in the migration aborts the UPDATE and the DELETE of a team row.
//          The mode never depends on which role a grant carries; the principal check lives in
//          internal/coordinator/mode.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
)

//go:embed migrations/node-coordmode/*.sql
var nodeCoordModeMigrations embed.FS

func init() {
	if err := Register("node-coordmode", nodeCoordModeMigrations); err != nil {
		panic(err)
	}
}

// The two coordinator modes.
const (
	CoordinatorPersonal = "personal"
	CoordinatorTeam     = "team"
)

// CoordinatorModeChange is what CoordinatorModeEnsure did.
type CoordinatorModeChange struct {
	Mode     string // the mode the coordinator runs in
	Created  bool   // the row did not exist before
	Switched bool   // this call set the mode to team (one audit row was written)
}

// CoordinatorModeGet returns the stored mode. found is false when no serve ever stored one. A read error
// returns team with the error; a stored value other than personal also reads team.
func (s *Store) CoordinatorModeGet(ctx context.Context) (mode string, found bool, err error) {
	err = s.readers.QueryRowContext(ctx, `SELECT mode FROM coordinator_mode WHERE id=1`).Scan(&mode)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return CoordinatorTeam, false, err
	case mode != CoordinatorPersonal:
		mode = CoordinatorTeam
	}
	return mode, true, nil
}

// CoordinatorMode is the fail-closed read: personal only when the row says exactly personal.
func (s *Store) CoordinatorMode(ctx context.Context) string {
	mode, found, err := s.CoordinatorModeGet(ctx)
	if err != nil || !found || mode != CoordinatorPersonal {
		return CoordinatorTeam
	}
	return CoordinatorPersonal
}

// CoordinatorModeEnsure runs at serve start. No row: store personal (team when team is set). A personal row
// and team set: switch to team and write one audit row. A team row stays team whatever the flag says, and
// nothing is written. On any error the returned mode is team.
func (s *Store) CoordinatorModeEnsure(ctx context.Context, team bool, actor string) (CoordinatorModeChange, error) {
	out := CoordinatorModeChange{Mode: CoordinatorTeam}
	err := s.writeRetry(ctx, func(tx *sql.Tx) error {
		out = CoordinatorModeChange{Mode: CoordinatorTeam}
		var cur string
		err := tx.QueryRowContext(ctx, `SELECT mode FROM coordinator_mode WHERE id=1`).Scan(&cur)
		found := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := s.now().UnixNano()
		switch {
		case !found:
			out.Created = true
			out.Mode = CoordinatorPersonal
			if team {
				out.Mode, out.Switched = CoordinatorTeam, true
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO coordinator_mode(id,mode,set_at) VALUES(1,?,?)`, out.Mode, now); err != nil {
				return err
			}
			if out.Switched {
				return s.appendAudit(ctx, tx, actor, "coordinator.mode", "coordinator", "mode=team from=none via=--team")
			}
		case cur == CoordinatorPersonal && team:
			if _, err = tx.ExecContext(ctx, `UPDATE coordinator_mode SET mode='team',set_at=? WHERE id=1 AND mode='personal'`, now); err != nil {
				return err
			}
			out.Switched = true
			return s.appendAudit(ctx, tx, actor, "coordinator.mode", "coordinator", "mode=team from=personal via=--team")
		case cur == CoordinatorPersonal:
			out.Mode = CoordinatorPersonal
		}
		return nil
	})
	if err != nil {
		return CoordinatorModeChange{Mode: CoordinatorTeam}, err
	}
	return out, nil
}
