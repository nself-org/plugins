package remote

// Purpose: the node half of a cache restore (P7-CACHE-21, Epic CACHE D7): answer
// the coordinator's `digest` announces from the read-through copies, take the
// `chunk` frames of a blob the node lacks, and publish a blob only after its
// re-hash matches.
// Inputs:  the lease's restore blobs, a transport.BlobFeed and a ReadThrough.
// Outputs: verified copies in the read-through store; `offset` answers (the size
// for a copy held, 0 before a fetch, 0 again after a failed re-hash).
// Constraints: only a blob of the lease plan, with the planned size, is taken: any
// other announce or chunk is ignored; a held copy is re-hashed on every use (a bad
// one is discarded and fetched again); a fetch failure leaves no copy, so the
// caller binds nothing and the job runs cold; every wait is bounded by the
// context and by Wait; a blob is retried at most maxNodeTries times.

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// DefaultNodeRestoreWait bounds the wait for each coordinator frame.
const DefaultNodeRestoreWait = 20 * time.Second

const maxNodeTries = 4

var errRestart = errors.New("E714 interrupted: the coordinator restarted the blob")

// NodeRestorer receives a lease's restore blobs from the coordinator.
type NodeRestorer struct {
	Copies *ReadThrough
	Feed   transport.BlobFeed
	// Realm is the lease's cache namespace (the read-through realm).
	Realm string
	// Wait overrides DefaultNodeRestoreWait.
	Wait time.Duration

	carry *transport.BlobFrame // a re-announce that cut a stream: handled next
}

// Fetch returns the digests of the blobs now held as verified copies. It returns
// when every blob is held, a blob ran out of tries, or a frame did not arrive.
func (n *NodeRestorer) Fetch(ctx context.Context, blobs []protocol.CacheBlob) map[string]bool {
	got := map[string]bool{}
	pending := map[string]protocol.CacheBlob{}
	for _, b := range blobs {
		pending[b.SHA256] = b
	}
	tries := map[string]int{}
	for len(pending) > 0 {
		fr, err := n.next(ctx)
		if err != nil {
			return got
		}
		if fr.Digest == nil {
			continue // a chunk with no open blob
		}
		sha := normDigest(fr.Digest.SHA256)
		b, ok := pending[sha]
		if !ok || fr.Digest.Stream != StreamCache || fr.Digest.Size != b.Size {
			continue // outside the plan: never answered, never stored
		}
		if tries[sha]++; tries[sha] > maxNodeTries {
			delete(pending, sha)
			continue
		}
		if n.receive(ctx, b) {
			got[sha] = true
			delete(pending, sha)
		}
	}
	return got
}

func (n *NodeRestorer) wait() time.Duration {
	if n.Wait > 0 {
		return n.Wait
	}
	return DefaultNodeRestoreWait
}

func (n *NodeRestorer) next(ctx context.Context) (transport.BlobFrame, error) {
	if n.carry != nil {
		fr := *n.carry
		n.carry = nil
		return fr, nil
	}
	timer := time.NewTimer(n.wait())
	defer timer.Stop()
	select {
	case fr := <-n.Feed.Frames():
		return fr, nil
	case <-ctx.Done():
		return transport.BlobFrame{}, ctx.Err()
	case <-timer.C:
		return transport.BlobFrame{}, errors.New("E714 interrupted: no frame from the coordinator")
	}
}

func normDigest(d string) string { return strings.ToLower(strings.TrimPrefix(d, "sha256:")) }

func (n *NodeRestorer) answer(b protocol.CacheBlob, off int64) error {
	return n.Feed.Offset(protocol.Offset{Stream: StreamCache, SHA256: b.SHA256, Offset: off})
}

// receive makes one announced blob a verified copy; false leaves nothing behind.
func (n *NodeRestorer) receive(ctx context.Context, b protocol.CacheBlob) bool {
	f, err := n.Copies.Open(ctx, n.Realm, b.SHA256, b.Size, func(ctx context.Context, w io.Writer) error {
		if err := n.answer(b, 0); err != nil { // nothing held (or a copy that failed its re-hash): send it all
			return err
		}
		return n.stream(ctx, w, b)
	})
	if err != nil {
		if ctx.Err() == nil {
			_ = n.answer(b, 0) // tells a sender that already finished to send the blob again
		}
		return false
	}
	_ = f.Close()
	return n.answer(b, b.Size) == nil
}

// stream writes the chunks of b to w until the final one.
func (n *NodeRestorer) stream(ctx context.Context, w io.Writer, b protocol.CacheBlob) error {
	var next int64
	for {
		fr, err := n.next(ctx)
		if err != nil {
			return err
		}
		if fr.Digest != nil {
			if normDigest(fr.Digest.SHA256) == b.SHA256 {
				n.carry = &fr
				return errRestart
			}
			continue
		}
		c := fr.Chunk
		if c == nil || c.Stream != StreamCache || normDigest(c.Digest) != b.SHA256 {
			continue
		}
		if c.Offset != next {
			return errRestart
		}
		data, err := base64.StdEncoding.Strict().DecodeString(c.Data)
		if err != nil {
			return errRestart
		}
		if _, err = w.Write(data); err != nil {
			return err
		}
		next += int64(len(data))
		if c.Final {
			return nil
		}
	}
}
