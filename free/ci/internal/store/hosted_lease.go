package store

// Purpose: the active hosted leases the coordinator's hosted poll loop watches (D-1297). A hosted attempt has
//          no agent heartbeat: its lease is kept alive from the provider's view of the run, which needs the
//          provider a lease was dispatched to.
// Inputs:  none (the lease, attempt and dispatch_intent tables).
// Outputs: one HostedLease per lease whose attempt is leased or running and whose dispatch intent was written
//          for that same lease epoch.
// Constraints: read only; a lease with no dispatch intent (not dispatched yet, or carried by an agent node) is
//          not listed, and neither is a stale intent of an older epoch, so a retried attempt that went to
//          another runner is never polled from its predecessor's run.

import "context"

// HostedLease is one live lease of an attempt dispatched to a hosted provider.
type HostedLease struct {
	Provider, LeaseID, AttemptID string
	Epoch                        int64
}

// HostedLeasesActive lists the live hosted leases, ordered by lease id.
func (s *Store) HostedLeasesActive(ctx context.Context) ([]HostedLease, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT d.provider,l.id,l.attempt_id,l.epoch FROM lease l
JOIN attempt a ON a.id=l.attempt_id
JOIN dispatch_intent d ON d.attempt_id=l.attempt_id AND d.epoch=l.epoch
WHERE a.state IN ('leased','running') ORDER BY l.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HostedLease
	for rows.Next() {
		var h HostedLease
		if err := rows.Scan(&h.Provider, &h.LeaseID, &h.AttemptID, &h.Epoch); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
