package remote

// Purpose: the handler of one lease: liveness, declarations, blob admission and
// artifact-ref intake (see handler.go).

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	xport "github.com/nself-org/plugins/free/ci/internal/transport"
)

// Lease is the handler of one delivered lease. It is safe for concurrent use.
type Lease struct {
	h *Handler
	b Binding

	mu       sync.Mutex
	facts    *records.Facts
	rcv      *transfer.Receiver
	declared bool
	decls    []records.Decl
	pending  map[string]int64            // digest -> announced size of blobs held for this lease and not yet recorded by a ref
	rowHeld  int64                       // what this attempt's reserve row holds in the store now (ArtifactUsage counts it)
	staged   int64                       // bytes of the records this lease staged (the store's reserve gate does not count them)
	writers  map[string]*transfer.Writer // digest -> the writer Blob handed out: its bytes were verified in this lease
	refs     map[string]int              // path -> refs seen, to pick the nth declaration
	names    map[string]bool
	audited  map[string]bool
}

// stale wraps cause as "this lease is no longer current" (transfer.ErrStale), the one class of
// refusal that is audited and reported as E714 stale_epoch.
func stale(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{transfer.ErrStale}, a...)...)
}

// fence is nil while the lease epoch is current.
func (l *Lease) fence(ctx context.Context) error {
	if l.b.Fence == nil {
		return nil
	}
	if err := l.b.Fence(ctx); err != nil {
		if errors.Is(err, transfer.ErrStale) {
			return stale("%v", err)
		}
		return err // a fault of the store or the engine, not a verdict on the lease
	}
	return nil
}

// live is nil while the lease is current and the attempt can still take uploads. A store fault is
// returned as it is, not as a stale epoch.
func (l *Lease) live(ctx context.Context) error {
	if err := l.fence(ctx); err != nil {
		return err
	}
	state, _, err := l.h.Ledger.AttemptStateEpoch(ctx, l.b.AttemptID)
	if err != nil {
		return err
	}
	if state != "leased" && state != "running" {
		return stale("attempt %s takes no more uploads", state)
	}
	return nil
}

// refuse audits a stale-epoch refusal: one row per blob (by digest; "" is the result frame).
func (l *Lease) refuse(ctx context.Context, sha string, e *transfer.Error) {
	l.mu.Lock()
	if l.audited == nil {
		l.audited = map[string]bool{}
	}
	done := l.audited[sha]
	l.audited[sha] = true
	l.mu.Unlock()
	if done {
		return
	}
	short := sha
	if len(short) > 12 {
		short = short[:12]
	}
	_ = l.h.Ledger.AppendAudit(ctx, "artifacts", "artifact.remote.refused", l.b.AttemptID, "reason="+e.Reason+" lease="+l.b.LeaseID+" sha="+short)
}

// refusal turns a guard error into the error the caller returns: a stale lease is E714 stale_epoch
// with one audit row for the blob; anything else (a store fault) passes through.
func (l *Lease) refusal(ctx context.Context, sha string, err error) error {
	if !errors.Is(err, transfer.ErrStale) {
		return err
	}
	e := transfer.Failf(transfer.ReasonStaleEpoch, "the lease is no longer current: %v", err)
	l.refuse(ctx, sha, e)
	return e
}

// guard proves the lease is current, makes the attempt running, and returns the attempt epoch the
// record writes are gated on. The epoch is read between two fences, so it belongs to this lease's
// incarnation: a transition in between would have moved the lease too.
func (l *Lease) guard(ctx context.Context, sha string) (int64, error) {
	if err := l.live(ctx); err != nil {
		return 0, l.refusal(ctx, sha, err)
	}
	if l.b.Ready != nil {
		if err := l.b.Ready(ctx); err != nil {
			return 0, err
		}
	}
	state, epoch, err := l.h.Ledger.AttemptStateEpoch(ctx, l.b.AttemptID)
	if err != nil {
		return 0, err
	}
	if state != "running" {
		return 0, l.refusal(ctx, sha, stale("attempt %s, not running", state))
	}
	if err = l.fence(ctx); err != nil {
		return 0, l.refusal(ctx, sha, err)
	}
	return epoch, nil
}

// facts reads the stored facts of the attempt once.
func (l *Lease) factsOnce(ctx context.Context) (records.Facts, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.facts != nil {
		return *l.facts, nil
	}
	f, err := l.h.Ledger.ArtifactFacts(ctx, l.b.AttemptID)
	if err != nil {
		return f, err
	}
	l.facts = &f
	return f, nil
}

// Declare writes the attempt's declarations once. The agent's upload is checked against them and
// CommitAndVerify fails a declaration nothing was delivered for, so a job that declares outputs
// cannot pass without them. It is idempotent per lease; a declaration left by an earlier
// incarnation of the same attempt row is superseded by the store (ArtifactDeclare).
func (l *Lease) Declare(ctx context.Context) error {
	_, err := l.declare(ctx, "")
	return err
}

// declare returns the attempt epoch of the guard it ran.
func (l *Lease) declare(ctx context.Context, sha string) (int64, error) {
	epoch, err := l.guard(ctx, sha)
	if err != nil {
		return 0, err
	}
	f, err := l.factsOnce(ctx)
	if err != nil {
		return 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.declared {
		return epoch, nil
	}
	decls, err := records.Normalize(l.b.Job, f.Release, records.ImplicitDecls(l.b.Job))
	if err != nil {
		return 0, err
	}
	if len(decls) > 0 {
		if err = l.h.Ledger.ArtifactDeclare(ctx, l.b.AttemptID, epoch, decls); records.IsReason(err, records.ReasonDeclaration) {
			if decls, err = l.h.Ledger.ArtifactDecls(ctx, l.b.AttemptID); err == nil && len(decls) == 0 {
				err = records.Failf(records.ReasonDeclaration, "attempt %s has declarations the store refuses to write", l.b.AttemptID)
			}
		}
		if err != nil {
			return 0, err
		}
	}
	l.decls, l.declared = decls, true
	return epoch, nil
}

func (l *Lease) receiver(f records.Facts) *transfer.Receiver {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rcv == nil {
		pol := l.h.Policy
		l.rcv = &transfer.Receiver{Store: l.h.Blobs, Realm: cas.ArtifactRealm(f.Project), Lease: fmt.Sprintf("%s/%d", l.b.LeaseID, l.b.Epoch), Live: l.live, Refused: l.refuse,
			Limits: transfer.Limits{MaxFile: pol.MaxFile, Disk: pol.CheckDisk, Budget: func(ctx context.Context, sha string) (int64, error) { return l.budget(ctx, f.Project, sha) }}}
	}
	return l.rcv
}

// budget is how many bytes the store still has room for in this project (the total cap and, in a
// TEAM store, the project's share of it) for a new blob with this digest: the usage already holds the
// records this lease staged, and the blobs it announced but has not recorded yet are subtracted, the
// blob in question excluded so a resume is not charged twice.
func (l *Lease) budget(ctx context.Context, project, sha string) (int64, error) {
	u, err := l.h.Ledger.ArtifactUsage(ctx, project)
	if err != nil {
		return 0, err
	}
	var held int64
	l.mu.Lock()
	for d, n := range l.pending {
		if d != sha {
			held += n
		}
	}
	own := l.rowHeld // the usage includes this attempt's reserve row; it is counted through pending instead
	l.mu.Unlock()
	pol := l.h.Policy
	left := pol.MaxTotal - (u.Total - own) - held
	if pol.Team {
		left = min(left, pol.MaxTotal/100*int64(pol.ProjectShare)-(u.Project-own)-held)
	}
	return max(left, 0), nil
}

// Blob admits one announced artifact blob (transport.Sink.Blob for the artifact stream).
func (l *Lease) Blob(ctx context.Context, h xport.BlobHeader) (xport.BlobWriter, error) {
	if h.Stream != transfer.StreamArtifact {
		return nil, transfer.Failf(transfer.ReasonProtocol, "the %q stream is not an artifact stream", h.Stream)
	}
	epoch, err := l.declare(ctx, h.SHA256)
	if err != nil {
		return nil, err
	}
	f, err := l.factsOnce(ctx)
	if err != nil {
		return nil, err
	}
	w, err := l.receiver(f).Open(ctx, h)
	if err != nil {
		return nil, err
	}
	if w.Held() {
		l.mu.Lock()
		if l.writers == nil {
			l.writers = map[string]*transfer.Writer{}
		}
		l.writers[h.SHA256] = w
		l.mu.Unlock()
		return w, nil // the realm holds these bytes already (re-verified by Open): nothing to reserve
	}
	l.mu.Lock()
	if l.pending == nil {
		l.pending, l.writers = map[string]int64{}, map[string]*transfer.Writer{}
	}
	l.pending[h.SHA256], l.writers[h.SHA256] = h.Size, w
	total := l.staged // the store's reserve gate counts other attempts' staged rows, never this attempt's own
	for _, n := range l.pending {
		total += n
	}
	l.mu.Unlock()
	if err = l.h.Ledger.ArtifactReserve(ctx, l.b.AttemptID, epoch, total, l.h.Policy); err != nil {
		_ = w.Abort()
		l.mu.Lock()
		delete(l.pending, h.SHA256)
		l.mu.Unlock()
		return nil, err
	}
	l.mu.Lock()
	l.rowHeld = total
	l.mu.Unlock()
	return w, nil
}

// verified reports whether this lease itself published or re-verified the blob, so a ref need not
// hash it again (CommitAndVerify does, before anything can pass).
func (l *Lease) verified(sha string) bool {
	l.mu.Lock()
	w := l.writers[sha]
	l.mu.Unlock()
	return w != nil && (w.Published() || w.Held())
}

// recorded releases the claim of a blob a ref has turned into a record.
func (l *Lease) recorded(sha string, size int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.pending, sha)
	l.staged += size
	l.rowHeld = 0 // staging a record deletes the attempt's reserve row: the record carries the claim now
}
