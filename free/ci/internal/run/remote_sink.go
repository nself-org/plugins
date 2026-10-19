package run

// Purpose: what the run loop needs to use a node it did not start (P7-AUTO-28,
// D-1041): the results sink the carriers of a run's remote leases report to, the
// lease engine behind it, and the per-attempt rendezvous the run loop waits on.
// Inputs:  protocol frames from a transport driver (log, result, refs, blobs);
// the end of a carrier; the run's cancel.
// Outputs: the attempt moves leased -> running -> finalizing through the lease
// engine (the only writer of those moves for a remote lease); the run loop
// receives the evidence or the loss on the attempt's channels.
// Constraints: no polling loop; every wake-up is a frame or a carrier end. A
// result is accepted only for the lease's current epoch (lease.Engine.Result).
// Secret values are never sent from here (the adapter's lease carries no secret
// refs). Blobs, artifact refs and cache refs of a node are not collected by a
// foreground run yet: blobs are discarded, and a job that declares artifacts
// fails at its finalize hook (a declared artifact was not committed).

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// RemoteLeaseTTL is how long a remote lease lives without a heartbeat. The ssh driver derives
// the agent's TTL from it (cmd/transport_ssh.go), so the silence bound equals the lease TTL.
const RemoteLeaseTTL = 45 * time.Second

// RemoteRepo, when set by the composition root, is told the repository root before a
// carrier bundles the run's revision (it points ssh.Default.RepoDir at it).
var RemoteRepo func(root string)

// leaseEngines maps a live remote lease id to the engine that renews it.
var leaseEngines sync.Map

// RenewLease extends a live remote lease on an agent heartbeat. The composition root sets it
// as ssh.Default.Renew; a lease no run holds is stale (E662 semantics: the driver fences it).
func RenewLease(ctx context.Context, leaseID string, epoch uint64) error {
	v, ok := leaseEngines.Load(leaseID)
	if !ok {
		return fail(CodeRun, "no live remote lease %s", leaseID)
	}
	return v.(*lease.Engine).Renew(ctx, leaseID, int64(epoch), 0)
}

// remoteAttempt is the rendezvous of one remote attempt.
type remoteAttempt struct {
	attemptID, leaseID string
	revision           string              // the commit the node was given: its evidence must name it
	result             chan model.Evidence // buffered 1: the accepted result
	lost               chan error          // buffered 1: the carrier ended without one
	ended              chan struct{}       // closed when the carrier has returned

	mu        sync.Mutex
	out       func([]byte)
	pending   [][]byte
	pendingN  int // bytes held in pending
	truncated bool
	running   bool
	cancelled bool // the run cancelled before the carrier started
	cancel    context.CancelFunc
}

// maxPendingLog bounds the output held before the run attaches (a node is not trusted to be polite).
const maxPendingLog = 1 << 20

// write passes job output to the run, or holds it until the run attaches.
func (ra *remoteAttempt) write(b []byte) {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	if ra.out == nil {
		if ra.pendingN+len(b) > maxPendingLog {
			if !ra.truncated {
				ra.truncated = true
				ra.pending = append(ra.pending, []byte("[output truncated: the node sent more than 1 MiB before the run attached]\n"))
			}
			return
		}
		ra.pendingN += len(b)
		ra.pending = append(ra.pending, b)
		return
	}
	ra.out(b)
}

// detach drops later output: the run no longer wants it.
func (ra *remoteAttempt) detach() {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	ra.pending, ra.out = nil, func([]byte) {}
}

// attach sets the output function and flushes what arrived before it.
func (ra *remoteAttempt) attach(out func([]byte)) {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	for _, b := range ra.pending {
		out(b)
	}
	ra.pending, ra.pendingN, ra.out = nil, 0, out
}

// remoteSink is the transport.Sink of a run's remote leases.
type remoteSink struct {
	st  *store.Store
	eng *lease.Engine

	mu sync.Mutex
	by map[string]*remoteAttempt // attempt id
}

func newRemoteSink(st *store.Store) *remoteSink {
	return &remoteSink{st: st, eng: lease.New(st, registry.New(st), lease.Config{TTL: RemoteLeaseTTL}), by: map[string]*remoteAttempt{}}
}

// expect registers the rendezvous of an attempt before it is submitted.
func (s *remoteSink) expect(attemptID, revision string) *remoteAttempt {
	ra := &remoteAttempt{attemptID: attemptID, revision: revision, leaseID: "lease-" + attemptID, result: make(chan model.Evidence, 1), lost: make(chan error, 1), ended: make(chan struct{})}
	s.mu.Lock()
	s.by[attemptID] = ra
	s.mu.Unlock()
	return ra
}

func (s *remoteSink) forget(attemptID string) {
	s.mu.Lock()
	delete(s.by, attemptID)
	s.mu.Unlock()
}

func (s *remoteSink) attempt(attemptID string) *remoteAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.by[attemptID]
}

// byLease finds the rendezvous of a lease id ("lease-"+attempt id).
func (s *remoteSink) byLease(leaseID string) *remoteAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ra := range s.by {
		if ra.leaseID == leaseID {
			return ra
		}
	}
	return nil
}

// release forgets every rendezvous of the run.
func (s *remoteSink) release() {
	s.mu.Lock()
	s.by = map[string]*remoteAttempt{}
	s.mu.Unlock()
}

// carrierStart is called by the transport adapter before a driver runs: it makes the carrier
// cancellable by attempt id and lets the lease be renewed (RenewLease).
func (s *remoteSink) carrierStart(ctx context.Context, attemptID, leaseID string) (context.Context, func()) {
	cctx, cancel := context.WithCancel(ctx)
	leaseEngines.Store(leaseID, s.eng)
	ra := s.attempt(attemptID)
	if ra != nil {
		ra.mu.Lock()
		ra.cancel = cancel
		early := ra.cancelled
		ra.mu.Unlock()
		if early {
			cancel() // the run was cancelled before this carrier started
		}
	}
	return cctx, func() {
		leaseEngines.Delete(leaseID)
		cancel()
		if ra != nil {
			close(ra.ended)
		}
	}
}

// cancelCarrier stops the carrier of an attempt: the driver tells the agent to cancel and the
// agent kills the job's process group.
func (s *remoteSink) cancelCarrier(attemptID string) {
	if ra := s.attempt(attemptID); ra != nil {
		ra.mu.Lock()
		ra.cancelled = true
		cancel := ra.cancel
		ra.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
}

// ended is the dispatcher's DeliveryError: the carrier returned an error, so the attempt is lost.
func (s *remoteSink) ended(attemptID string, cause error) {
	if ra := s.attempt(attemptID); ra != nil {
		select {
		case ra.lost <- cause:
		default:
		}
	}
}

// markRunning moves a leased attempt to running on the first sign of life of its job.
func (s *remoteSink) markRunning(ctx context.Context, ra *remoteAttempt) error {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	if ra.running {
		return nil
	}
	state, epoch, err := s.st.AttemptStateEpoch(ctx, ra.attemptID)
	if err != nil {
		return err
	}
	if state == "leased" {
		if err = s.st.Transition(ctx, ra.attemptID, "leased", "running", store.TransitionDetail{Epoch: epoch}); err != nil {
			return err
		}
	}
	ra.running = state == "leased" || state == "running"
	return nil
}

// Log passes one output line to the run.
func (s *remoteSink) Log(ctx context.Context, l protocol.Log) error {
	ra := s.byLease(l.LeaseID)
	if ra == nil {
		return fail(CodeRun, "log for unknown lease %s", l.LeaseID)
	}
	if err := s.markRunning(ctx, ra); err != nil {
		return err
	}
	text := l.Text
	if text == "" || text[len(text)-1] != '\n' {
		text += "\n"
	}
	ra.write([]byte(text))
	return nil
}

// Result accepts the node's evidence for the lease's current epoch and moves the attempt to
// finalizing; the run loop settles it from the evidence.
func (s *remoteSink) Result(ctx context.Context, r protocol.Result) error {
	ra := s.byLease(r.LeaseID)
	if ra == nil {
		return fail(CodeRun, "result for unknown lease %s", r.LeaseID)
	}
	if err := s.markRunning(ctx, ra); err != nil {
		return err
	}
	if err := s.eng.Result(ctx, r.LeaseID, int64(r.Epoch)); err != nil {
		return err
	}
	ev := ra.checkEvidence(r.Evidence)
	select {
	case ra.result <- ev:
	default:
	}
	return nil
}

// ArtifactRef and CacheRef are not collected by a foreground run (see the file comment).
func (*remoteSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (*remoteSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }

// Blob discards the bytes: the digest check of the driver still runs.
func (*remoteSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return &discardBlob{}, nil
}

type discardBlob struct{ n int64 }

func (b *discardBlob) Write(p []byte) (int, error) { b.n += int64(len(p)); return len(p), nil }
func (b *discardBlob) Close() error                { return nil }
func (b *discardBlob) Offset() int64               { return b.n }
func (b *discardBlob) Abort() error                { return nil }

var _ transport.Sink = (*remoteSink)(nil)

// checkEvidence decodes the node's evidence and holds it to the delivery: a document that does not
// parse, is not ci.evidence/v1, names another attempt or revision, or passes with a failing check is an
// error result and never a pass. The envelope (lease and epoch) was already fenced by the driver.
func (ra *remoteAttempt) checkEvidence(raw json.RawMessage) model.Evidence {
	bad := model.Evidence{Result: "error"}
	var ev model.Evidence
	if err := json.Unmarshal(raw, &ev); err != nil || ev.Result == "" || ev.Schema != "ci.evidence/v1" {
		return bad
	}
	if ev.Attempt == nil || ev.Attempt.ID != ra.attemptID || ra.revision != "" && !strings.EqualFold(ev.Revision, ra.revision) {
		return bad
	}
	if ev.Result == "pass" {
		for _, c := range ev.Checks {
			if c.Result == "fail" || c.Result == "error" {
				return bad
			}
		}
	}
	return ev
}
