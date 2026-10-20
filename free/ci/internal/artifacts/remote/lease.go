package remote

// Purpose: the handler of one lease: liveness, declarations, blob admission and
// artifact-ref intake (see handler.go).

import (
	"context"
	"errors"
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
	reserved int64
	refs     map[string]int // path -> refs seen, to pick the nth declaration
	names    map[string]bool
	audited  map[string]bool
}

// live is nil while the lease is current and the attempt can still take uploads.
func (l *Lease) live(ctx context.Context) error {
	if l.b.Fence != nil {
		if err := l.b.Fence(ctx); err != nil {
			return err
		}
	}
	state, _, err := l.h.Ledger.AttemptStateEpoch(ctx, l.b.AttemptID)
	if err != nil {
		return err
	}
	if state != "leased" && state != "running" {
		return errors.New("attempt " + string(state) + " takes no more uploads")
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

// ready moves the attempt to running and returns its stored facts.
func (l *Lease) ready(ctx context.Context) (records.Facts, error) {
	if l.b.Ready != nil {
		if err := l.b.Ready(ctx); err != nil {
			return records.Facts{}, err
		}
	}
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

// attemptEpoch is the attempt's own epoch, read after the lease was fenced: the record writes
// are gated on it by the store, so a transition in between is a refusal, not a stale write.
func (l *Lease) attemptEpoch(ctx context.Context) (int64, error) {
	_, e, err := l.h.Ledger.AttemptStateEpoch(ctx, l.b.AttemptID)
	return e, err
}

// Declare writes the attempt's declarations once. The agent's upload is checked against them and
// CommitAndVerify fails a declaration nothing was delivered for, so a job that declares outputs
// cannot pass without them. It is idempotent.
func (l *Lease) Declare(ctx context.Context) error { return l.declare(ctx, "") }

func (l *Lease) declare(ctx context.Context, sha string) error {
	if err := l.live(ctx); err != nil {
		e := transfer.Failf(transfer.ReasonStaleEpoch, "the lease is no longer current: %v", err)
		l.refuse(ctx, sha, e)
		return e
	}
	f, err := l.ready(ctx)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.declared {
		return nil
	}
	decls, err := records.Normalize(l.b.Job, f.Release, records.ImplicitDecls(l.b.Job))
	if err != nil {
		return err
	}
	if have, err := l.h.Ledger.ArtifactDecls(ctx, l.b.AttemptID); err != nil {
		return err
	} else if len(have) > 0 {
		decls = have
	} else if len(decls) > 0 {
		e, err := l.attemptEpoch(ctx)
		if err != nil {
			return err
		}
		if err = l.h.Ledger.ArtifactDeclare(ctx, l.b.AttemptID, e, decls); err != nil {
			return err
		}
	}
	l.decls, l.declared = decls, true
	return nil
}

func (l *Lease) receiver(f records.Facts) *transfer.Receiver {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rcv == nil {
		pol := l.h.Policy
		l.rcv = &transfer.Receiver{Store: l.h.Blobs, Realm: cas.ArtifactRealm(f.Project), Lease: l.b.LeaseID, Live: l.live, Refused: l.refuse,
			Limits: transfer.Limits{MaxFile: pol.MaxFile, Disk: pol.CheckDisk, Budget: func(ctx context.Context) (int64, error) { return l.budget(ctx, f.Project) }}}
	}
	return l.rcv
}

// budget is how many bytes the store still has room for in this project (the total cap and, in a
// TEAM store, the project's share of it).
func (l *Lease) budget(ctx context.Context, project string) (int64, error) {
	u, err := l.h.Ledger.ArtifactUsage(ctx, project)
	if err != nil {
		return 0, err
	}
	pol := l.h.Policy
	left := pol.MaxTotal - u.Total
	if pol.Team {
		left = min(left, pol.MaxTotal/100*int64(pol.ProjectShare)-u.Project)
	}
	return max(left, 0), nil
}

// Blob admits one announced artifact blob (transport.Sink.Blob for the artifact stream).
func (l *Lease) Blob(ctx context.Context, h xport.BlobHeader) (xport.BlobWriter, error) {
	if h.Stream != transfer.StreamArtifact {
		return nil, transfer.Failf(transfer.ReasonProtocol, "the %q stream is not an artifact stream", h.Stream)
	}
	if err := l.declare(ctx, h.SHA256); err != nil {
		return nil, err
	}
	f, err := l.ready(ctx)
	if err != nil {
		return nil, err
	}
	w, err := l.receiver(f).Open(ctx, h)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.reserved += h.Size
	total := l.reserved
	l.mu.Unlock()
	e, err := l.attemptEpoch(ctx)
	if err == nil {
		err = l.h.Ledger.ArtifactReserve(ctx, l.b.AttemptID, e, total, l.h.Policy)
	}
	if err != nil {
		_ = w.Abort()
		return nil, err
	}
	return w, nil
}
