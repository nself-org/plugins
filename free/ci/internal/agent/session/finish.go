package session

// Purpose: the end of a lease: After hooks, canonical evidence, the optional
// signature, and the result frame; and the Uploader hooks use to send blobs.

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"strings"
	"time"
)

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

type uploader struct {
	s  *Session
	al *activeLease
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
	hookFatal := false
	for _, h := range s.jobHooks {
		if h.After == nil {
			continue
		}
		if err := h.After(hookCtx, l, root, res.Outcome, up); err != nil {
			_ = s.send("log", protocol.Log{LeaseID: l.LeaseID, LineSeq: al.lines.Add(1), Stream: "stderr", Text: oneLine(al.redact("agent: job hook " + h.Name + ": " + err.Error()))}, al)
			hookFatal = hookFatal || errors.Is(err, ErrHookFatal)
		}
	}
	ev, cerr := canonicalJSON(res.Evidence)
	if hookFatal && res.Outcome.State == "passed" {
		// A hook that owes the coordinator something the result depends on (the artifacts of the job)
		// failed to deliver it: the job did not pass, whatever its command said.
		ev, cerr = json.RawMessage(`{"schema":"ci.evidence/v1","result":"error","reason":"agent_hook_failed"}`), nil
	}
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

// uploader sends transfer frames for one lease.
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
