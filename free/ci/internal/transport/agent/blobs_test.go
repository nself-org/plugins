package agent

// Purpose: unit tests of the blob upload protocol (CACHE delta 5) without a listener: every refusal, the limits at
//          their exact boundaries, announce-again after a reconnect, resume with and without a verifying sink,
//          digest checks, and the lease failing when a blob is rejected.
// Inputs:  a driver with one registered route and scripted blob writers. Constraints: no network, no sleeping.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// plainBlob is a writer with no Abort and no Verify.
type plainBlob struct {
	data      []byte
	held      int64 // what Offset reports at the start
	closeErr  error
	writeErr  error
	committed bool
}

func (b *plainBlob) Write(p []byte) (int, error) {
	if b.writeErr != nil {
		return 0, b.writeErr
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func (b *plainBlob) Offset() int64 { return b.held + int64(len(b.data)) }
func (b *plainBlob) Close() error  { b.committed = b.closeErr == nil; return b.closeErr }

// fullBlob adds Abort and Verify.
type fullBlob struct {
	plainBlob
	aborted   int
	verifyErr error
	verified  []string
}

func (b *fullBlob) Abort() error { b.aborted++; return nil }
func (b *fullBlob) Verify(want string) error {
	b.verified = append(b.verified, want)
	return b.verifyErr
}

// abortOnly adds Abort but no Verify.
type abortOnly struct {
	plainBlob
	aborted int
}

func (b *abortOnly) Abort() error { b.aborted++; return nil }

type blobSinkFake struct {
	runSink
	next   transport.BlobWriter
	err    error
	heads  []transport.BlobHeader
	called int
}

func (s *blobSinkFake) Blob(_ context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	s.called++
	s.heads = append(s.heads, h)
	return s.next, s.err
}

type blobRig struct {
	d    *Driver
	rt   *route
	sink *blobSinkFake
	h    blobHandler
}

func newBlobRig() *blobRig {
	r := &blobRig{sink: &blobSinkFake{}}
	r.d = &Driver{routes: map[string]*route{}, finished: map[string]bool{}}
	r.rt = &route{node: "n-1", leaseID: "l1", sink: r.sink, done: make(chan struct{})}
	r.d.routes["l1"] = r.rt
	r.h = blobHandler{r.d}
	return r
}

func msgOf(lease string, body any) protocol.Message {
	ep := uint64(3)
	typ := "digest"
	if _, ok := body.(protocol.Chunk); ok {
		typ = "chunk"
	}
	return protocol.Message{Type: typ, LeaseID: lease, Epoch: &ep, Body: body}
}

func shaOf(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func (r *blobRig) digest(stream, sha string, size int64) ([]protocol.Message, error) {
	return r.h.Frame(context.Background(), "n-1", msgOf("l1", &protocol.Digest{Stream: stream, SHA256: sha, Size: size}))
}

func (r *blobRig) chunk(stream, sha string, off int64, data []byte, final bool) error {
	_, err := r.h.Frame(context.Background(), "n-1", msgOf("l1", &protocol.Chunk{Stream: stream, Offset: off, Data: base64.StdEncoding.EncodeToString(data), Digest: sha, Final: final}))
	return err
}

func wantBlobRefusal(t *testing.T, err error) {
	t.Helper()
	var be blobError
	if !errors.As(err, &be) || !be.Refusal() || ReasonOf(err) != ReasonBlob || !strings.HasPrefix(err.Error(), "E656 "+ReasonBlob) {
		t.Fatalf("err = %v, want an E656 %s refusal", err, ReasonBlob)
	}
}

func (r *blobRig) failed() error {
	select {
	case <-r.rt.done:
		return r.rt.err
	default:
		return nil
	}
}

func TestBlobFramesOfOtherNodesUnknownAndFinishedLeases(t *testing.T) {
	r := newBlobRig()
	ctx := context.Background()
	if out, err := r.h.Frame(ctx, "n-2", msgOf("l1", &protocol.Digest{Stream: "artifact"})); out != nil || err != nil {
		t.Fatalf("another node's frame = %v %v, want it ignored", out, err)
	}
	if _, err := r.h.Frame(ctx, "n-1", msgOf("ghost", &protocol.Digest{Stream: "artifact"})); ReasonOf(err) != ReasonLeaseLost {
		t.Fatalf("unknown lease = %v", err)
	}
	r.d.finished["l9"] = true
	if out, err := r.h.Frame(ctx, "n-1", msgOf("l9", &protocol.Digest{Stream: "artifact"})); out != nil || err != nil {
		t.Fatalf("finished lease = %v %v, want it accepted and dropped", out, err)
	}
	if r.failed() != nil || r.sink.called != 0 {
		t.Fatal("an ignored frame touched the route")
	}
}

func TestBlobDigestRefusals(t *testing.T) {
	good := shaOf([]byte("x"))
	for name, c := range map[string]struct {
		stream, sha string
		size        int64
	}{
		"checkout stream": {"checkout", good, 10},
		"empty stream":    {"", good, 10},
		"short sha":       {"artifact", good[:63], 10},
		"long sha":        {"artifact", good + "0", 10},
		"not hex":         {"artifact", strings.Repeat("g", 64), 10},
		"size zero":       {"artifact", good, 0},
		"size negative":   {"artifact", good, -1},
		"size past max":   {"cache", good, maxBlob + 1},
	} {
		r := newBlobRig()
		r.sink.next = &fullBlob{}
		_, err := r.digest(c.stream, c.sha, c.size)
		wantBlobRefusal(t, err)
		if r.failed() == nil {
			t.Errorf("%s: the lease was not failed", name)
		}
		if r.sink.called != 0 {
			t.Errorf("%s: the sink was asked for a writer", name)
		}
	}
}

func TestBlobDigestAcceptsTheBoundsAndNormalisesTheForm(t *testing.T) {
	good := shaOf([]byte("x"))
	for name, c := range map[string]struct {
		sha  string
		size int64
	}{"size one": {good, 1}, "size max": {good, maxBlob}, "prefixed": {"sha256:" + good, 5}, "upper case": {strings.ToUpper(good), 5}} {
		r := newBlobRig()
		r.sink.next = &fullBlob{}
		out, err := r.digest("artifact", c.sha, c.size)
		if err != nil || len(out) != 1 || out[0].Type != "offset" {
			t.Fatalf("%s: %v %v", name, out, err)
		}
		off := out[0].Body.(protocol.Offset)
		if off.SHA256 != good || off.Offset != 0 || off.Stream != "artifact" || out[0].LeaseID != "l1" || out[0].Epoch == nil || *out[0].Epoch != 3 {
			t.Fatalf("%s: offset frame %+v", name, out[0])
		}
		if h := r.sink.heads[0]; h.SHA256 != good || h.Size != c.size || h.Stream != "artifact" {
			t.Fatalf("%s: header %+v", name, h)
		}
	}
}

func TestBlobAnnouncedAgainKeepsTheBlobAndADifferentOneReplacesIt(t *testing.T) {
	r := newBlobRig()
	w := &fullBlob{}
	r.sink.next = w
	a := shaOf([]byte("a"))
	if _, err := r.digest("artifact", a, 10); err != nil {
		t.Fatal(err)
	}
	if err := r.chunk("artifact", a, 0, []byte("abcd"), false); err != nil {
		t.Fatal(err)
	}
	out, err := r.digest("artifact", a, 10) // the agent reconnected and announces again
	if err != nil || out[0].Body.(protocol.Offset).Offset != 4 || w.aborted != 0 || r.sink.called != 1 {
		t.Fatalf("re-announce: %v %v aborted=%d sink calls=%d", out, err, w.aborted, r.sink.called)
	}
	for name, d := range map[string]protocol.Digest{"other size": {Stream: "artifact", SHA256: a, Size: 11}, "other digest": {Stream: "artifact", SHA256: shaOf([]byte("b")), Size: 10}} {
		rr := newBlobRig()
		old := &fullBlob{}
		rr.sink.next = old
		if _, err := rr.digest("artifact", a, 10); err != nil {
			t.Fatal(err)
		}
		rr.sink.next = &fullBlob{}
		if _, err := rr.digest(d.Stream, d.SHA256, d.Size); err != nil || old.aborted != 1 {
			t.Fatalf("%s: err %v, old writer aborted %d times, want 1", name, err, old.aborted)
		}
	}
}

func TestBlobBudgetsAtTheirExactLimits(t *testing.T) {
	r := newBlobRig()
	r.sink.next = &fullBlob{}
	for i := 0; i < maxLeaseBlobs; i++ { // 1024 blobs of one byte each are within the budget
		if _, err := r.digest("artifact", shaOf([]byte(fmt.Sprint(i))), 1); err != nil {
			t.Fatalf("blob %d refused: %v", i, err)
		}
	}
	_, err := r.digest("artifact", shaOf([]byte("one too many")), 1)
	wantBlobRefusal(t, err)
	if r.failed() == nil {
		t.Fatal("the lease did not fail")
	}

	r = newBlobRig()
	r.sink.next = &fullBlob{}
	for i := 0; i < maxLeaseBytes/maxBlob; i++ { // 4 blobs of 4 GiB fill the byte budget exactly
		if _, err := r.digest("artifact", shaOf([]byte{byte(i)}), maxBlob); err != nil {
			t.Fatalf("blob %d refused: %v", i, err)
		}
	}
	_, err = r.digest("artifact", shaOf([]byte("last byte")), 1)
	wantBlobRefusal(t, err)
}

func TestBlobWriterRequirements(t *testing.T) {
	sha := shaOf([]byte("x"))
	for name, c := range map[string]struct {
		w   transport.BlobWriter
		err error
	}{
		"sink refuses":       {nil, errors.New("no space")},
		"no abort":           {&plainBlob{}, nil},
		"resume, no verify":  {&abortOnly{plainBlob: plainBlob{held: 5}}, nil},
		"resume, no verify2": {&abortOnly{plainBlob: plainBlob{held: 1}}, nil},
	} {
		r := newBlobRig()
		r.sink.next, r.sink.err = c.w, c.err
		_, err := r.digest("artifact", sha, 10)
		wantBlobRefusal(t, err)
		if r.failed() == nil {
			t.Errorf("%s: lease not failed", name)
		}
	}
	// a fresh blob into a writer that aborts but cannot verify is fine
	r := newBlobRig()
	r.sink.next = &abortOnly{}
	if _, err := r.digest("artifact", sha, 10); err != nil {
		t.Fatalf("fresh blob: %v", err)
	}
	// the writer refused for lack of Verify is discarded
	r = newBlobRig()
	ao := &abortOnly{plainBlob: plainBlob{held: 3}}
	r.sink.next = ao
	_, _ = r.digest("artifact", sha, 10)
	if ao.aborted == 0 {
		t.Fatal("a writer that cannot resume was not discarded")
	}
}

func TestBlobHeldInFullIsVerifiedAndCommittedAtOnce(t *testing.T) {
	sha := shaOf([]byte("x"))
	for name, c := range map[string]struct {
		verifyErr, closeErr error
		wantErr             bool
	}{"ok": {}, "digest wrong": {verifyErr: errors.New("mismatch"), wantErr: true}, "close fails": {closeErr: errors.New("disk"), wantErr: true}} {
		r := newBlobRig()
		w := &fullBlob{plainBlob: plainBlob{held: 10, closeErr: c.closeErr}, verifyErr: c.verifyErr}
		r.sink.next = w
		out, err := r.digest("artifact", sha, 10)
		if c.wantErr {
			wantBlobRefusal(t, err)
			if c.verifyErr != nil && w.aborted != 1 {
				t.Errorf("%s: aborted %d, want the bad blob discarded", name, w.aborted)
			}
			continue
		}
		if err != nil || !w.committed || len(w.verified) != 1 || w.verified[0] != sha || out[0].Body.(protocol.Offset).Offset != 10 {
			t.Fatalf("%s: %v %v committed=%v verified=%v", name, out, err, w.committed, w.verified)
		}
		if _, active := r.rt.blobs.active["artifact"]; active {
			t.Fatal("a complete blob stayed active")
		}
	}
	// a complete blob whose writer cannot verify is committed without the check
	r := newBlobRig()
	pw := &abortOnly{plainBlob: plainBlob{held: 10}}
	r.sink.next = pw
	if _, err := r.digest("artifact", sha, 10); err == nil {
		t.Fatal("a held blob that cannot be verified must be refused")
	}
}

func fresh(t *testing.T, data []byte, w transport.BlobWriter) (*blobRig, string) {
	t.Helper()
	r := newBlobRig()
	r.sink.next = w
	sha := shaOf(data)
	if _, err := r.digest("artifact", sha, int64(len(data))); err != nil {
		t.Fatal(err)
	}
	return r, sha
}

func TestBlobChunksStreamAndCommitOnTheFinalOne(t *testing.T) {
	data := []byte("0123456789abcdef")
	w := &fullBlob{}
	r, sha := fresh(t, data, w)
	if err := r.chunk("artifact", sha, 0, data[:6], false); err != nil {
		t.Fatal(err)
	}
	if w.committed {
		t.Fatal("committed before the final chunk")
	}
	if err := r.chunk("artifact", "sha256:"+strings.ToUpper(sha), 6, data[6:], true); err != nil {
		t.Fatal(err)
	}
	if !w.committed || string(w.data) != string(data) || len(r.rt.blobs.active) != 0 || r.failed() != nil {
		t.Fatalf("committed=%v data=%q active=%d failed=%v", w.committed, w.data, len(r.rt.blobs.active), r.failed())
	}
}

func TestBlobChunkRefusals(t *testing.T) {
	data := []byte("0123456789")
	sha := shaOf(data)
	type run func(*blobRig) error
	b64 := func(n int) []byte { return []byte(strings.Repeat("z", n)) }
	for name, c := range map[string]struct {
		w      transport.BlobWriter
		action run
	}{
		"not announced": {&fullBlob{}, func(r *blobRig) error { return r.chunk("cache", sha, 0, data, true) }},
		"other digest":  {&fullBlob{}, func(r *blobRig) error { return r.chunk("artifact", shaOf([]byte("other")), 0, data, true) }},
		"bad base64": {&fullBlob{}, func(r *blobRig) error {
			_, err := r.h.Frame(context.Background(), "n-1", msgOf("l1", &protocol.Chunk{Stream: "artifact", Offset: 0, Data: "!!!", Digest: sha}))
			return err
		}},
		"larger than announced": {&fullBlob{}, func(r *blobRig) error { return r.chunk("artifact", sha, 0, b64(11), false) }},
		"sink write fails":      {&fullBlob{plainBlob: plainBlob{writeErr: errors.New("disk full")}}, func(r *blobRig) error { return r.chunk("artifact", sha, 0, data[:4], false) }},
		"final but short":       {&fullBlob{}, func(r *blobRig) error { return r.chunk("artifact", sha, 0, data[:9], true) }},
		"digest mismatch":       {&fullBlob{}, func(r *blobRig) error { return r.chunk("artifact", sha, 0, []byte("012345678X"), true) }},
		"close fails":           {&fullBlob{plainBlob: plainBlob{closeErr: errors.New("disk")}}, func(r *blobRig) error { return r.chunk("artifact", sha, 0, data, true) }},
	} {
		r, _ := fresh(t, data, c.w)
		err := c.action(r)
		wantBlobRefusal(t, err)
		if r.failed() == nil {
			t.Errorf("%s: the lease did not fail", name)
		}
		if fb, ok := c.w.(*fullBlob); ok && fb.committed {
			t.Errorf("%s: a rejected blob was committed", name)
		}
	}
}

func TestBlobChunkThatFillsTheAnnouncedSizeExactlyIsAccepted(t *testing.T) {
	data := []byte("0123456789")
	w := &fullBlob{}
	r, sha := fresh(t, data, w)
	if err := r.chunk("artifact", sha, 0, data, false); err != nil { // fills the size, final comes next
		t.Fatalf("a chunk that ends exactly at the size: %v", err)
	}
	if err := r.chunk("artifact", sha, 10, nil, true); err != nil {
		t.Fatalf("empty final chunk: %v", err)
	}
	if !w.committed {
		t.Fatal("not committed")
	}
}

func TestBlobOutOfOrderChunksAreAnsweredByASilentResyncAndGiveUpAfterThree(t *testing.T) {
	data := []byte("0123456789")
	w := &fullBlob{}
	r, sha := fresh(t, data, w)
	for i := 1; i <= maxResync; i++ {
		if err := r.chunk("artifact", sha, 5, data[5:], false); err != nil {
			t.Fatalf("out-of-order chunk %d: %v", i, err)
		}
	}
	if len(w.data) != 0 {
		t.Fatal("an out-of-order chunk was written")
	}
	err := r.chunk("artifact", sha, 5, data[5:], false)
	wantBlobRefusal(t, err)
	if w.aborted == 0 || r.failed() == nil {
		t.Fatalf("aborted=%d failed=%v", w.aborted, r.failed())
	}
}

func TestBlobResumedWithoutAHashChecksWithTheSinkOnTheFinalChunk(t *testing.T) {
	data := []byte("0123456789")
	sha := shaOf(data)
	for name, verifyErr := range map[string]error{"verifies": nil, "does not": errors.New("mismatch")} {
		w := &fullBlob{plainBlob: plainBlob{held: 4}, verifyErr: verifyErr}
		r := newBlobRig()
		r.sink.next = w
		out, err := r.digest("artifact", sha, 10)
		if err != nil || out[0].Body.(protocol.Offset).Offset != 4 {
			t.Fatalf("%s: %v %v", name, out, err)
		}
		err = r.chunk("artifact", sha, 4, data[4:], true)
		if verifyErr != nil {
			wantBlobRefusal(t, err)
			if w.committed {
				t.Fatalf("%s: committed a blob the sink rejected", name)
			}
			continue
		}
		if err != nil || !w.committed || len(w.verified) != 1 {
			t.Fatalf("%s: err %v committed %v verified %v", name, err, w.committed, w.verified)
		}
	}
}

func TestBlobFailureAbortsTheOtherBlobsOfTheLease(t *testing.T) {
	r := newBlobRig()
	cacheW, artW := &fullBlob{}, &fullBlob{}
	r.sink.next = cacheW
	cs := shaOf([]byte("c"))
	if _, err := r.digest("cache", cs, 5); err != nil {
		t.Fatal(err)
	}
	r.sink.next = artW
	as := shaOf([]byte("a"))
	if _, err := r.digest("artifact", as, 5); err != nil {
		t.Fatal(err)
	}
	wantBlobRefusal(t, r.chunk("artifact", as, 0, []byte("zzzzz"), true)) // wrong digest
	if cacheW.aborted != 1 || len(r.rt.blobs.active) != 0 {
		t.Fatalf("cache writer aborted %d times, %d blobs still active", cacheW.aborted, len(r.rt.blobs.active))
	}
}

func TestShortDigestForMessages(t *testing.T) {
	if short("abcdef") != "abcdef" || short("0123456789abc") != "0123456789ab" || short(strings.Repeat("x", 12)) != strings.Repeat("x", 12) {
		t.Fatal("short")
	}
}

func TestDriverBlobsReturnsTheHandler(t *testing.T) {
	d := &Driver{}
	if h, ok := d.Blobs().(blobHandler); !ok || h.d != d {
		t.Fatal("Blobs() is not bound to the driver")
	}
}
