package checkout

// Purpose: the agent half, part 1: collect the checkout chunk stream of each
// lease into a private staging file and verify it as it arrives.
// Inputs:  Expect (the lease frame), Feed (each chunk frame, with the lease id
// and epoch of its envelope), Stage (the Before hook).
// Outputs: a verified bundle file under the receiver's staging base.
// Constraints: a chunk for a lease nobody announced, for another epoch, out of
// order (offset != bytes so far), not base64, or beyond the announced size
// fails the transfer closed; the sha256 must equal the lease's bundle_digest
// before Stage lets any git process read the file; the staging root of a lease
// is created and removed through internal/hygiene; there is no resume (SSH
// session loss is a lost attempt, X11).

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// MaxBundle is the largest checkout the agent accepts (2 GiB).
const MaxBundle int64 = 2 << 30

// Stream is the chunk stream name of the checkout.
const Stream = "checkout"

// MaxOpen bounds the transfers one agent process holds at once. A lease the session later
// refuses (no free slot, stale epoch) never reaches Stage, so its transfer is announced before
// it is admitted; the bound and Deadline keep those from piling up.
const MaxOpen = 16

// Deadline is how long a transfer may stay unclaimed: receipt of the whole bundle plus the
// start of Stage. A variable so tests can shorten it.
var Deadline = 15 * time.Minute

// Errors callers can match with errors.Is.
var (
	ErrDigest     = errors.New("checkout: bundle digest does not match the lease")
	ErrUnexpected = errors.New("checkout: chunk for a lease that is not expecting one")
	ErrOrder      = errors.New("checkout: chunk out of order")
	ErrTooLarge   = errors.New("checkout: bundle larger than announced")
	ErrIncomplete = errors.New("checkout: bundle ended short of the announced size")
	ErrDeadline   = errors.New("checkout: the bundle was not received and claimed in time")
	ErrBusy       = errors.New("checkout: too many transfers are open")
)

type key struct {
	lease string
	epoch uint64
}

type transfer struct {
	lease    protocol.Lease
	name     string
	file     *os.File
	path     string
	hash     hash.Hash
	got      int64
	finished bool
	staging  bool // Stage took the transfer; its own cleanup owns it from here
	err      error
	done     chan struct{}
	release  func() // drops the directory's owner lock
	timer    *time.Timer
}

func (t *transfer) fail(err error) {
	if t.err == nil {
		t.err = err
	}
	if t.file != nil {
		_ = t.file.Close()
		t.file = nil
	}
	select {
	case <-t.done:
	default:
		close(t.done)
	}
}

// Receiver collects the checkout streams of the leases of one agent process.
type Receiver struct {
	host *hygiene.Host
	run  Runner
	mu   sync.Mutex
	open map[key]*transfer
}

// NewReceiver stages under base (created 0700 on first use). run nil means DefaultRunner.
func NewReceiver(base string, run Runner) *Receiver {
	if run == nil {
		run = DefaultRunner
	}
	return &Receiver{host: &hygiene.Host{Base: base}, run: run, open: map[key]*transfer{}}
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// stageName is the hygiene attempt name of a lease: file-name safe and unique per epoch.
func stageName(l protocol.Lease) string {
	n := unsafeName.ReplaceAllString(l.LeaseID, "_")
	if len(n) > 80 {
		n = n[:80]
	}
	return "co-" + n + "-" + strconv.FormatUint(l.Epoch, 10)
}

// Expect announces a lease with a checkout. A lease without one (Size 0) is ignored.
func (r *Receiver) Expect(l protocol.Lease) error {
	if l.Checkout.Size <= 0 {
		return nil
	}
	if l.Checkout.Size > MaxBundle {
		return fmt.Errorf("checkout: bundle of %d bytes exceeds the %d byte limit", l.Checkout.Size, MaxBundle)
	}
	if !ValidRevision(l.Checkout.Revision) {
		return fmt.Errorf("checkout: lease revision is not a full commit id")
	}
	if _, err := digestHex(l.Checkout.BundleDigest); err != nil {
		return err
	}
	r.mu.Lock()
	busy := len(r.open) >= MaxOpen
	r.mu.Unlock()
	if busy {
		return ErrBusy
	}
	name := stageName(l)
	dir, err := r.host.JobRoot(name)
	if err != nil {
		return fmt.Errorf("checkout: staging root: %w", err)
	}
	release, err := Hold(dir)
	if err != nil {
		_ = hygiene.RemoveConfined(r.host.Base, name, nil)
		return fmt.Errorf("checkout: staging root: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "checkout.bundle"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		release()
		_ = hygiene.RemoveConfined(r.host.Base, name, nil)
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key{l.LeaseID, l.Epoch}
	if _, ok := r.open[k]; ok {
		_ = f.Close()
		release()
		return fmt.Errorf("checkout: lease %s epoch %d announced twice", l.LeaseID, l.Epoch)
	}
	t := &transfer{lease: l, name: name, file: f, path: f.Name(), hash: sha256.New(), done: make(chan struct{}), release: release}
	t.timer = time.AfterFunc(Deadline, func() { r.expire(k, t) })
	r.open[k] = t
	return nil
}

// expire drops a transfer nobody completed or claimed in time: the lease it was announced
// for was refused, or the coordinator stopped sending. A whole bundle that Stage is already
// unpacking is left to Stage; an unfinished one fails, which wakes Stage if it is waiting.
func (r *Receiver) expire(k key, t *transfer) {
	r.mu.Lock()
	if r.open[k] != t {
		r.mu.Unlock()
		return
	}
	if !t.finished {
		t.fail(ErrDeadline)
	}
	claimed := t.staging
	r.mu.Unlock()
	if !claimed {
		r.Forget(k.lease, k.epoch)
	}
}

// digestHex returns the lower-case hex of "sha256:<hex>" or bare hex.
func digestHex(s string) (string, error) {
	h := strings.ToLower(strings.TrimPrefix(s, "sha256:"))
	if len(h) != 64 {
		return "", fmt.Errorf("checkout: bundle_digest is not a sha256")
	}
	if _, err := hex.DecodeString(h); err != nil {
		return "", fmt.Errorf("checkout: bundle_digest is not a sha256")
	}
	return h, nil
}

// Feed appends one chunk frame. The transfer fails closed on any violation; the
// error is also returned to the caller for logging.
func (r *Receiver) Feed(leaseID string, epoch uint64, c protocol.Chunk) error {
	r.mu.Lock()
	t := r.open[key{leaseID, epoch}]
	r.mu.Unlock()
	if t == nil || c.Stream != Stream {
		return ErrUnexpected
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.finished || t.err != nil {
		return ErrOrder
	}
	if c.Offset != t.got {
		t.fail(ErrOrder)
		return ErrOrder
	}
	data, err := base64.StdEncoding.Strict().DecodeString(c.Data)
	if err != nil {
		t.fail(fmt.Errorf("checkout: chunk is not base64"))
		return t.err
	}
	if t.got+int64(len(data)) > t.lease.Checkout.Size {
		t.fail(ErrTooLarge)
		return ErrTooLarge
	}
	if _, err := t.file.Write(data); err != nil {
		t.fail(err)
		return err
	}
	t.hash.Write(data)
	t.got += int64(len(data))
	if !c.Final {
		return nil
	}
	t.finished = true
	if t.got != t.lease.Checkout.Size {
		t.fail(ErrIncomplete)
		return ErrIncomplete
	}
	want, _ := digestHex(t.lease.Checkout.BundleDigest)
	got := hex.EncodeToString(t.hash.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		t.fail(ErrDigest)
		return ErrDigest
	}
	if err := t.file.Close(); err != nil {
		t.fail(err)
		return err
	}
	t.file = nil
	close(t.done)
	return nil
}

// Forget drops the transfer of a lease and removes its staging root.
func (r *Receiver) Forget(leaseID string, epoch uint64) {
	r.mu.Lock()
	t := r.open[key{leaseID, epoch}]
	delete(r.open, key{leaseID, epoch})
	if t != nil {
		t.fail(context.Canceled)
		t.timer.Stop()
	}
	r.mu.Unlock()
	if t != nil {
		_ = hygiene.RemoveConfined(r.host.Base, t.name, nil)
		t.release()
	}
}
