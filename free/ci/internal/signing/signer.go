package signing

// Purpose: the signer's request handler (Epic CACHE D12): re-check every
// payload against the store, resolve the release key itself, sign DSSE
// envelopes, zero the key and return envelopes only.
// Inputs:  a decoded Request and its payloads; Source, Blobs and a
// secrets.ValueResolver.
// Outputs: one DSSE envelope per item, in request order.
// Constraints: all items are checked before the key is resolved, and the key
// is resolved only when every item passed, so a refused request never touches
// the secret; one failing item fails the whole request (no partial
// signature); the key is zeroed on every path; a manifest that names another
// key id is refused (key_id_mismatch).

import (
	"context"
	"sort"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

// Handler signs checked requests.
type Handler struct {
	Src      Source
	Blobs    Blobs
	Resolver secrets.ValueResolver
}

// Handle checks and signs one request.
func (h Handler) Handle(ctx context.Context, req Request, payloads [][]byte) ([]signing.Envelope, error) {
	facts, err := loadFacts(ctx, h.Src, h.Blobs, req.RunID)
	if err != nil {
		return nil, err
	}
	attempt, wantKeyID := "", ""
	for i, it := range req.Items {
		switch it.PayloadType {
		case manifest.PayloadType:
			a, k, err := facts.checkManifest(payloads[i])
			if err != nil {
				return nil, err
			}
			if wantKeyID != "" && wantKeyID != k {
				return nil, Failf(ReasonKeyIDMismatch, "the manifests of one request name different keys")
			}
			attempt, wantKeyID = a, k
		case PayloadStatement:
			if err := facts.checkStatement(payloads[i]); err != nil {
				return nil, err
			}
		}
	}
	if attempt == "" && (facts.lastReleaseAttempt() == "") {
		return nil, Failf(ReasonPayloadRejected, "run %q has no passed release attempt", echo(req.RunID))
	}
	if attempt == "" {
		attempt = facts.lastReleaseAttempt()
	}
	key, err := LoadKey(ctx, h.Resolver, attempt)
	if err != nil {
		return nil, err
	}
	defer key.Zero()
	if wantKeyID != "" && wantKeyID != key.ID {
		return nil, Failf(ReasonKeyIDMismatch, "the manifest names key %s but the release key is %s", echo(wantKeyID), key.ID)
	}
	out := make([]signing.Envelope, len(req.Items))
	for i, it := range req.Items {
		if out[i], err = signing.SignEnvelope(key.Signer, it.PayloadType, payloads[i]); err != nil {
			return nil, Failf(ReasonSignerFailed, "an envelope could not be signed")
		}
	}
	return out, nil
}

// lastReleaseAttempt is the highest-numbered passed attempt of a release job.
func (f *runFacts) lastReleaseAttempt() string {
	release := map[string]bool{}
	for _, j := range f.jobs {
		if j.Kind == "release" {
			release[j.ID] = true
		}
	}
	type cand struct {
		id string
		n  int
	}
	var cs []cand
	for _, a := range f.attempts {
		if release[a.JobID] && a.State == "passed" {
			cs = append(cs, cand{a.ID, a.N})
		}
	}
	if len(cs) == 0 {
		return ""
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].n > cs[j].n })
	return cs[0].id
}

// KeyInfo is what the key commands print and the signer's key modes return.
type KeyInfo struct {
	KeyID  string `json:"key_id"`
	Public string `json:"public_key"`
	// OldKeyID is set by a rotation: the id for `.revoked`.
	OldKeyID string `json:"old_key_id,omitempty"`
}

// TrustLine is the line `.nself/trust/ci-release.keys` takes.
func (k KeyInfo) TrustLine() string { return k.KeyID + " " + k.Public }

// Public resolves the key and returns its public half only.
func Public(ctx context.Context, res secrets.ValueResolver, attempt string) (KeyInfo, error) {
	key, err := LoadKey(ctx, res, attempt)
	if err != nil {
		return KeyInfo{}, err
	}
	defer key.Zero()
	return KeyInfo{KeyID: key.ID, Public: key.PublicB64()}, nil
}
