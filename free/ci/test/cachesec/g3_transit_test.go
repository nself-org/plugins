//go:build cachesec

package cachesec

// Purpose: TM-15 blobs damaged at the SINK BOUNDARY of the real ssh carrier (real sshd container, real agent): one bit
// flipped and a blob cut in half at the transfer boundary of the coordinator sink. The fault is applied after the
// carrier delivered the bytes to the sink, not on the wire (no netem or proxy fault). The carrier refuses the blob (E656
// transport.ssh_blob) and the handler's own verdict is captured from the writer; production appends no audit row for
// either (debt D-1451), so the case asserts "none".
// Inputs: the shared g3Fleet. Outputs: registered fleet cases of the g3 rows of matrix.yaml.
// Constraints: the only injected fault is a wrapper around the admitted blob writer; no production check is changed.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// g3FaultWriter damages the bytes passing through (flip one bit, or deliver only half) and records the handler's errors.
type g3FaultWriter struct {
	transport.BlobWriter
	s       *g3Sink
	flip    bool
	cut     bool
	size    int64
	n       int64
	flipped bool
}

func (f *g3FaultWriter) Write(p []byte) (int, error) {
	if f.flip && !f.flipped && len(p) > 0 {
		f.flipped = true
		q := append([]byte(nil), p...)
		q[len(q)/2] ^= 0x01
		_, err := f.BlobWriter.Write(q)
		f.s.note(err)
		return len(p), err
	}
	if f.cut {
		room := f.size/2 - f.n
		if room <= 0 {
			return len(p), nil
		}
		q := p
		if int64(len(q)) > room {
			q = q[:room]
		}
		n, err := f.BlobWriter.Write(q)
		f.n += int64(n)
		f.s.note(err)
		return len(p), err
	}
	n, err := f.BlobWriter.Write(p)
	f.s.note(err)
	return n, err
}

func (f *g3FaultWriter) Close() error {
	err := f.BlobWriter.Close()
	f.s.note(err)
	return err
}

func (f *g3FaultWriter) Abort() error {
	if a, ok := f.BlobWriter.(interface{ Abort() error }); ok {
		return a.Abort()
	}
	return nil
}

func (f *g3FaultWriter) Verify(want string) error {
	if v, ok := f.BlobWriter.(interface{ Verify(string) error }); ok {
		err := v.Verify(want)
		f.s.note(err)
		return err
	}
	return nil
}

// g3TransitCase runs the damaged-blob attack on the fleet after an honest control, and wants the handler reason.
func g3TransitCase(t *testing.T, fl *g3Fleet, id string, damage func(s *g3Sink) func(transport.BlobWriter) transport.BlobWriter, wantReason string) Observed {
	g3ArtifactControl(t, fl, "c"+id)
	w := newWorld(t)
	r := newG3Rig(t, w, "x"+id, g3Main, nil, g3FleetArtifactJob())
	s := r.Sink()
	s.wrapW = damage(s)
	runErr := fl.Run(t, r, s)
	got := g3Refused(t, runErr) // the carrier's verdict: E656 transport.ssh_blob
	var handler error
	for _, e := range s.Errs() {
		if c, reason := g3Reasons(e); c == "E714" && reason == wantReason {
			handler = e
		}
	}
	if handler == nil || !transfer.IsReason(handler, wantReason) {
		t.Fatalf("the handler did not refuse the blob as E714 %s: %v", wantReason, s.Errs())
	}
	got.Audit = g3NoAudit(t, w)
	got.Landed = r.Landed(g3Sum([]byte(g3ArtifactBody)))
	return got
}

func init() {
	g3RegisterFleet("C15-SSH-02", func(t *testing.T, fl *g3Fleet) Observed {
		return g3TransitCase(t, fl, "flip", func(s *g3Sink) func(transport.BlobWriter) transport.BlobWriter {
			return func(w transport.BlobWriter) transport.BlobWriter {
				return &g3FaultWriter{BlobWriter: w, s: s, flip: true}
			}
		}, transfer.ReasonDigest)
	})
	g3RegisterFleet("C15-SSH-03", func(t *testing.T, fl *g3Fleet) Observed {
		return g3TransitCase(t, fl, "cut", func(s *g3Sink) func(transport.BlobWriter) transport.BlobWriter {
			return func(w transport.BlobWriter) transport.BlobWriter {
				return &g3FaultWriter{BlobWriter: w, s: s, cut: true, size: int64(len(g3ArtifactBody))}
			}
		}, transfer.ReasonInterrupted)
	})
}
