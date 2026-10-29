//go:build cachesec

package cachesec

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"time"

	cacheRemote "github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// wire joins the coordinator's Restore to a node's NodeRestorer in memory; mangle changes every chunk
// on its way to the node (the bytes the node's read-through copy receives).
type wire struct {
	frames chan transport.BlobFrame
	offs   chan protocol.Offset
	mangle func(*protocol.Chunk)
}

func (w *wire) send(f transport.BlobFrame) error {
	select {
	case w.frames <- f:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("wire: node not reading")
	}
}

func (w *wire) Digest(d protocol.Digest) error { return w.send(transport.BlobFrame{Digest: &d}) }
func (w *wire) Chunk(c protocol.Chunk) error {
	w.mangle(&c)
	return w.send(transport.BlobFrame{Chunk: &c})
}
func (w *wire) Offsets() <-chan protocol.Offset { return w.offs }

type wireFeed struct{ w *wire }

func (f wireFeed) Frames() <-chan transport.BlobFrame { return f.w.frames }
func (f wireFeed) Offset(o protocol.Offset) error     { f.w.offs <- o; return nil }
func (f wireFeed) Close()                             {}

// readThrough restores one planned blob from the coordinator into a node's read-through copies while
// every chunk is corrupted or cut short in transit; the node's re-hash must refuse each copy.
func (r *rig) readThrough(id string) error {
	want := []byte("intact payload")
	ns := namespace(r.t, owner())
	sha, size, err := r.blobs.Put(r.ctx, cacheRealm(ns.Canonical()), bytes.NewReader(want))
	must(r.t, err)
	b := protocol.CacheBlob{Path: "data", SHA256: sha, Size: size}
	l, realm := r.bindRemote(false, b)
	dir := r.t.TempDir()
	copies, err := cacheRemote.NewReadThrough(dir, 4<<20)
	must(r.t, err)
	w := &wire{frames: make(chan transport.BlobFrame, 8), offs: make(chan protocol.Offset, 64), mangle: func(c *protocol.Chunk) {
		c.Data = mangled(r, c.Data, id == "partial-readthrough")
	}}
	ctx, cancel := context.WithTimeout(r.ctx, 60*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- l.Restore(ctx, w) }()
	n := &cacheRemote.NodeRestorer{Copies: copies, Feed: wireFeed{w}, Realm: realm, Wait: 5 * time.Second}
	held := n.Fetch(ctx, []protocol.CacheBlob{b})
	err = <-done
	if held[sha] {
		r.t.Error("invalid read-through copy was held")
	}
	if f, e := copies.Open(ctx, realm, sha, size, nil); e == nil {
		f.Close()
		r.t.Error("invalid read-through copy was served")
	}
	if len(tree(r.t, dir)) != 0 {
		r.t.Error("read-through published invalid bytes")
	}
	return err
}

// mangled flips the first byte of a chunk's data, or keeps only its first half.
func mangled(r *rig, data string, partial bool) string {
	b, err := base64.StdEncoding.DecodeString(data)
	must(r.t, err)
	if partial {
		b = b[:len(b)/2]
	} else if len(b) > 0 {
		b[0] ^= 1
	}
	return base64.StdEncoding.EncodeToString(b)
}
