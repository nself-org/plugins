//go:build cachesec

package cachesec

// Purpose: group g3 helpers shared by the case files: the code and reason key of a production refusal, the
// Observed builder for it, a straight-into-the-sink blob upload, a byte-reader and the artifact commit.
// Inputs: errors and sinks of the g3 rig. Outputs: g3Reasons, g3Refused, g3Upload, bytesReader, CommitArtifacts.
// Constraints: no production check is weakened; codes and reasons come from the production error types.

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	sshdrv "github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

// g3Reasons extracts the code and reason key of a typed fabric error produced by the owning handlers.
func g3Reasons(err error) (code, reason string) {
	var tr *transfer.Error
	var en *entries.Refused
	var rec interface{ Code() string }
	var dn *authz.Denied
	var se *sshdrv.Error
	var pe *authz.PathError
	var rr *records.Error
	switch {
	case errors.As(err, &pe):
		return authz.CodeArtifactPath, string(pe.Reason)
	case errors.As(err, &tr):
		return tr.Code(), tr.Reason
	case errors.As(err, &en):
		return en.Code(), en.Reason
	case errors.As(err, &se):
		return se.Code, se.Reason
	case errors.As(err, &dn):
		return authz.CodeCacheWrite, string(dn.Reason)
	case errors.As(err, &rr):
		return rr.Code(), rr.Reason
	case errors.As(err, &rec):
		return rec.Code(), ""
	}
	return "", ""
}

var _ = authz.ModeNone

// CommitArtifacts finishes the attempt the way the run does and returns the committed records.
func (r *g3rig) CommitArtifacts() ([]records.Record, error) {
	state, epoch, err := r.w.St.AttemptStateEpoch(r.w.ctx, r.id)
	if err != nil {
		r.t.Fatal(err)
	}
	if state != "finalizing" {
		r.t.Fatalf("attempt %s is %s after the result, want finalizing", r.id, state)
	}
	return records.CommitAndVerify(r.w.ctx, r.w.St, r.w.Blobs, r.id, epoch, r.pol)
}

// g3Upload plays a node's blob upload straight into the sink (no carrier): announce, stream, close.
func g3Upload(ctx context.Context, s *g3Sink, stream string, data []byte) error {
	w, err := s.Blob(ctx, transport.BlobHeader{Stream: stream, SHA256: g3Sum(data), Size: int64(len(data))})
	if err != nil {
		return err
	}
	if _, err = w.Write(data); err != nil {
		return err
	}
	return w.Close()
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// g3Refused turns an error produced by the owning packages into the Observed refusal (code and reason key from
// the production type); it fails when err is nil or carries no fabric code.
func g3Refused(t *testing.T, err error) Observed {
	t.Helper()
	if err == nil {
		t.Fatal("the operation was admitted; the matrix expects a refusal")
	}
	code, reason := g3Reasons(err)
	if code == "" {
		t.Fatalf("the refusal carries no fabric code: %v", err)
	}
	// the matrix reason pattern is [a-z0-9_]+; production keys such as path.traversal keep their dot in the code, so the dot reads as _ here
	return Observed{Outcome: Refused, Code: code, Reason: strings.ReplaceAll(reason, ".", "_")}
}

// g3NoAudit proves production appended no refusal audit row (action containing "refused" or "denied") and
// returns "none". The case fails the day production adds one, so the debt row has to be closed with it.
func g3NoAudit(t *testing.T, w *world) string {
	t.Helper()
	for _, r := range w.Audit("") {
		if strings.Contains(r.Action, "refused") || strings.Contains(r.Action, "denied") {
			t.Fatalf("production now audits this refusal (%s %s): close the debt gap and switch the row to that action", r.Action, r.Detail)
		}
	}
	return "none"
}
