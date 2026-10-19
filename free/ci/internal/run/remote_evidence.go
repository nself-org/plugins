package run

// Purpose: what the remote sink does with the node's untrusted frames that is not log or result
// bookkeeping: the evidence check, and the refs and blobs a foreground run does not collect yet.
// Inputs:  protocol frames from a transport driver.
// Outputs: an evidence document that is a pass only when it is this attempt's own; discarded blobs.
// Constraints: see remote_sink.go; nothing here trusts the document's claims beyond its envelope.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// checkEvidence decodes the node's evidence and holds it to the delivery: a document that does not
// parse, is not ci.evidence/v1, names another attempt or revision, or passes with a failing check is an
// error result and never a pass. The envelope (lease and epoch) was already fenced by the driver.
func (ra *remoteAttempt) checkEvidence(raw json.RawMessage) model.Evidence {
	bad := model.Evidence{Result: "error"}
	var ev model.Evidence
	if err := json.Unmarshal(raw, &ev); err != nil || ev.Result == "" || ev.Schema != "ci.evidence/v1" {
		return bad
	}
	if ev.Attempt == nil || ev.Attempt.ID != ra.attemptID || ra.revision != "" && !strings.EqualFold(ev.Revision, ra.revision) {
		return bad
	}
	if ev.Result == "pass" {
		for _, c := range ev.Checks {
			if c.Result == "fail" || c.Result == "error" {
				return bad
			}
		}
	}
	return ev
}

// ArtifactRef and CacheRef are not collected by a foreground run (see the file comment).
func (*remoteSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (*remoteSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }

// Blob discards the bytes: the digest check of the driver still runs.
func (*remoteSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return &discardBlob{}, nil
}

type discardBlob struct{ n int64 }

func (b *discardBlob) Write(p []byte) (int, error) { b.n += int64(len(p)); return len(p), nil }
func (b *discardBlob) Close() error                { return nil }
func (b *discardBlob) Offset() int64               { return b.n }
func (b *discardBlob) Abort() error                { return nil }

var _ transport.Sink = (*remoteSink)(nil)
