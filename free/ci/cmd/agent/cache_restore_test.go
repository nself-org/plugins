package main

// Purpose: a node with no copy of a planned cache blob fetches it from the coordinator over the
// lease's transfer channel before the job; with a silent coordinator the spec is a cold miss that
// binds nothing, and the job is never failed by the restore.

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// coordinatorUp is the lease's Uploader plus a tiny coordinator behind it: a request for a planned
// digest is announced, the node's answer is followed by the whole blob in one chunk.
type coordinatorUp struct {
	mu     sync.Mutex
	blobs  map[string][]byte
	in     chan protocol.Message
	asked  map[string]bool
	chunks int
	silent bool
}

func (c *coordinatorUp) Offset(o protocol.Offset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.blobs[o.SHA256]
	if c.silent || !ok || o.Offset == int64(len(data)) {
		return nil
	}
	if !c.asked[o.SHA256] {
		c.asked[o.SHA256] = true
		c.in <- protocol.Message{Type: "digest", Body: &protocol.Digest{Stream: "cache", SHA256: o.SHA256, Size: int64(len(data))}}
		return nil
	}
	c.chunks++
	c.in <- protocol.Message{Type: "chunk", Body: &protocol.Chunk{Stream: "cache", Offset: 0, Data: base64.StdEncoding.EncodeToString(data), Digest: o.SHA256, Final: true}}
	return nil
}
func (c *coordinatorUp) Digest(protocol.Digest) error           { return nil }
func (c *coordinatorUp) Chunk(protocol.Chunk) error             { return nil }
func (c *coordinatorUp) ArtifactRef(protocol.ArtifactRef) error { return nil }
func (c *coordinatorUp) CacheRef(protocol.CacheRef) error       { return nil }

func tarOf(name, body string) []byte {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	_ = tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0644})
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	return b.Bytes()
}

func TestCacheHookFetchesMissingBlobsFromTheCoordinator(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	l := cacheHookLease("ro", "container")
	co := &coordinatorUp{blobs: map[string][]byte{}, in: make(chan protocol.Message, 16), asked: map[string]bool{}}
	var blobs []protocol.CacheBlob
	for path, body := range map[string]string{"mod": "module bytes", "build": "build bytes"} {
		data := tarOf("payload", body)
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])
		co.blobs[sha] = data
		blobs = append(blobs, protocol.CacheBlob{Path: path, SHA256: sha, Size: int64(len(data))})
	}
	l.Cache.Restore = []protocol.CacheRestore{{Spec: "go", Key: strings.Repeat("a", 64), Dir: "go", Blobs: blobs}}
	root := session.JobRoot{Dir: t.TempDir()}
	setup, err := cacheBeforeTransfer(context.Background(), l, root, session.Transfer{Up: co, In: co.in})
	if err != nil {
		t.Fatal(err)
	}
	defer cacheAfter(context.Background(), l, root, session.Outcome{State: "cancelled"}, nil)
	if len(setup.Mounts) != 2 || setup.Env["GOMODCACHE"] == "" {
		t.Fatalf("the restored cache was not bound: %+v", setup)
	}
	if co.chunks != 2 {
		t.Fatalf("expected one transfer per blob, got %d", co.chunks)
	}
	// the second lease on the same node finds both copies and asks for nothing
	co2 := &coordinatorUp{blobs: co.blobs, in: make(chan protocol.Message, 16), asked: map[string]bool{}}
	setup, err = cacheBeforeTransfer(context.Background(), l, session.JobRoot{Dir: t.TempDir()}, session.Transfer{Up: co2, In: co2.in})
	if err != nil || len(setup.Mounts) != 2 || len(co2.asked) != 0 {
		t.Fatalf("second lease: mounts=%d asked=%d err=%v", len(setup.Mounts), len(co2.asked), err)
	}
	_ = cacheAfter(context.Background(), l, session.JobRoot{}, session.Outcome{State: "cancelled"}, nil)
}

func TestCacheHookSilentCoordinatorIsAColdMiss(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_HOME", t.TempDir())
	l := cacheHookLease("ro", "container")
	data := tarOf("payload", "x")
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	l.Cache.Restore = []protocol.CacheRestore{{Spec: "go", Key: strings.Repeat("a", 64), Dir: "go", Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: sha, Size: int64(len(data))}}}}
	co := &coordinatorUp{blobs: map[string][]byte{sha: data}, in: make(chan protocol.Message, 4), asked: map[string]bool{}, silent: true}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond) // the lease context ends the wait
	defer cancel()
	setup, err := cacheBeforeTransfer(ctx, l, session.JobRoot{Dir: t.TempDir()}, session.Transfer{Up: co, In: co.in})
	if err != nil {
		t.Fatalf("a silent coordinator failed the lease: %v", err)
	}
	if len(setup.Mounts) != 0 || len(setup.Env) != 0 {
		t.Fatalf("a cold miss bound a cache: %+v", setup)
	}
}
