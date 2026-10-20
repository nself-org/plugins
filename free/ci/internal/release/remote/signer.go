package remote

// Purpose: LocalSigner, the operator's own signer for a TEAM run: the same signing.Handler the
// isolated signer process runs (it re-checks every payload against the run's facts and the staged,
// re-hashed bytes, resolves the release key by reference, signs and zeroes the key), called in this
// process over the Snapshot instead of over a run store the operator's machine does not have.
// Inputs:  the Snapshot, the staged blob store, the secret resolver of the operator's project.
// Outputs: DSSE envelopes (publish.Signer) and the key's trust line.
// Constraints: the key is resolved only by the operator's machine and only after every payload
// passed the checks; nothing is sent anywhere; a signing request is never forwarded to the
// coordinator; the handler's project is the run's own project (the repository match against the
// operator's checkout is checked by Sign before the key is touched).

import (
	"bytes"
	"context"
	"encoding/json"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

// LocalSigner implements publish.Signer over a signing.Handler.
type LocalSigner struct {
	H   signing.Handler
	Res secrets.ValueResolver
}

// NewLocalSigner builds the signer over src and blobs.
func NewLocalSigner(src signing.Source, blobs signing.Blobs, res secrets.ValueResolver, project string) *LocalSigner {
	return &LocalSigner{H: signing.Handler{Src: src, Blobs: blobs, Project: project, Resolver: res}, Res: res}
}

// Sign checks and signs the items of one request.
func (s *LocalSigner) Sign(ctx context.Context, run string, items []signing.Item) ([]sdk.Envelope, error) {
	req := signing.Request{RunID: run, Items: items}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, signing.Failf(signing.ReasonPayloadRejected, "the request could not be encoded")
	}
	if len(raw) > signing.MaxRequestBytes {
		return nil, signing.Failf(signing.ReasonPayloadRejected, "the request is larger than %d bytes", signing.MaxRequestBytes)
	}
	req, payloads, err := signing.DecodeRequest(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return s.H.Handle(ctx, req, payloads)
}

// PublicKey resolves the release key and returns its public half only.
func (s *LocalSigner) PublicKey(ctx context.Context) (signing.KeyInfo, error) {
	return signing.Public(ctx, s.Res, signing.KeyAttempt)
}
