package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// activeLease is one running lease, owned by the Run goroutine except for the
// fields marked safe.
type activeLease struct {
	lease     protocol.Lease
	cancelCtx context.CancelFunc
	cancelled atomic.Bool // safe
	done      atomic.Bool // safe: set when the job goroutine is finished
	expires   time.Time   // the lease's expiry on the session clock; the secrets wait ends here
	tailMu    sync.Mutex
	tail      []byte // the last bytes of masked output lines
	secSeen   map[string]bool
	maskMu    sync.Mutex
	mask      [][]byte
	secMu     sync.Mutex
	secVals   map[string]string // declared secret values received so far
	secBytes  int
	secWake   chan struct{}
	lines     atomic.Uint64 // safe
	lw        *lineWriter
}

func (l *activeLease) cancel() { l.cancelled.Store(true); l.cancelCtx() }

func (l *activeLease) state() string {
	if l.cancelled.Load() {
		return "cancelling"
	}
	return "running"
}

func (s *Session) onLease(m protocol.Message, l protocol.Lease) {
	if cur, ok := s.active[l.LeaseID]; ok && cur.done.Load() {
		delete(s.active, l.LeaseID) // finished; its notice is still queued
	} else if ok {
		switch {
		case l.Epoch < cur.lease.Epoch:
			s.refuse(l, "stale_epoch", "")
		case l.Epoch > cur.lease.Epoch:
			// A newer epoch for a running lease id: the old run is stale. Stop it
			// and ask for the new lease again once the slot is free.
			cur.cancel()
			s.refuse(l, "lease_active", "")
		}
		return // equal epoch: a duplicate delivery of a lease already running
	}
	if prev, ok := s.seenEpoch[l.LeaseID]; ok && l.Epoch <= prev {
		s.refuse(l, "stale_epoch", "")
		return
	}
	if s.draining {
		s.refuse(l, "draining", "")
		return
	}
	now := s.cfg.Clock.Now()
	exp, perr := time.Parse(time.RFC3339, l.ExpiresAt)
	if perr != nil {
		s.refuse(l, "invalid_expiry", "")
		return
	}
	if !now.Before(exp) {
		s.refuse(l, "lease_expired", "")
		return
	}
	slot, err := s.cfg.Slots.Acquire(slots.Self(s.cfg.Coordinator.ID, l.LeaseID))
	if err != nil {
		var se *slots.Error
		if errors.As(err, &se) {
			s.refuse(l, se.Reason, slots.CodeFull)
		} else {
			s.refuse(l, "slot_table_error", "")
		}
		return
	}
	s.noteEpoch(l)
	// expires_at gates admission only. The coordinator renews a running lease by
	// heartbeat (Epic D6) and protocol v1 carries no new expiry, so the lease
	// frame's value is not a job deadline; a job ends by its own timeout, a
	// cancel, a drain deadline or the agent fence (P7-NODE-09).
	ctx, cancel := context.WithCancel(s.runCtx)
	al := &activeLease{lease: l, cancelCtx: cancel, secWake: make(chan struct{}, 1), secVals: map[string]string{}, expires: exp, secSeen: map[string]bool{}}
	if l.RunnerCredential != nil {
		al.addMask(l.RunnerCredential.Value)
	}
	al.addMask(l.Token)
	s.active[l.LeaseID] = al
	s.wg.Add(1)
	go s.runLease(ctx, al, slot)
}

// maxSeen bounds the epoch memory of a long-lived session.
const maxSeen = 2048

// noteEpoch remembers the highest epoch per lease id for stale-epoch refusal,
// forgetting the oldest finished ids beyond maxSeen.
func (s *Session) noteEpoch(l protocol.Lease) {
	if _, ok := s.seenEpoch[l.LeaseID]; !ok {
		s.seenOrder = append(s.seenOrder, l.LeaseID)
	}
	s.seenEpoch[l.LeaseID] = l.Epoch
	for tries := len(s.seenOrder); len(s.seenOrder) > maxSeen && tries > 0; tries-- {
		old := s.seenOrder[0]
		s.seenOrder = s.seenOrder[1:]
		if _, running := s.active[old]; !running {
			delete(s.seenEpoch, old)
		} else {
			s.seenOrder = append(s.seenOrder, old)
		}
	}
}

// refuse tells the coordinator a lease was not started: one stderr log line and
// a heartbeat naming the lease state "refused".
func (s *Session) refuse(l protocol.Lease, reason, code string) {
	text := "refused: " + reason
	if code != "" {
		text = code + ": " + strings.ReplaceAll(reason, "_", " ")
	}
	probe := &activeLease{lease: l}
	_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: 1, Stream: "stderr", Text: text}, probe)
	s.beat([]protocol.LeaseState{{LeaseID: l.LeaseID, Epoch: l.Epoch, State: "refused"}})
}

const (
	maxSecretValue = 64 << 10
	maxSecretBytes = 256 << 10
)

// onSecrets accepts the values of the refs the lease declared, once each, from
// a frame fenced to this exact lease and epoch; anything else is dropped, so a
// delayed frame for an older lease or a flood of frames cannot grow the lease.
func (s *Session) onSecrets(m protocol.Message, sec protocol.Secrets) {
	l, ok := s.active[sec.LeaseID]
	if !ok || m.Epoch == nil || *m.Epoch != l.lease.Epoch || m.LeaseID != sec.LeaseID {
		return
	}
	declared := map[string]bool{}
	for _, r := range l.lease.Secrets {
		declared[r.Ref] = true
	}
	// Decide what is taken, install the masks, and only then publish the values:
	// the job must never be able to print a value its masks do not yet cover.
	l.secMu.Lock()
	var refs, vals []string
	for _, v := range sec.Values {
		if l.secSeen[v.Ref] || !declared[v.Ref] || len(v.Value) > maxSecretValue || l.secBytes+len(v.Value) > maxSecretBytes {
			continue
		}
		l.secSeen[v.Ref] = true
		l.secBytes += len(v.Value)
		refs, vals = append(refs, v.Ref), append(vals, v.Value)
	}
	l.secMu.Unlock()
	for _, v := range vals {
		l.addMask(v)
	}
	l.secMu.Lock()
	for i, r := range refs {
		l.secVals[r] = vals[i]
	}
	l.secMu.Unlock()
	select {
	case l.secWake <- struct{}{}:
	default:
	}
}

const tailKeep = 4 << 10

// noteTail keeps the last tailKeep bytes of masked output.
func (l *activeLease) noteTail(text string) {
	l.tailMu.Lock()
	defer l.tailMu.Unlock()
	l.tail = append(l.tail, text...)
	l.tail = append(l.tail, '\n')
	if len(l.tail) > tailKeep {
		l.tail = l.tail[len(l.tail)-tailKeep:]
	}
}

// maskedTail is JobRun.Tail: flush the unterminated line, return the masked tail.
func (l *activeLease) maskedTail() string {
	l.lw.Flush()
	l.tailMu.Lock()
	defer l.tailMu.Unlock()
	return strings.ToValidUTF8(string(l.tail), "")
}

func (l *activeLease) addMask(v string) {
	l.maskMu.Lock()
	defer l.maskMu.Unlock()
	for _, m := range secretMasks(v) {
		l.mask = append(l.mask, []byte(m))
	}
}

// redact hides every literal secret the lease carries from a string.
func (l *activeLease) redact(s string) string {
	return string(maskAll([]byte(s), l.masks()))
}

func (l *activeLease) masks() [][]byte {
	l.maskMu.Lock()
	defer l.maskMu.Unlock()
	return append([][]byte(nil), l.mask...)
}

func (s *Session) runLease(ctx context.Context, al *activeLease, slot *slots.Slot) {
	defer s.wg.Done()
	defer func() {
		al.done.Store(true)
		select {
		case s.finished <- al:
		case <-s.closing:
		}
	}()
	defer func() { _ = s.cfg.Slots.Release(slot) }()
	defer al.cancelCtx()
	l := al.lease
	al.lw = newLineWriter(func(text string) {
		al.noteTail(text)
		_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stdout", Text: text}, al)
	}, al.masks)
	root, err := newJobRoot(s.cfg.StateDir, l)
	if err != nil {
		s.finish(ctx, al, root, JobResult{Outcome: Outcome{State: "failed", FailureClass: "infra"}}, fmt.Errorf("job root: %w", err))
		return
	}
	defer func() { _ = os.RemoveAll(root.Dir) }()
	var cred []byte
	if l.RunnerCredential != nil {
		cred = []byte(l.RunnerCredential.Value)
		defer zero(cred)
	}

	var setup JobSetup
	skip := ""
	for _, h := range s.jobHooks {
		if h.Before == nil {
			continue
		}
		got, err := h.Before(ctx, l, root)
		if err != nil && ctx.Err() != nil {
			skip = "cancelled" // the hook stopped because the lease was cancelled
			break
		}
		if err != nil {
			s.finish(ctx, al, root, JobResult{Outcome: Outcome{State: "failed", FailureClass: "infra"}}, fmt.Errorf("job hook %s: %w", h.Name, err))
			return
		}
		mergeSetup(&setup, got)
	}
	waitCtx, stopWait := context.WithTimeout(ctx, max(al.expires.Sub(s.cfg.Clock.Now()), 0))
	for waitCtx.Err() == nil {
		al.secMu.Lock()
		for k, v := range al.secVals {
			if setup.Secrets == nil {
				setup.Secrets = map[string]string{}
			}
			setup.Secrets[k] = v
		}
		for name, n := range shortNames(al.secVals) {
			if n == 1 {
				setup.Secrets[name] = shortValue(al.secVals, name)
			}
		}
		al.secMu.Unlock()
		if !missingSecrets(l.Job.Secrets, setup.Secrets) {
			break
		}
		select { // bounded by the lease's expiry
		case <-al.secWake:
		case <-waitCtx.Done():
		}
	}
	stopWait()
	switch {
	case skip != "":
	case ctx.Err() != nil:
		skip = "cancelled"
	case missingSecrets(l.Job.Secrets, setup.Secrets):
		skip = "secrets_missing"
	}
	res, err := s.cfg.Runner.Run(ctx, JobRun{Lease: l, Slot: slot.Index, Root: root, Setup: setup, Credential: cred, Output: al.lw.Write, Tail: al.maskedTail, Skip: skip})
	al.lw.Flush()
	s.finish(ctx, al, root, res, err)
}

// newJobRoot makes a private, uniquely named directory for one lease run: two
// coordinators may use the same lease id, and ids are not file-name safe.
func newJobRoot(state string, l protocol.Lease) (JobRoot, error) {
	base := filepath.Join(state, "jobs")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return JobRoot{}, err
	}
	dir, err := os.MkdirTemp(base, unsafeName.ReplaceAllString(l.LeaseID, "_")+"-"+strconv.FormatUint(l.Epoch, 10)+"-")
	return JobRoot{Dir: dir}, err
}

// shortNames counts how many delivered refs share each short name ("TOKEN" in
// "project/TOKEN"); a name two refs share is not aliased.
func shortNames(vals map[string]string) map[string]int {
	n := map[string]int{}
	for ref := range vals {
		if _, name, ok := strings.Cut(ref, "/"); ok {
			n[name]++
		}
	}
	return n
}

func shortValue(vals map[string]string, name string) string {
	for ref, v := range vals {
		if _, short, ok := strings.Cut(ref, "/"); ok && short == name {
			return v
		}
	}
	return ""
}

func missingSecrets(need []string, have map[string]string) bool {
	for _, n := range need {
		if _, ok := have[n]; !ok {
			return true
		}
	}
	return false
}

// finish runs the After hooks, then signs and sends the result.
func (s *Session) finish(ctx context.Context, al *activeLease, root JobRoot, res JobResult, runErr error) {
	l := al.lease
	if runErr != nil {
		al.lw.Flush()
		_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stderr", Text: oneLine(al.redact("agent: " + runErr.Error()))}, al)
	}
	hookCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	up := &uploader{s: s, al: al}
	for _, h := range s.jobHooks {
		if h.After == nil {
			continue
		}
		if err := h.After(hookCtx, l, root, res.Outcome, up); err != nil {
			_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stderr", Text: oneLine(al.redact("agent: job hook " + h.Name + ": " + err.Error()))}, al)
		}
	}
	ev, cerr := canonicalJSON(res.Evidence)
	if cerr != nil {
		// The runner failed before it had a document. This one cannot validate as
		// ci.evidence/v1, so the coordinator records the lease as errored.
		ev = json.RawMessage(`{"schema":"ci.evidence/v1","result":"error","reason":"agent_runner_failed"}`)
	}
	sig, err := signEvidence(currentSigner(s.cfg.Signer), ev)
	if err != nil {
		_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stderr", Text: oneLine(al.redact("agent: result not signed: " + err.Error()))}, al)
		sig = protocol.Signature{}
	}
	err = s.send("result", protocol.Result{LeaseID: l.LeaseID, Epoch: l.Epoch, Evidence: ev, Signature: sig}, al)
	var pe *protocol.Error
	if errors.As(err, &pe) && pe.Reason == "frame_too_large" {
		// A document the frame cannot hold must not vanish: say so, unsigned.
		small := json.RawMessage(`{"schema":"ci.evidence/v1","result":"error","reason":"evidence_too_large"}`)
		_ = s.send("result", protocol.Result{LeaseID: l.LeaseID, Epoch: l.Epoch, Evidence: small}, al)
	}
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// uploader sends transfer frames for one lease.
type uploader struct {
	s  *Session
	al *activeLease
}

func (u *uploader) Digest(b protocol.Digest) error { return u.s.send("digest", b, u.al) }
func (u *uploader) Offset(b protocol.Offset) error { return u.s.send("offset", b, u.al) }
func (u *uploader) Chunk(b protocol.Chunk) error   { return u.s.send("chunk", b, u.al) }
func (u *uploader) CacheRef(b protocol.CacheRef) error {
	b.LeaseID = u.al.lease.LeaseID
	return u.s.send("cache-ref", b, u.al)
}
func (u *uploader) ArtifactRef(b protocol.ArtifactRef) error {
	b.LeaseID = u.al.lease.LeaseID
	return u.s.send("artifact-ref", b, u.al)
}
