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
	secrets   chan protocol.Secrets
	maskMu    sync.Mutex
	mask      [][]byte
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
	if cur, ok := s.active[l.LeaseID]; ok && l.Epoch == cur.lease.Epoch {
		return // a duplicate delivery of a lease already running
	}
	if prev, ok := s.seenEpoch[l.LeaseID]; ok && l.Epoch <= prev {
		s.refuse(l, "stale_epoch", "")
		return
	}
	if s.draining {
		s.refuse(l, "draining", "")
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
	s.seenEpoch[l.LeaseID] = l.Epoch
	ctx, cancel := context.WithCancel(s.runCtx)
	al := &activeLease{lease: l, cancelCtx: cancel, secrets: make(chan protocol.Secrets, 1)}
	if l.RunnerCredential != nil {
		al.mask = append(al.mask, []byte(l.RunnerCredential.Value))
	}
	s.active[l.LeaseID] = al
	s.wg.Add(1)
	go s.runLease(ctx, al, slot)
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

func (s *Session) onSecrets(m protocol.Message, sec protocol.Secrets) {
	l, ok := s.active[sec.LeaseID]
	if !ok || m.Epoch != nil && *m.Epoch != l.lease.Epoch {
		return
	}
	l.maskMu.Lock()
	for _, v := range sec.Values {
		l.mask = append(l.mask, []byte(v.Value))
	}
	l.maskMu.Unlock()
	select {
	case l.secrets <- sec:
	default: // the job already has its values; a second frame is ignored
	}
}

func (l *activeLease) masks() [][]byte {
	l.maskMu.Lock()
	defer l.maskMu.Unlock()
	return append([][]byte(nil), l.mask...)
}

func (s *Session) runLease(ctx context.Context, al *activeLease, slot *slots.Slot) {
	defer s.wg.Done()
	defer func() {
		select {
		case s.finished <- al.lease.LeaseID:
		case <-s.closing:
		}
	}()
	defer func() { _ = s.cfg.Slots.Release(slot) }()
	defer al.cancelCtx()
	l := al.lease
	al.lw = newLineWriter(func(text string) {
		_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stdout", Text: text}, al)
	}, al.masks)
	root := JobRoot{Dir: filepath.Join(s.cfg.StateDir, "jobs", unsafeName.ReplaceAllString(l.LeaseID, "_")+"-"+strconv.FormatUint(l.Epoch, 10))}
	if err := os.MkdirAll(root.Dir, 0o700); err != nil {
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
	for _, h := range s.jobHooks {
		if h.Before == nil {
			continue
		}
		got, err := h.Before(ctx, l, root)
		if err != nil {
			s.finish(ctx, al, root, JobResult{Outcome: Outcome{State: "failed", FailureClass: "infra"}}, fmt.Errorf("job hook %s: %w", h.Name, err))
			return
		}
		mergeSetup(&setup, got)
	}
	for ctx.Err() == nil && missingSecrets(l.Job.Secrets, setup.Secrets) {
		select {
		case sec := <-al.secrets:
			for _, v := range sec.Values {
				if setup.Secrets == nil {
					setup.Secrets = map[string]string{}
				}
				setup.Secrets[v.Ref] = v.Value
				if _, name, ok := strings.Cut(v.Ref, "/"); ok {
					setup.Secrets[name] = v.Value
				}
			}
		case <-ctx.Done():
		}
	}
	res, err := s.cfg.Runner.Run(ctx, JobRun{Lease: l, Slot: slot.Index, Root: root, Setup: setup, Credential: cred, Output: al.lw.Write})
	al.lw.Flush()
	s.finish(ctx, al, root, res, err)
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
		_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stderr", Text: "agent: " + oneLine(runErr.Error())}, al)
	}
	hookCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	up := &uploader{s: s, al: al}
	for _, h := range s.jobHooks {
		if h.After == nil {
			continue
		}
		if err := h.After(hookCtx, l, root, res.Outcome, up); err != nil {
			_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stderr", Text: "agent: job hook " + h.Name + ": " + oneLine(err.Error())}, al)
		}
	}
	ev := res.Evidence
	if len(ev) == 0 || !json.Valid(ev) {
		// The runner failed before it had a document. This one cannot validate as
		// ci.evidence/v1, so the coordinator records the lease as errored.
		ev = json.RawMessage(`{"schema":"ci.evidence/v1","result":"error","reason":"agent_runner_failed"}`)
	}
	sig, err := signEvidence(currentSigner(s.cfg.Signer), ev)
	if err != nil {
		_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stderr", Text: "agent: result not signed: " + oneLine(err.Error())}, al)
		sig = protocol.Signature{}
	}
	_ = s.send("result", protocol.Result{LeaseID: l.LeaseID, Epoch: l.Epoch, Evidence: ev, Signature: sig}, al)
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
