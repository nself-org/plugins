package remote

// Purpose: the node half of a cache restore (Epic CACHE D7, P7-CACHE-21): a blob the read-through
// store lacks (or holds corrupt) is asked for from the coordinator, received as digest and
// chunk frames, re-hashed on arrival and published only when whole and correct.
// Inputs:  the lease's cache plan, the read-through store and the lease's transfer channel (the
// coordinator's digest and chunk frames, and a function that sends `offset` frames).
// Outputs: a verified, pinned copy; `offset` frames: a request (the digest and byte 0), the
// answer to an announce (the position held), a resync, and the confirmation (the size) once the
// copy is published.
// Constraints: only digests of the lease plan are asked for and only at the planned size; the
// announce must match the plan; a chunk out of order is answered with the expected position
// (at most maxResync times); the whole-blob digest is checked before the copy is published and
// ReadThrough.Open re-hashes it again; a corrupt transfer is asked for again up to maxTransit
// times; a transfer that gets no answer marks the fetcher broken so the rest of the lease
// degrades to local copies at once; nothing here fails the job.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Fetch limits.
const (
	// DefaultFirstWait bounds the wait for the coordinator's first announce of a blob.
	DefaultFirstWait = 15 * time.Second
	// DefaultIdleWait bounds the wait for the next frame of a running transfer.
	DefaultIdleWait = 30 * time.Second
	maxResync       = 3
	maxTransit      = 3
	maxStalls       = 2
)

var (
	errTransit = errors.New("E714 digest_mismatch: the blob changed in transit")
	// ErrNoRestore means the coordinator did not serve the blob; the caller falls back to a cold miss.
	ErrNoRestore = errors.New("E711 not_planned: the coordinator did not serve the cache blob")
)

// Wire is the node's end of the lease's transfer channel.
type Wire struct {
	// Offset sends one offset frame to the coordinator.
	Offset func(protocol.Offset) error
	// In carries the coordinator's digest and chunk frames of the lease (Body *protocol.Digest or *protocol.Chunk).
	In <-chan protocol.Message
}

// Fetcher fetches the restore blobs of one lease.
type Fetcher struct {
	Wire Wire
	// First and Idle override the default waits when set.
	First, Idle time.Duration

	plan   map[string]int64
	broken bool
}

// NewFetcher returns a fetcher for the restore blobs of plan; nil when there is nothing to fetch.
func NewFetcher(w Wire, plan *protocol.LeaseCache) *Fetcher {
	if plan == nil || w.Offset == nil || w.In == nil {
		return nil
	}
	f := &Fetcher{Wire: w, plan: map[string]int64{}}
	for _, r := range plan.Restore {
		for _, b := range r.Blobs {
			f.plan[strings.ToLower(b.SHA256)] = b.Size
		}
	}
	return f
}

func (f *Fetcher) first() time.Duration {
	if f.First > 0 {
		return f.First
	}
	return DefaultFirstWait
}

func (f *Fetcher) idle() time.Duration {
	if f.Idle > 0 {
		return f.Idle
	}
	return DefaultIdleWait
}

// Open returns a verified copy of the blob: the node's own when it checks out, else one fetched
// from the coordinator. A nil Fetcher serves node copies only.
func (f *Fetcher) Open(ctx context.Context, rt *ReadThrough, realm, sha string, size int64) (*os.File, error) {
	file, err := rt.Open(ctx, realm, sha, size, nil)
	if err == nil || !errors.Is(err, os.ErrNotExist) || f == nil {
		return file, err
	}
	if f.broken || size < 1 {
		return nil, ErrNoRestore
	}
	for try := 0; ; try++ {
		file, err = rt.Open(ctx, realm, sha, size, func(c context.Context, w io.Writer) error { return f.fetch(c, sha, size, w) })
		switch {
		case err == nil:
			_ = f.Wire.Offset(protocol.Offset{Stream: "cache", SHA256: sha, Offset: size}) // the copy is published: confirm it
			return file, nil
		case errors.Is(err, errTransit) && try < maxTransit:
			continue
		}
		return nil, err
	}
}

// fetch asks for the blob and writes what arrives to w; it returns nil only for a whole blob whose digest is sha.
func (f *Fetcher) fetch(ctx context.Context, sha string, size int64, w io.Writer) error {
	if want, ok := f.plan[sha]; !ok || want != size {
		return fmt.Errorf("E711 not_planned: blob %s is not in the lease plan", sha[:min(len(sha), 12)])
	}
	for drained := false; !drained; { // frames of an earlier blob are not ours
		select {
		case <-f.Wire.In:
		default:
			drained = true
		}
	}
	if err := f.Wire.Offset(protocol.Offset{Stream: "cache", SHA256: sha, Offset: 0}); err != nil {
		f.broken = true
		return err
	}
	rc := &inflight{sha: sha, size: size, h: sha256.New(), w: w, lastResync: -1}
	deadline := time.Now().Add(f.first())
	for {
		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			if rc.announced && rc.stalls < maxStalls { // frames were lost (the last chunk, say): ask for the position again
				rc.stalls++
				if err := rc.offset(f.Wire.Offset, rc.next); err != nil {
					f.broken = true
					return err
				}
				deadline = time.Now().Add(f.idle())
				continue
			}
			f.broken = true // a coordinator that does not answer is not asked again for this lease
			return ErrNoRestore
		case m := <-f.Wire.In:
			timer.Stop()
			var done, ours bool
			var err error
			switch b := m.Body.(type) {
			case *protocol.Digest:
				ours = sameDigest(b.SHA256, sha)
				err = rc.announce(f.Wire.Offset, b)
			case *protocol.Chunk:
				ours = rc.announced && sameDigest(b.Digest, sha)
				done, err = rc.chunk(f.Wire.Offset, b)
			}
			if err != nil || done {
				if rc.sendFailed {
					f.broken = true // the carrier cannot take our frames: no more asking on this lease
				}
				return err
			}
			if ours { // only progress of this blob extends the wait; stray frames do not
				deadline = time.Now().Add(f.idle())
			}
		}
	}
}

func sameDigest(got, want string) bool {
	return strings.ToLower(strings.TrimPrefix(got, "sha256:")) == want
}

// inflight is the state of one blob being received.
type inflight struct {
	sha        string
	size, next int64
	h          hash.Hash
	w          io.Writer
	announced  bool
	resyncs    int
	stalls     int
	lastResync int64
	sendFailed bool
}

func (rc *inflight) offset(send func(protocol.Offset) error, at int64) error {
	err := send(protocol.Offset{Stream: "cache", SHA256: rc.sha, Offset: at})
	rc.sendFailed = err != nil
	return err
}

// announce answers the coordinator's announce with the position held (a resume point after a cut).
func (rc *inflight) announce(send func(protocol.Offset) error, d *protocol.Digest) error {
	if !sameDigest(d.SHA256, rc.sha) {
		return nil
	}
	if d.Size != rc.size {
		return fmt.Errorf("E714 digest_mismatch: the coordinator announced %d B for a planned %d B blob", d.Size, rc.size)
	}
	rc.announced = true
	return rc.offset(send, rc.next)
}

// chunk applies one chunk frame; done is true after the final chunk of a correct blob. A chunk that
// is not at the held position asks once for that position (the frames already in flight behind
// it are dropped without another answer).
func (rc *inflight) chunk(send func(protocol.Offset) error, c *protocol.Chunk) (bool, error) {
	if !rc.announced || !sameDigest(c.Digest, rc.sha) {
		return false, nil
	}
	if c.Offset != rc.next {
		if rc.lastResync == rc.next {
			return false, nil
		}
		rc.lastResync = rc.next
		rc.resyncs++
		if rc.resyncs > maxResync {
			return false, fmt.Errorf("E714 digest_mismatch: the coordinator kept sending out of order")
		}
		return false, rc.offset(send, rc.next)
	}
	data, err := base64.StdEncoding.Strict().DecodeString(c.Data)
	if err != nil || rc.next+int64(len(data)) > rc.size {
		return false, errTransit
	}
	if _, err = rc.w.Write(data); err != nil {
		return false, err
	}
	_, _ = rc.h.Write(data)
	rc.next += int64(len(data))
	if !c.Final {
		return false, nil
	}
	if rc.next != rc.size || hex.EncodeToString(rc.h.Sum(nil)) != rc.sha {
		return false, errTransit
	}
	return true, nil
}
