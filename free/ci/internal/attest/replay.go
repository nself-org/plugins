package attest

// Purpose: the coordinator-side facts a runner signature is checked against, as one interface so the
// verifier and Revoke need no store (internal/store imports authz, which imports this package): the
// attempt's lease holder and revision, the nodes that pin a key, the revoked evidence keys and the replay
// set of consumed (attempt, key id, signature) triples.
// Inputs:  implemented by store.AttestLedger in production and by a map in tests.
// Outputs: AttemptFacts, KeyNode, Use, Ledger, ErrReplay, SignatureDigest.
// Constraints: every method fails closed: an error is a refusal, never a pass; Consume is atomic and
//          idempotent for one triple and returns ErrReplay for a signature already consumed under
//          another attempt.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ErrReplay is returned by Ledger.Consume for a signature consumed under another attempt.
var ErrReplay = errors.New("attest: signature already consumed for another attempt")

// ErrRevoked is returned by Ledger.Consume for a key revoked since the verifier checked it (a revocation
// that lands between the check and the consume must not let the use through).
var ErrRevoked = errors.New("attest: key revoked")

// AttemptFacts is what the coordinator recorded of an attempt. A node chooses every field of its
// evidence, so each one the coordinator also knows must equal it (a forged repository, trust class or
// policy digest would otherwise ride on a valid signature).
type AttemptFacts struct {
	// RunnerID is the node that held the lease of the attempt.
	RunnerID string
	// Revision, RunID, RepoID, SourceTrust, PolicyDigest and SelectionMode are those of the attempt's
	// pipeline; Job is the name of the attempt's job.
	Revision, RunID, RepoID, SourceTrust, PolicyDigest, SelectionMode, Job string
}

// KeyNode is a node whose pinned node key has a given key id.
type KeyNode struct {
	NodeID string
	Public ed25519.PublicKey
	// SigningEligible is the node's registry fact (NODE D8); false refuses its evidence.
	SigningEligible bool
}

// Use is one accepted signed-remote document.
type Use struct {
	AttemptID, KeyID, SigDigest, NodeID, RunID, RepoID, Revision, Job string
}

// Ledger is the verifier's view of the coordinator.
type Ledger interface {
	// AttemptFacts returns the facts of an attempt; ok is false for an unknown attempt or one with no
	// recorded lease holder.
	AttemptFacts(ctx context.Context, attemptID string) (AttemptFacts, bool, error)
	// NodesByKey lists the nodes that pin the 16-hex node key id.
	NodesByKey(ctx context.Context, nodeKeyID string) ([]KeyNode, error)
	// Revoked reports whether the evidence key id (ci-node-<16 hex>) is revoked.
	Revoked(ctx context.Context, keyID string) (bool, error)
	// Replayed reports whether (keyID, sigDigest) was consumed under an attempt other than attemptID.
	Replayed(ctx context.Context, attemptID, keyID, sigDigest string) (bool, error)
	// Consume records u; the same triple again is a no-op. It returns ErrReplay for a signature consumed
	// under another attempt and ErrRevoked when the key is revoked at the time of the write.
	Consume(ctx context.Context, u Use) error
}

// SignatureDigest is the replay-set form of a raw signature: the lower-case hex SHA-256, so the padded
// and the unpadded text of one signature are one entry.
func SignatureDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
