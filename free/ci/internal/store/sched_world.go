package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"maps"
	"sync"
)

// SchedWorld is a single, consistent read of the durable placement facts.
type SchedWorld struct {
	Nodes         []SchedNode
	Active        []SchedAttempt
	Queued        map[string]int
	Previous      map[string]string
	PreviousByJob map[string]string
	Reservations  map[string]SchedReservation
	Projects      map[string]string
}
type SchedNode struct {
	ID, State  string
	Capability json.RawMessage
}
type SchedAttempt struct {
	PipelineID, Project, JobID, RunnerID, Provider, State string
	Epoch                                                 int64
}
type SchedReservation struct{ CPU, MemMB, Slots int64 }

type pinKey struct{}

// pinnedWorld is the one snapshot a pinned context shares; it is read on first use.
type pinnedWorld struct {
	mu   sync.Mutex
	snap SchedWorld
	done bool
}

// PinWorldSnapshot returns a context whose WorldSnapshot calls all see the first one's result, so worlds
// built one after another (one per pipeline in a dispatch tick) read the same instant of the store and a
// capacity release between two builds cannot make a later world see more free slots than an earlier one
// (D-1412). A failed read is not kept: the next call reads again. Pinning does not nest or expire; drop the
// context when the batch is done.
func PinWorldSnapshot(ctx context.Context) context.Context {
	return context.WithValue(ctx, pinKey{}, &pinnedWorld{})
}

// WorldSnapshot reads all placement tables in one read transaction; it never writes. On a pinned context
// (PinWorldSnapshot) the first call reads and the others get copies of that result.
func (s *Store) WorldSnapshot(ctx context.Context) (SchedWorld, error) {
	p, ok := ctx.Value(pinKey{}).(*pinnedWorld)
	if !ok {
		return s.readWorldSnapshot(ctx)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.done {
		snap, err := s.readWorldSnapshot(ctx)
		if err != nil {
			return SchedWorld{}, err
		}
		p.snap, p.done = snap, true
	}
	return p.snap.clone(), nil
}

// clone copies the maps, which a caller may keep and change; the slices are read-only facts.
func (w SchedWorld) clone() SchedWorld {
	w.Queued, w.Previous, w.PreviousByJob = maps.Clone(w.Queued), maps.Clone(w.Previous), maps.Clone(w.PreviousByJob)
	w.Reservations, w.Projects = maps.Clone(w.Reservations), maps.Clone(w.Projects)
	return w
}

func (s *Store) readWorldSnapshot(ctx context.Context) (out SchedWorld, err error) {
	defer func() {
		if err != nil {
			err = coded("E607", err.Error())
			out = SchedWorld{}
		}
	}()
	tx, err := s.readers.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	out.Queued = map[string]int{}
	out.Previous = map[string]string{}
	out.PreviousByJob = map[string]string{}
	out.Reservations = map[string]SchedReservation{}
	out.Projects = map[string]string{}
	projectRows, err := tx.QueryContext(ctx, `SELECT id,project FROM pipeline ORDER BY id`)
	if err != nil {
		return SchedWorld{}, err
	}
	for projectRows.Next() {
		var id, project string
		if err = projectRows.Scan(&id, &project); err != nil {
			break
		}
		out.Projects[id] = project
	}
	if err == nil {
		err = projectRows.Err()
	}
	_ = projectRows.Close()
	if err != nil {
		return SchedWorld{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT n.id,n.state,c.doc_json FROM node n JOIN capability_snapshot c ON c.node_id=n.id ORDER BY n.id`)
	if err != nil {
		return SchedWorld{}, err
	}
	for rows.Next() {
		var n SchedNode
		var doc string
		if err = rows.Scan(&n.ID, &n.State, &doc); err != nil {
			break
		}
		n.Capability = json.RawMessage(doc)
		out.Nodes = append(out.Nodes, n)
	}
	if err == nil {
		err = rows.Err()
	}
	_ = rows.Close()
	if err != nil {
		return SchedWorld{}, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT p.id,p.project,j.id,coalesce(l.runner_id,a.runner_id,''),coalesce(r.provider,''),a.state,a.epoch FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id LEFT JOIN lease l ON l.attempt_id=a.id LEFT JOIN runner_ref r ON r.id=coalesce(l.runner_id,a.runner_id) WHERE a.state IN ('leased','running','finalizing') ORDER BY a.id`)
	if err != nil {
		return SchedWorld{}, err
	}
	for rows.Next() {
		var a SchedAttempt
		if err = rows.Scan(&a.PipelineID, &a.Project, &a.JobID, &a.RunnerID, &a.Provider, &a.State, &a.Epoch); err != nil {
			break
		}
		out.Active = append(out.Active, a)
	}
	if err == nil {
		err = rows.Err()
	}
	_ = rows.Close()
	if err != nil {
		return SchedWorld{}, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT p.project,count(*) FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.state='queued' GROUP BY p.project ORDER BY p.project`)
	if err != nil {
		return SchedWorld{}, err
	}
	for rows.Next() {
		var p string
		var n int
		if err = rows.Scan(&p, &n); err != nil {
			break
		}
		out.Queued[p] = n
	}
	if err == nil {
		err = rows.Err()
	}
	_ = rows.Close()
	if err != nil {
		return SchedWorld{}, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT a.id,a.job_id,a.previous_runner_id FROM attempt a WHERE a.previous_runner_id IS NOT NULL AND a.previous_runner_id!='' ORDER BY a.job_id,a.n`)
	if err != nil {
		return SchedWorld{}, err
	}
	for rows.Next() {
		var id, jobID, runner string
		if err = rows.Scan(&id, &jobID, &runner); err != nil {
			break
		}
		out.Previous[id] = runner
		out.PreviousByJob[jobID] = runner
	}
	if err == nil {
		err = rows.Err()
	}
	_ = rows.Close()
	if err != nil {
		return SchedWorld{}, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT l.runner_id,coalesce(sum(h.cpu),0),coalesce(sum(h.mem_mb),0),count(*) FROM host_reservation h JOIN lease l ON l.id=h.lease_id JOIN attempt a ON a.id=l.attempt_id WHERE a.state IN ('leased','running','finalizing') GROUP BY l.runner_id ORDER BY l.runner_id`)
	if err != nil {
		return SchedWorld{}, err
	}
	for rows.Next() {
		var id string
		var r SchedReservation
		if err = rows.Scan(&id, &r.CPU, &r.MemMB, &r.Slots); err != nil {
			break
		}
		out.Reservations[id] = r
	}
	if err == nil {
		err = rows.Err()
	}
	_ = rows.Close()
	if err != nil {
		return SchedWorld{}, err
	}
	if err = tx.Commit(); err != nil {
		return SchedWorld{}, err
	}
	return out, nil
}
