package attest

// Purpose: the RemoteVerifier for signed-remote evidence (Epic D13, review R-AR-17). Every key-id,
// purpose and revocation decision is made by cli sdk/go/signing; this file only supplies the key set (the
// node registry's pinned node keys) and the revoked list, then checks what the sdk cannot know: the
// attempt, its revision, who held its lease, whether the node may sign, and the replay set.
// Inputs:  the raw document (or the re-marshalled evidence) and its decoded form.
// Outputs: nil, or a *RemoteError whose Reason says why (code E686).
// Constraints: fail closed on every ledger error; checks run in a fixed order (signature fields, attempt,
//          revision, replay, then the sdk's revoked/unknown/purpose/signature, then lease holder and
//          eligibility), the use is consumed only after all of them pass; a node whose registry record
//          says signing_eligible false never has its evidence accepted.

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// RunnerVerifier checks runner-ed25519 evidence signatures against a Ledger.
type RunnerVerifier struct {
	// Ledger returns the coordinator's ledger; nil, or a nil result, refuses (ledger_unavailable).
	Ledger func() Ledger
	// Timeout bounds one verification (default 10 s).
	Timeout time.Duration
}

var _ RemoteVerifier = (*RunnerVerifier)(nil)

// decodeSignature accepts the two canonical text forms of a raw Ed25519 signature: unpadded standard
// base64 (the agent's result frame) and padded (signing.EncodeSig), both strict.
func decodeSignature(v string) ([]byte, error) {
	if !strings.HasSuffix(v, "=") {
		raw, err := base64.RawStdEncoding.Strict().DecodeString(v)
		if err == nil && len(raw) == ed25519.SignatureSize {
			return raw, nil
		}
		return nil, errors.New("not a canonical signature")
	}
	return signing.DecodeSig(v)
}

// splitKeyID splits "<purpose>-<16 lower-case hex>" into its purpose and the node key id.
func splitKeyID(id string) (signing.Purpose, string, bool) {
	i := strings.LastIndexByte(id, '-')
	if i <= 0 || len(id)-i-1 != 16 {
		return "", "", false
	}
	p, hexID := signing.Purpose(id[:i]), id[i+1:]
	for _, c := range hexID {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", "", false
		}
	}
	return p, hexID, p.Valid()
}

// VerifyEvidence implements RemoteVerifier.
func (v *RunnerVerifier) VerifyEvidence(doc []byte, ev model.Evidence) error {
	s := ev.Signature
	if s.Kind != "runner-ed25519" || s.KeyID == nil || s.Value == nil || *s.KeyID == "" || *s.Value == "" {
		return remoteFail(RemoteSignatureMissing, "no runner signature", nil)
	}
	if ev.Attempt == nil || ev.Attempt.ID == "" {
		return remoteFail(RemoteAttemptUnknown, "the evidence names no attempt", nil)
	}
	obj, err := decodeObject(doc)
	if err != nil {
		return remoteFail(RemoteSignatureInvalid, "no signing payload: "+err.Error(), nil)
	}
	if !documentMatches(obj, ev) {
		return remoteFail(RemoteSignatureInvalid, "the document and the decoded evidence disagree", nil)
	}
	payload, err := payloadOf(obj)
	if err != nil {
		return remoteFail(RemoteSignatureInvalid, "no signing payload: "+err.Error(), nil)
	}
	raw, err := decodeSignature(*s.Value)
	if err != nil {
		return remoteFail(RemoteSignatureMalformed, err.Error(), nil)
	}
	var led Ledger
	if v != nil && v.Ledger != nil {
		led = v.Ledger()
	}
	if led == nil {
		return remoteFail(RemoteLedgerUnavailable, "no ledger", nil)
	}
	limit := v.Timeout
	if limit <= 0 {
		limit = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	return verifyAgainst(ctx, led, ev, payload, raw)
}

// contextMismatch names the first evidence field that differs from what the coordinator recorded for the
// attempt ("" when they agree). The revision is checked before this by its own reason.
func contextMismatch(f AttemptFacts, ev model.Evidence) string {
	switch {
	case ev.RunID != f.RunID:
		return "run_id"
	case ev.RepoID != f.RepoID:
		return "repo_id"
	case string(ev.SourceTrust) != f.SourceTrust:
		return "source_trust"
	case ev.PolicyDigest != f.PolicyDigest:
		return "policy_digest"
	case string(ev.Selection.Mode) != f.SelectionMode:
		return "selection.mode"
	case ev.Job != nil && *ev.Job != f.Job:
		return "job"
	}
	return ""
}

// documentMatches reports whether the bytes that are signed and the decoded evidence the checks read
// name the same revision, run, repository and attempt, so a caller that pairs the two cannot have the
// signature cover one document while the lease and revision checks judge another.
func documentMatches(obj map[string]any, ev model.Evidence) bool {
	str := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	att, _ := obj["attempt"].(map[string]any)
	return str(obj, "revision") == ev.Revision && str(obj, "run_id") == ev.RunID && str(obj, "repo_id") == ev.RepoID &&
		att != nil && str(att, "id") == ev.Attempt.ID
}

func verifyAgainst(ctx context.Context, led Ledger, ev model.Evidence, payload, raw []byte) error {
	keyID, attempt, digest := *ev.Signature.KeyID, ev.Attempt.ID, SignatureDigest(raw)
	facts, ok, err := led.AttemptFacts(ctx, attempt)
	if err != nil {
		return remoteFail(RemoteLedgerUnavailable, "attempt lookup", err)
	}
	if !ok {
		return remoteFail(RemoteAttemptUnknown, "no lease holder is recorded for the attempt", nil)
	}
	if ev.Revision != facts.Revision {
		return remoteFail(RemoteRevisionMismatch, "the evidence revision is not the attempt's revision", nil)
	}
	if field := contextMismatch(facts, ev); field != "" {
		return remoteFail(RemoteContextMismatch, "the evidence "+field+" is not the attempt's", nil)
	}
	if replayed, rerr := led.Replayed(ctx, attempt, keyID, digest); rerr != nil {
		return remoteFail(RemoteLedgerUnavailable, "replay lookup", rerr)
	} else if replayed {
		return remoteFail(RemoteReplay, "the signature was consumed for another attempt", nil)
	}
	var nodes []KeyNode
	var ledgerErr error
	lookup := func(ctx context.Context, id string) (signing.Key, bool, error) {
		purpose, hexID, ok := splitKeyID(id)
		if !ok {
			return signing.Key{}, false, nil
		}
		found, err := led.NodesByKey(ctx, hexID)
		if err != nil {
			ledgerErr = err
			return signing.Key{}, false, err
		}
		if len(found) == 0 {
			return signing.Key{}, false, nil
		}
		for _, n := range found[1:] {
			if !n.Public.Equal(found[0].Public) {
				return signing.Key{}, false, nil // two different keys under one id: refuse both
			}
		}
		nodes = found
		return signing.Key{ID: id, Purpose: purpose, Public: found[0].Public}, true, nil
	}
	revoked := func(id string) bool {
		r, err := led.Revoked(ctx, id)
		if err != nil {
			ledgerErr = err
			return true
		}
		return r
	}
	sv, err := signing.NewLookupVerifier(signing.PurposeCINode, lookup, signing.WithRevoked(revoked))
	if err == nil {
		err = sv.VerifyContext(ctx, payload, signing.Signature{KeyID: keyID, Sig: raw})
	}
	if err != nil {
		return mapSigning(err, ledgerErr)
	}
	holder := KeyNode{}
	for _, n := range nodes {
		if n.NodeID == facts.RunnerID {
			holder = n
		}
	}
	switch {
	case holder.NodeID == "":
		return remoteFail(RemoteLeaseHolderMismatch, "the key is not the pinned key of the node that held the lease", nil)
	case !holder.SigningEligible:
		return remoteFail(RemoteNodeNotEligible, "the node is not signing eligible", nil)
	}
	// The use is recorded under the coordinator's own context (equal to the evidence's, checked above), so
	// a revocation finds the runs, repositories and jobs the key vouched for.
	use := Use{AttemptID: attempt, KeyID: keyID, SigDigest: digest, NodeID: holder.NodeID, RunID: facts.RunID, RepoID: facts.RepoID, Revision: facts.Revision, Job: facts.Job}
	if err := led.Consume(ctx, use); errors.Is(err, ErrReplay) {
		return remoteFail(RemoteReplay, "the signature was consumed for another attempt", nil)
	} else if errors.Is(err, ErrRevoked) {
		return remoteFail(RemoteKeyRevoked, "the key was revoked", err)
	} else if err != nil {
		return remoteFail(RemoteLedgerUnavailable, "consume", err)
	}
	return nil
}

// mapSigning turns an sdk refusal into a reason. ledgerErr is a ledger failure the sdk saw as a refusal.
func mapSigning(err, ledgerErr error) error {
	switch {
	case ledgerErr != nil:
		return remoteFail(RemoteLedgerUnavailable, "key lookup", ledgerErr)
	case errors.Is(err, signing.ErrRevoked):
		return remoteFail(RemoteKeyRevoked, "the key is revoked", err)
	case errors.Is(err, signing.ErrUnknownKey):
		return remoteFail(RemoteKeyUnknown, "no node pins this key", err)
	case errors.Is(err, signing.ErrWrongPurpose):
		return remoteFail(RemoteKeyPurpose, "the key is not a ci-node key", err)
	case errors.Is(err, signing.ErrBadSignature):
		return remoteFail(RemoteSignatureInvalid, "the signature does not verify", err)
	case errors.Is(err, signing.ErrMalformed):
		return remoteFail(RemoteKeyMalformed, "the key id is malformed", err)
	}
	return remoteFail(RemoteKeyUnknown, "the key is not valid", err)
}
