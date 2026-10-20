package remote

// Purpose: turn an agent's `artifact-ref{lease_id, path, sha256, size}` into a
// staged artifact record (Epic CACHE D6, D7).
// Inputs:  the ref, the attempt's declarations, the blob realm.
// Outputs: one staged record in the store (state staged, the attempt's epoch);
// E714 stale_epoch, E712 not_declared / missing_blob / digest_mismatch /
// duplicate_name.
// Constraints: the path is judged by the same sanitiser the collector uses
// (authz.ArtifactPath); the record's project, run, job and revision come from
// the store; the blob must already be in the project's realm at the stated size
// (CommitAndVerify re-hashes it later); the carriers announce blobs of one byte
// or more, so a ref for an empty file is refused (the agent reports it first);
// nothing the node sends is trusted beyond the digest it proves.

import (
	"context"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ArtifactRef records one delivered artifact (transport.Sink.ArtifactRef).
func (l *Lease) ArtifactRef(ctx context.Context, ref protocol.ArtifactRef) error {
	if ref.LeaseID != l.b.LeaseID {
		return transfer.Failf(transfer.ReasonProtocol, "an artifact-ref names lease %q, not this one", ref.LeaseID)
	}
	if _, err := l.declare(ctx, ref.SHA256); err != nil {
		return err
	}
	f, err := l.factsOnce(ctx)
	if err != nil {
		return err
	}
	if !sha256Hex.MatchString(ref.SHA256) || ref.Size < 1 {
		return transfer.Failf(transfer.ReasonProtocol, "an artifact-ref needs a sha256 and a size of at least one byte (the carriers do not transfer empty blobs)")
	}
	rel, d, name, err := l.resolve(ref.Path)
	if err != nil {
		return err
	}
	realm := cas.ArtifactRealm(f.Project)
	if err = l.hold(ctx, realm, ref); err != nil {
		return err
	}
	// every record counts against the store, however many refs name one blob
	if left, berr := l.budget(ctx, f.Project, ref.SHA256); berr != nil {
		return berr
	} else if ref.Size > left {
		return transfer.Failf(transfer.ReasonSizeCap, "%s: a record of %d B would take the project past the artifact store cap (%d B left)", rel, ref.Size, left)
	}
	st := records.Staged{Decl: d, Name: name, Rel: rel, SHA256: ref.SHA256, Size: ref.Size}
	if d.Kind == records.KindImage {
		rc, oerr := l.h.Blobs.Open(realm, ref.SHA256)
		if oerr != nil {
			return records.Failf(records.ReasonMissingBlob, "%s: %v", rel, oerr)
		}
		defer func() { _ = rc.Close() }()
		if st.Image, err = records.ReadImageLayout(rc); err != nil {
			return err
		}
		st.Image.Ref = d.ImageRef
	}
	epoch, err := l.guard(ctx, ref.SHA256) // the epoch may have moved while the blob was read
	if err != nil {
		return err
	}
	if f.Platform == "" {
		f.Platform = "unknown"
	}
	if f.Isolation = string(l.b.Job.Isolation); f.Isolation == "" {
		f.Isolation = "process"
	}
	f.Producer = "nself-ci/" + f.Job
	if err = l.h.Ledger.ArtifactStage(ctx, l.b.AttemptID, epoch, records.ToRecords(l.b.AttemptID, f, epoch, []records.Staged{st}, l.h.now())); err != nil {
		return err
	}
	l.claim(rel, name)
	l.recorded(ref.SHA256, ref.Size)
	return nil
}

// resolve judges the path and finds the declaration and record name for it.
func (l *Lease) resolve(p string) (string, records.Decl, string, error) {
	lim := authz.Limits{MaxFileBytes: math.MaxInt64 / 4, MaxTotalBytes: math.MaxInt64 / 4}
	rel, err := authz.ArtifactPath(authz.Entry{Name: p, Mode: 0o644}, lim)
	if err != nil || rel != p || strings.Contains(p, "\\") {
		return "", records.Decl{}, "", records.Failf(ReasonNotDeclared, "the artifact path is not a clean relative path inside the workdir")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	nth := l.refs[p]
	d, name, ok := Select(l.decls, p, nth)
	if !ok && nth > 0 {
		return "", records.Decl{}, "", records.Failf(records.ReasonDuplicate, "%q was already delivered", p)
	}
	if !ok {
		return "", records.Decl{}, "", records.Failf(ReasonNotDeclared, "no declaration of this attempt selects %q", p)
	}
	if l.names[name] {
		return "", records.Decl{}, "", records.Failf(records.ReasonDuplicate, "two refs produce the artifact name %q", name)
	}
	return rel, d, name, nil
}

// claim remembers a staged ref, so a second ref for the same name or the same path moves on.
func (l *Lease) claim(p, name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.names == nil {
		l.names, l.refs = map[string]bool{}, map[string]int{}
	}
	l.names[name] = true
	l.refs[p]++
}

// hold checks the blob a ref names is in the realm at the stated size.
func (l *Lease) hold(ctx context.Context, realm cas.Realm, ref protocol.ArtifactRef) error {
	info, err := l.h.Blobs.Stat(realm, ref.SHA256)
	if err != nil {
		if os.IsNotExist(err) {
			return records.Failf(records.ReasonMissingBlob, "%s: the blob was never delivered", ref.Path)
		}
		return err
	}
	if info.Size() != ref.Size {
		return transfer.Failf(transfer.ReasonDigest, "%s: the stored blob is %d B, the ref says %d B", ref.Path, info.Size(), ref.Size)
	}
	if l.verified(ref.SHA256) {
		return nil // published or re-verified by this lease a moment ago
	}
	// restart the blob's sweep grace: the record that references it is written a moment from now
	if err = l.h.Blobs.Refresh(realm, ref.SHA256); err != nil {
		return records.Failf(records.ReasonMissingBlob, "%s: the stored blob does not verify: %v", ref.Path, err)
	}
	return nil
}
