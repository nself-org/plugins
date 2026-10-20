package transfer

// Purpose: the receiving half of a blob transfer (Epic CACHE D7, protocol v1
// digest/offset/chunk): it turns a `digest` announce into a writer the carrier
// driver streams `chunk` data into, backed by a resumable cas Partial. The
// writer reports the bytes it already holds (the `offset` answer), so a
// restarted sender continues instead of starting over.
// Inputs:  a transport.BlobHeader (stream, sha256, size), the limits of the
// lease, a liveness check for the lease epoch, a cas.Store and a realm.
// Outputs: a Writer (transport.BlobWriter) whose Close publishes the blob in the
// realm only when the whole-blob digest matches; E714 refusals.
// Constraints: every refusal happens before a byte is stored (file cap, lease
// byte budget, disk floor, epoch); the epoch is re-checked on every Write and on
// Close; a blob is visible only after the final digest matches; the partial of a
// lease is keyed by a digest of the lease id, so another lease never appends to
// it; no network, no process, no SQL.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// StreamArtifact is the protocol stream of artifact blobs.
const StreamArtifact = "artifact"

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Limits bound what one lease may send.
type Limits struct {
	// MaxFile refuses an announce above it (NSELF_CI_ARTIFACT_MAX_FILE). Zero or less refuses everything.
	MaxFile int64
	// Budget returns how many bytes the lease may still take for a NEW blob with this digest: what
	// the caller holds against the lease (other blobs announced and not yet recorded) is already
	// subtracted, so a re-announce of the same digest is never charged twice. Nil means no total bound.
	Budget func(ctx context.Context, sha string) (int64, error)
	// Disk refuses when writing add more bytes would cross the free-disk floor. Nil skips it.
	Disk func(add int64) error
}

// Receiver opens writers for the blobs of one lease.
type Receiver struct {
	Store *cas.Store
	Realm cas.Realm
	// Lease names the partial owner; any string, it is hashed.
	Lease  string
	Limits Limits
	// Live returns nil while the lease epoch is the current one. An error wrapping ErrStale is a
	// stale epoch (refused, audited); any other error (a store fault) is a transient failure.
	Live func(ctx context.Context) error
	// Refused is told once per refused blob and reason, so the caller can
	// write its audit row. It must not block.
	Refused func(ctx context.Context, sha string, err *Error)

	mu   sync.Mutex
	told map[string]bool
}

// ErrStale is wrapped by a Live error that means "this lease epoch is no longer current".
var ErrStale = errors.New("the lease epoch is no longer current")

func (r *Receiver) writerID() string {
	sum := sha256.Sum256([]byte(r.Lease))
	return "w" + hex.EncodeToString(sum[:16])
}

// epoch returns an E714 stale_epoch error when the lease is no longer current, and tells Refused once per blob.
func (r *Receiver) epoch(ctx context.Context, sha string) error {
	if r.Live == nil {
		return Failf(ReasonStaleEpoch, "no lease check is wired; the blob is refused")
	}
	err := r.Live(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrStale) {
		return Failf(ReasonInterrupted, "the lease check failed: %v", err)
	}
	e := Failf(ReasonStaleEpoch, "the lease epoch is no longer current; blob %s is refused", short(sha))
	r.notify(ctx, sha, e)
	return e
}

// Open admits an announced blob. A blob the realm already holds opens as a finished writer
// (offset == size), which is how a duplicate is answered without a byte on the wire.
func (r *Receiver) Open(ctx context.Context, h transport.BlobHeader) (writer *Writer, resultErr error) {
	defer func() { r.notify(ctx, h.SHA256, resultErr) }()
	if h.Stream != StreamArtifact {
		return nil, Failf(ReasonProtocol, "the %q stream is not carried here", h.Stream)
	}
	if !sha256Hex.MatchString(h.SHA256) || h.Size < 1 {
		return nil, Failf(ReasonProtocol, "the announce is not a sha256 with a size above zero")
	}
	if err := r.epoch(ctx, h.SHA256); err != nil {
		return nil, err
	}
	if h.Size > r.Limits.MaxFile {
		return nil, Failf(ReasonSizeCap, "blob %s is %d B, above the per-file cap %d B (NSELF_CI_ARTIFACT_MAX_FILE)", short(h.SHA256), h.Size, r.Limits.MaxFile)
	}
	if info, err := r.Store.Stat(r.Realm, h.SHA256); err == nil {
		if info.Size() != h.Size {
			return nil, Failf(ReasonDigest, "the realm holds blob %s at %d B, not the announced %d B", short(h.SHA256), info.Size(), h.Size)
		}
		// held already: re-verify its bytes and restart its sweep grace (cas.Refresh), so the blob a
		// record is about to reference cannot be swept between this announce and that record
		switch rerr := r.Store.Refresh(r.Realm, h.SHA256); {
		case rerr == nil:
			return &Writer{r: r, h: h, done: true, offset: h.Size}, nil
		case !errors.Is(rerr, os.ErrNotExist):
			return nil, Failf(ReasonDigest, "the stored blob %s failed re-verification: %v", short(h.SHA256), rerr)
		}
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, cas.ErrInvalid) {
		return nil, err
	}
	if err := r.admit(ctx, h); err != nil {
		return nil, err
	}
	if r.Limits.Disk != nil {
		if err := r.Limits.Disk(h.Size); err != nil {
			return nil, err
		}
	}
	p, err := r.Store.Partial(r.Realm, h.SHA256, r.writerID())
	if err != nil {
		if errors.Is(err, cas.ErrPartialConflict) {
			return nil, Failf(ReasonInterrupted, "the partial of blob %s is inconsistent; the sweep removes it", short(h.SHA256))
		}
		return nil, err
	}
	if p.Offset() > h.Size {
		return nil, Failf(ReasonInterrupted, "the partial of blob %s holds more bytes than announced", short(h.SHA256))
	}
	return &Writer{r: r, h: h, p: p, offset: p.Offset()}, nil
}

// admit refuses a new blob the lease has no room for.
func (r *Receiver) admit(ctx context.Context, h transport.BlobHeader) error {
	if r.Limits.Budget == nil {
		return nil
	}
	left, err := r.Limits.Budget(ctx, h.SHA256)
	if err != nil {
		return err
	}
	if h.Size > left {
		return Failf(ReasonSizeCap, "blob %s (%d B) would take the lease past its byte budget (%d B left)", short(h.SHA256), h.Size, max(left, 0))
	}
	return nil
}

// notify reports each stable refusal once per blob and reason. Store faults are
// not verdicts and are deliberately omitted.
func (r *Receiver) notify(ctx context.Context, sha string, err error) {
	var e *Error
	if !errors.As(err, &e) || e.Reason == ReasonInterrupted {
		return
	}
	if !sha256Hex.MatchString(sha) {
		sha = ""
	}
	key := sha + "/" + e.Reason
	r.mu.Lock()
	first := !r.told[key]
	if r.told == nil {
		r.told = map[string]bool{}
	}
	r.told[key] = true
	r.mu.Unlock()
	if first && r.Refused != nil {
		r.Refused(ctx, sha, e)
	}
}
