package store

// Purpose: tell whether the coordinator that owns an attempt is still alive,
// by the same test StaleCoordinators applies (fresh heartbeat, current boot).
// Inputs:  the attempt id and this machine's boot id ("" = skip the boot test).
// Outputs: true when the attempt has an owner that is not stale; an attempt
// with no owner, or whose owner row is gone, has no live coordinator.
// Constraints: read only.

import (
	"context"
	"database/sql"
)

// AttemptCoordinatorLive reports whether the attempt's coordinator is alive.
func (s *Store) AttemptCoordinatorLive(ctx context.Context, attempt, currentBoot string) (bool, error) {
	var heartbeat int64
	var boot string
	err := s.readers.QueryRowContext(ctx, "SELECT c.heartbeat_at,c.boot_id FROM attempt a JOIN coordinator c ON c.id=a.coordinator_id WHERE a.id=?", attempt).Scan(&heartbeat, &boot)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return heartbeat >= s.now().Add(-3*s.ttl).UnixNano() && (currentBoot == "" || boot == currentBoot), nil
}
