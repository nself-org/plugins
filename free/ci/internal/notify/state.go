package notify

// Purpose: the durable state the engine needs (de-duplication, last result per ref, digest
// days, delivery counters), as an interface so the engine has no SQL, plus the adapter over
// the store's notify set.
// Inputs: ids, results, counts. Outputs: claims and the previous result.
// Constraints: the interface carries no URL, argv or log text.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// DigestStats are the counts a digest reports.
type DigestStats = store.DigestStats

// State is what the engine reads and writes.
type State interface {
	// Claim takes one (repo, sha, context, kind) message; false means it was already sent.
	Claim(ctx context.Context, repo, sha, statusContext string, kind Kind) (bool, error)
	// Release gives a claim back (no sink took the message).
	Release(ctx context.Context, repo, sha, statusContext string, kind Kind) error
	// Observe records the latest result of (repo, ref, context) and returns the previous one.
	Observe(ctx context.Context, repo, ref, statusContext, sha, result string) (string, error)
	DigestClaim(ctx context.Context, day string) (bool, error)
	DigestRelease(ctx context.Context, day string) error
	// LastDigest is when the latest digest was claimed, in unix nanoseconds (0: never).
	LastDigest(ctx context.Context) (int64, error)
	DigestStats(ctx context.Context, since, until int64) (DigestStats, error)
	RecordDelivery(ctx context.Context, sink string, ok bool) error
}

// StoreState adapts a store to State.
type StoreState struct{ S *store.Store }

func (s StoreState) Claim(ctx context.Context, repo, sha, c string, k Kind) (bool, error) {
	return s.S.NotifyClaim(ctx, repo, sha, c, string(k))
}
func (s StoreState) Release(ctx context.Context, repo, sha, c string, k Kind) error {
	return s.S.NotifyRelease(ctx, repo, sha, c, string(k))
}
func (s StoreState) Observe(ctx context.Context, repo, ref, c, sha, result string) (string, error) {
	return s.S.NotifyObserve(ctx, repo, ref, c, sha, result)
}
func (s StoreState) DigestClaim(ctx context.Context, day string) (bool, error) {
	return s.S.NotifyDigestClaim(ctx, day)
}
func (s StoreState) DigestRelease(ctx context.Context, day string) error {
	return s.S.NotifyDigestRelease(ctx, day)
}
func (s StoreState) LastDigest(ctx context.Context) (int64, error) { return s.S.NotifyLastDigest(ctx) }
func (s StoreState) DigestStats(ctx context.Context, since, until int64) (DigestStats, error) {
	return s.S.NotifyDigestStats(ctx, since, until)
}
func (s StoreState) RecordDelivery(ctx context.Context, sink string, ok bool) error {
	return s.S.NotifyRecordDelivery(ctx, sink, ok)
}
