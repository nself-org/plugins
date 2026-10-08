package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type Coordinator struct {
	ID                   string
	PID                  int
	ProcessStart, BootID string
	Incarnation          int64
	HeartbeatAt          int64
}

// Register is the aggregate coordinator identity operation.
func (s *Store) Register(ctx context.Context, pid int, start, boot string) (Coordinator, error) {
	return s.RegisterCoordinator(ctx, pid, start, boot)
}

// RegisterCoordinator creates a process identity and returns its incarnation.
func (s *Store) RegisterCoordinator(ctx context.Context, pid int, start, boot string) (Coordinator, error) {
	c := Coordinator{ID: fmt.Sprintf("%s:%d:%s", boot, pid, start), PID: pid, ProcessStart: start, BootID: boot, HeartbeatAt: s.now().UnixNano()}
	err := s.write(ctx, func(tx *sql.Tx) error {
		var existing int64
		err := tx.QueryRowContext(ctx, "SELECT incarnation FROM coordinator WHERE id=?", c.ID).Scan(&existing)
		switch err {
		case nil:
			c.Incarnation = existing + 1
		case sql.ErrNoRows:
			c.Incarnation = 1
		default:
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO coordinator(id,pid,process_start,boot_id,incarnation,heartbeat_at) VALUES (?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET incarnation=excluded.incarnation,heartbeat_at=excluded.heartbeat_at`, c.ID, pid, start, boot, c.Incarnation, c.HeartbeatAt)
		return err
	})
	return c, err
}

// Heartbeat coalesces writes to at most once per TTL/3 per process.
func (s *Store) Heartbeat(ctx context.Context, id string) error {
	now := s.now()
	s.mu.Lock()
	last := s.lastBeat[id]
	if !last.IsZero() && now.Sub(last) < s.ttl/3 {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE coordinator SET heartbeat_at=? WHERE id=?", now.UnixNano(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return coded("E607", "unknown coordinator")
		}
		return nil
	})
	if err == nil {
		s.mu.Lock()
		s.lastBeat[id] = now
		s.mu.Unlock()
	}
	return err
}

// StaleCoordinators finds expired identities or identities from an older boot.
func (s *Store) StaleCoordinators(ctx context.Context, currentBoot string) ([]Coordinator, error) {
	cutoff := s.now().Add(-3 * s.ttl).UnixNano()
	rows, err := s.readers.QueryContext(ctx, "SELECT id,pid,process_start,boot_id,incarnation,heartbeat_at FROM coordinator WHERE heartbeat_at<? OR boot_id!=? ORDER BY id", cutoff, currentBoot)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Coordinator
	for rows.Next() {
		var c Coordinator
		if err = rows.Scan(&c.ID, &c.PID, &c.ProcessStart, &c.BootID, &c.Incarnation, &c.HeartbeatAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SweepStale changes only attempts owned by stale coordinators.
func (s *Store) SweepStale(ctx context.Context, currentBoot string) (int, error) {
	stale, err := s.StaleCoordinators(ctx, currentBoot)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, c := range stale {
		rows, err := s.readers.QueryContext(ctx, "SELECT id,state,epoch FROM attempt WHERE coordinator_id=? AND state IN ('leased','running')", c.ID)
		if err != nil {
			return count, err
		}
		type pending struct {
			id, state string
			epoch     int64
		}
		var list []pending
		for rows.Next() {
			var p pending
			if err = rows.Scan(&p.id, &p.state, &p.epoch); err != nil {
				_ = rows.Close()
				return count, err
			}
			list = append(list, p)
		}
		err = rows.Close()
		if err != nil {
			return count, err
		}
		for _, p := range list {
			err = s.write(ctx, func(tx *sql.Tx) error {
				var heartbeat int64
				var boot string
				if err := tx.QueryRowContext(ctx, "SELECT heartbeat_at,boot_id FROM coordinator WHERE id=?", c.ID).Scan(&heartbeat, &boot); err != nil {
					return err
				}
				if heartbeat >= s.now().Add(-3*s.ttl).UnixNano() && boot == currentBoot {
					return nil
				}
				return s.apply(ctx, tx, p.id, jobState(p.state), "lost", TransitionDetail{Epoch: p.epoch, FailureClass: "lost"})
			})
			if err != nil {
				return count, err
			}
			var state string
			if err = s.readers.QueryRowContext(ctx, "SELECT state FROM attempt WHERE id=?", p.id).Scan(&state); err != nil {
				return count, err
			}
			if state == "lost" {
				count++
			}
		}
	}
	return count, nil
}

func jobState(s string) model.JobState { return model.JobState(s) }
