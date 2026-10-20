package agent

// Purpose: the coordinator-facing Sink of the pull carrier (agentapi.Config.Sink): it routes each frame the
//          agents send to the Sink of the run that owns the lease, writes the durable log with line_seq, and
//          spends the job token once the result is accepted.
// Inputs:  protocol.Log, ArtifactRef, CacheRef, Result and blob requests from agentapi.
// Outputs: calls on the owning run's Sink; <LogDir>/<lease>.jsonl; the run's Run call returns after Result.
// Constraints: a log line is acknowledged only after fsync and a duplicate line_seq is dropped without
//          reaching the run; a frame for a lease this process never delivered is refused (the agent keeps it
//          and retries), a frame for a lease whose Run already returned is accepted and dropped (it is a
//          resend); CommitsResult is true because the run's sink commits the result through the lease engine,
//          so agentapi must not commit it a second time; the token is spent after the sink accepted the result.

import (
	"context"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// Sink returns the Sink for agentapi.Config.Sink.
func (d *Driver) Sink() transport.Sink { return (*hubSink)(d) }

type hubSink Driver

func (s *hubSink) d() *Driver { return (*Driver)(s) }

// CommitsResult tells agentapi that Result commits the lease through the run's sink.
func (*hubSink) CommitsResult() bool { return true }

// route finds the route of a lease. late is true when the lease's Run already returned.
func (s *hubSink) route(leaseID string) (rt *route, late bool, err error) {
	d := s.d()
	d.mu.Lock()
	defer d.mu.Unlock()
	if rt = d.routes[leaseID]; rt != nil {
		return rt, false, nil
	}
	if d.finished[leaseID] {
		return nil, true, nil
	}
	return nil, false, lost("no run is waiting for lease %s on this coordinator", leaseID)
}

func (s *hubSink) Log(ctx context.Context, l protocol.Log) error {
	rt, late, err := s.route(l.LeaseID)
	if err != nil || late {
		return err
	}
	d := s.d()
	dir := d.cfg().logDir
	if dir == "" {
		return rt.sink.Log(ctx, l)
	}
	lw, err := d.logWriter(rt, dir)
	if err != nil {
		return fmt.Errorf("lease log: %w", err)
	}
	// Forward to the run first, then persist: a run that fails the line makes the agent resend it, and the
	// resend must not be taken for a duplicate. The high-water mark of what the run already saw keeps a
	// resend after a failed write from reaching the run twice.
	if l.LineSeq > lw.Last() && d.forward(rt, l.LineSeq) {
		if err = rt.sink.Log(ctx, l); err != nil {
			d.unforward(rt, l.LineSeq)
			return err
		}
	}
	if _, err = lw.Append(l); err != nil {
		return fmt.Errorf("lease log: %w", err)
	}
	return nil
}

// forward reserves seq for the run: true when the run has not seen a line this high.
func (d *Driver) forward(rt *route, seq uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if seq <= rt.fwd {
		return false
	}
	rt.fwd, rt.prevFwd = seq, rt.fwd
	return true
}

// unforward gives seq back after the run refused the line.
func (d *Driver) unforward(rt *route, seq uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if rt.fwd == seq {
		rt.fwd = rt.prevFwd
	}
}

// syncLogs flushes every open lease log to stable storage; it runs before the driver lets an acknowledgement
// go out.
func (d *Driver) syncLogs() error {
	d.mu.Lock()
	var lws []*LogWriter
	for _, rt := range d.routes {
		if rt.lw != nil {
			lws = append(lws, rt.lw)
		}
	}
	d.mu.Unlock()
	var first error
	for _, lw := range lws {
		if err := lw.Sync(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// logWriter opens the lease's log file once.
func (d *Driver) logWriter(rt *route, dir string) (*LogWriter, error) {
	d.mu.Lock()
	lw := rt.lw
	d.mu.Unlock()
	if lw != nil {
		return lw, nil
	}
	opened, err := OpenLog(LogFile(dir, rt.leaseID))
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if rt.lw != nil { // another frame opened it first
		_ = opened.Close()
		return rt.lw, nil
	}
	rt.lw = opened
	return opened, nil
}

func (s *hubSink) ArtifactRef(ctx context.Context, a protocol.ArtifactRef) error {
	rt, late, err := s.route(a.LeaseID)
	if err != nil || late {
		return err
	}
	return rt.sink.ArtifactRef(ctx, a)
}

func (s *hubSink) CacheRef(ctx context.Context, c protocol.CacheRef) error {
	rt, late, err := s.route(c.LeaseID)
	if err != nil || late {
		return err
	}
	return rt.sink.CacheRef(ctx, c)
}

func (s *hubSink) Blob(ctx context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, fail(ReasonLeaseLost, "blobs are routed by lease through BlobFor")
}

func (s *hubSink) Result(ctx context.Context, r protocol.Result) error {
	rt, late, err := s.route(r.LeaseID)
	if err != nil || late {
		return err
	}
	if err = s.d().syncLogs(); err != nil { // the acknowledgement of the result covers every line before it
		return fmt.Errorf("lease log: %w", err)
	}
	if err = rt.sink.Result(ctx, r); err != nil {
		return err
	}
	if d := s.d(); d.cfg().gate != nil {
		if err = d.cfg().gate.Spend(ctx, rt.jti); err != nil {
			d.logf("agent transport: cannot spend the token of lease %s: %v", rt.leaseID, err)
		}
	}
	rt.finish()
	return nil
}
