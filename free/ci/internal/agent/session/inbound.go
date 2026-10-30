package session

// Purpose: route the coordinator's digest and chunk frames to the job hook that
// restores a cache from them (P7-CACHE-21, Epic CACHE D7): the reverse of an
// upload. The hook finds the lease's feed on its context (FeedFrom).
// Inputs:  `digest` and `chunk` frames fenced to a running lease and its epoch.
// Outputs: a bounded channel per lease, read through transport.BlobFeed.
// Constraints: only a lease whose plan restores blobs, on a carrier that sends
// coordinator frames (RestoreCarrier), gets a feed; any other frame is dropped.
// The Run goroutine blocks on a full channel only while the hook still reads it
// (backpressure on the carrier, ended by Close, the lease context or shutdown);
// a frame for an unknown lease or another epoch is ignored.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// inboundBuffer is how many unread frames a lease keeps (32 chunks is under 24 MiB of base64).
const inboundBuffer = 32

// RestoreCarrier is implemented by a Carrier that delivers coordinator digest and
// chunk frames to the session (carrier A). A carrier without it gets no feed.
type RestoreCarrier interface{ BlobRestore() bool }

// BlobRestore says the stream carrier delivers every coordinator frame.
func (*streamCarrier) BlobRestore() bool { return true }

type feedKey struct{}

// FeedFrom returns the reverse blob feed of the lease a Before hook runs for.
func FeedFrom(ctx context.Context) (transport.BlobFeed, bool) {
	f, ok := ctx.Value(feedKey{}).(*feed)
	if !ok {
		return nil, false
	}
	return f, true
}

type feed struct {
	s      *Session
	al     *activeLease
	frames chan transport.BlobFrame
	closed chan struct{}
	once   sync.Once
}

func (s *Session) newFeed(al *activeLease) *feed {
	rc, ok := s.cfg.Carrier.(RestoreCarrier)
	if !ok || !rc.BlobRestore() || al.lease.Cache == nil || len(al.lease.Cache.Restore) == 0 {
		return nil
	}
	return &feed{s: s, al: al, frames: make(chan transport.BlobFrame, inboundBuffer), closed: make(chan struct{})}
}

func (f *feed) Frames() <-chan transport.BlobFrame { return f.frames }
func (f *feed) Offset(o protocol.Offset) error     { return f.s.send("offset", o, f.al) }
func (f *feed) Close()                             { f.once.Do(func() { close(f.closed) }) }

// onInbound hands a digest or chunk frame to the lease's feed.
func (s *Session) onInbound(m protocol.Message, fr transport.BlobFrame) {
	l, ok := s.active[m.LeaseID]
	if !ok || m.Epoch == nil || *m.Epoch != l.lease.Epoch || l.feed == nil {
		return
	}
	select {
	case <-l.feed.closed:
		return
	default:
	}
	select {
	case l.feed.frames <- fr:
	case <-l.feed.closed:
	case <-l.ctxDone:
	case <-s.closing:
	}
}
