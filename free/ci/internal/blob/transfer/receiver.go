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
	// Budget returns how many more bytes the lease may take in total, before the bytes this
	// receiver already accepted are subtracted. Nil means no total bound.
	Budget func(ctx context.Context) (int64, error)
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
	// Live returns nil while the lease epoch is the current one. Any error is a stale epoch.
	Live func(ctx context.Context) error
	// Refused is told once per refused blob (by digest) when the epoch is stale, so the caller can
	// write its audit row. It must not block.
	Refused func(ctx context.Context, sha string, err *Error)

	mu       sync.Mutex
	accepted int64
	told     map[string]bool
}

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
	e := Failf(ReasonStaleEpoch, "the lease epoch is no longer current; blob %s is refused", short(sha))
	r.mu.Lock()
	first := !r.told[sha]
	if r.told == nil {
		r.told = map[string]bool{}
	}
	r.told[sha] = true
	r.mu.Unlock()
	if first && r.Refused != nil {
		r.Refused(ctx, sha, e)
	}
	return e
}

// Open admits an announced blob. A blob the realm already holds opens as a finished writer
// (offset == size), which is how a duplicate is answered without a byte on the wire.
func (r *Receiver) Open(ctx context.Context, h transport.BlobHeader) (*Writer, error) {
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
	if err := r.admit(ctx, h); err != nil {
		return nil, err
	}
	if info, err := r.Store.Stat(r.Realm, h.SHA256); err == nil {
		if info.Size() != h.Size {
			return nil, Failf(ReasonDigest, "the realm holds blob %s at %d B, not the announced %d B", short(h.SHA256), info.Size(), h.Size)
		}
		return &Writer{r: r, h: h, done: true, offset: h.Size}, nil
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, cas.ErrInvalid) {
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

// admit counts the blob against the lease byte budget.
func (r *Receiver) admit(ctx context.Context, h transport.BlobHeader) error {
	var left int64 = -1
	if r.Limits.Budget != nil {
		b, err := r.Limits.Budget(ctx)
		if err != nil {
			return err
		}
		left = b
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if left >= 0 && r.accepted+h.Size > left {
		return Failf(ReasonSizeCap, "blob %s (%d B) would take the lease past its byte budget (%d B left)", short(h.SHA256), h.Size, max(left-r.accepted, 0))
	}
	r.accepted += h.Size
	return nil
}
