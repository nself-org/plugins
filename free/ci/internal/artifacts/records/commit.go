package records

// Purpose: stage collected files as records and make them part of a pass:
// CommitAndVerify re-hashes every blob before the attempt may leave
// `finalizing` (R-DS-20: results before artifacts, verified digests only).
// Inputs:  Facts, Staged values, the Ledger, the Blobs, the lease epoch.
// Outputs: Records, the committed records, a refusal that fails the attempt.
// Constraints: the digest of a blob is re-computed from the stored bytes
// (cas.Open hashes before it serves); a missing, resized or unverifiable blob
// fails the commit (E712, class infra); a declaration with no committed
// record fails it (missing); the ledger re-checks state and epoch.

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

// ToRecords builds the staged records of one attempt from the stored facts.
func ToRecords(attempt string, f Facts, epoch int64, staged []Staged, now time.Time) []Record {
	out := make([]Record, 0, len(staged))
	for _, s := range staged {
		r := Record{ID: RecordID(attempt, s.Name), Project: f.Project, Run: f.Run, Job: f.Job, Attempt: attempt, Revision: f.Revision,
			Name: s.Name, Path: s.Decl.Path, Decl: s.Decl.Ord, Kind: s.Decl.Kind, SHA256: s.SHA256, Size: s.Size, Platform: f.Platform,
			RunnerID: f.RunnerID, Isolation: f.Isolation, Producer: f.Producer, Visibility: s.Decl.Visibility, Retention: s.Decl.Retention,
			CreatedAt: now, State: StateStaged, Release: f.Release, Image: s.Image, Epoch: epoch}
		if d, keep, err := RetentionDuration(s.Decl.Retention); err == nil && !keep {
			t := now.Add(d)
			r.ExpiresAt = &t
		}
		out = append(out, r)
	}
	return out
}

// CommitAndVerify verifies every staged record of the attempt against its
// stored bytes and commits them. It returns the committed records.
func CommitAndVerify(ctx context.Context, l Ledger, b Blobs, attempt string, epoch int64, pol Policy) (records []Record, resultErr error) {
	defer func() { resultErr = auditIntegrity(ctx, l, attempt, resultErr) }()
	decls, err := l.ArtifactDecls(ctx, attempt)
	if err != nil {
		return nil, err
	}
	staged, err := l.ArtifactStaged(ctx, attempt)
	if err != nil {
		return nil, err
	}
	have := map[int]bool{}
	for _, r := range staged {
		have[r.Decl] = true
		if err := verifyBlob(b, r); err != nil {
			return nil, err
		}
	}
	for _, d := range decls {
		if !have[d.Ord] {
			return nil, failf(ReasonMissing, "declared artifact %q has no collected file", d.Path)
		}
	}
	if len(staged) == 0 {
		return nil, nil
	}
	warn, err := l.ArtifactCommit(ctx, attempt, epoch, staged, pol)
	if err != nil {
		return nil, err
	}
	if warn != "" && pol.Warn != nil {
		pol.Warn(warn)
	}
	for i := range staged {
		staged[i].State = StateCommitted
	}
	return staged, nil
}

// verifyBlob re-hashes the stored bytes of r and compares digest and size.
func verifyBlob(b Blobs, r Record) error {
	realm := cas.ArtifactRealm(r.Project)
	if info, err := b.Stat(realm, r.SHA256); err != nil {
		return failf(ReasonMissingBlob, "%s: the stored bytes are gone: %v", r.Name, err)
	} else if info.Size() != r.Size {
		return failf(ReasonDigest, "%s: stored size %d B differs from the recorded %d B", r.Name, info.Size(), r.Size)
	}
	rc, err := b.Open(realm, r.SHA256) // hashes into a snapshot first: a differing digest is an error here
	if err != nil {
		return errors.Join(failf(ReasonDigest, "%s: the stored bytes do not verify: %v", r.Name, err), err)
	}
	defer func() { _ = rc.Close() }()
	if n, err := io.Copy(io.Discard, rc); err != nil || n != r.Size {
		return failf(ReasonDigest, "%s: the stored bytes read back as %d B (%v)", r.Name, n, err)
	}
	return nil
}
