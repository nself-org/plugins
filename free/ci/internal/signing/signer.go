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

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

// Handler signs checked requests.
type Handler struct {
	Src   Source
	Blobs Blobs
	// Project is the project root this signer serves: it resolves that
	// project's release key, so it signs only that project's runs.
	Project  string
	Resolver secrets.ValueResolver
}

// checked is what a pass over the request found.
type checked struct {
	attempt string // the release attempt the key is resolved for
	keyID   string // the key id the manifests name ("" when none)
}

// Handle checks and signs one request. Every item is checked, the key is
// resolved, and every item is checked again against fresh facts before
// anything is signed, so the run cannot change while the key is resolved.
func (h Handler) Handle(ctx context.Context, req Request, payloads [][]byte) ([]signing.Envelope, error) {
	first, err := h.check(ctx, req, payloads)
	if err != nil {
		return nil, err
	}
	key, err := LoadKey(ctx, h.Resolver, first.attempt)
	if err != nil {
		return nil, err
	}
	defer key.Zero()
	if first.keyID != "" && first.keyID != key.ID {
		return nil, Failf(ReasonKeyIDMismatch, "the manifest names key %s but the release key is %s", echo(first.keyID), key.ID)
	}
	second, err := h.check(ctx, req, payloads)
	if err != nil {
		return nil, err
	}
	if second != first {
		return nil, Failf(ReasonPayloadRejected, "the run changed while the key was resolved")
	}
	out := make([]signing.Envelope, len(req.Items))
	for i, it := range req.Items {
		if out[i], err = signing.SignEnvelope(key.Signer, it.PayloadType, payloads[i]); err != nil {
			return nil, Failf(ReasonSignerFailed, "an envelope could not be signed")
		}
	}
	return out, nil
}

// check loads fresh facts and validates every item against them.
func (h Handler) check(ctx context.Context, req Request, payloads [][]byte) (checked, error) {
	facts, err := loadFacts(ctx, h.Src, h.Blobs, req.RunID, h.Project)
	if err != nil {
		return checked{}, err
	}
	var c checked
	for i, it := range req.Items {
		switch it.PayloadType {
		case manifest.PayloadType:
			a, k, err := facts.checkManifest(ctx, payloads[i])
			if err != nil {
				return checked{}, err
			}
			if c.keyID != "" && c.keyID != k {
				return checked{}, Failf(ReasonKeyIDMismatch, "the manifests of one request name different keys")
			}
			c.attempt, c.keyID = a, k
		case PayloadStatement:
			if err := facts.checkStatement(ctx, payloads[i]); err != nil {
				return checked{}, err
			}
		}
	}
	if c.attempt == "" {
		if c.attempt = facts.lastReleaseAttempt(); c.attempt == "" {
			return checked{}, Failf(ReasonPayloadRejected, "run %q has no passed release attempt", echo(req.RunID))
		}
	}
	return c, nil
}

// lastReleaseAttempt is the highest-numbered passed attempt of a release job
// (the first listed when two share a number).
func (f *runFacts) lastReleaseAttempt() string {
	release := map[string]bool{}
	for _, j := range f.jobs {
		if j.Kind == "release" {
			release[j.ID] = true
		}
	}
	best, bestN := "", 0
	for _, a := range f.attempts {
		if release[a.JobID] && a.State == "passed" && (best == "" || a.N > bestN) {
			best, bestN = a.ID, a.N
		}
	}
	return best
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
