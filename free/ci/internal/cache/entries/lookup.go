package entries

// Purpose: the read side (Epic D3): Lookup by exact key, integrity re-hash of
// every blob through cas, and Verify over all entries.
// Inputs: the namespace being read, its key and the authz access mode.
// Outputs: the Manifest and a hit flag; every refusal is a miss, never a
// fallback.
// Constraints: no restore-key or prefix fallback, no other key, no other
// namespace. The stored namespace is compared byte for byte (a mismatch is a
// miss plus an audit row). ro-snapshot serves only authz.SnapshotEligible
// entries. A corrupt or missing blob drops the entry (miss plus audit row).

import (
	"bytes"
	"context"
	"errors"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

// intact re-hashes every blob (cas.Open verifies the digest before serving). A
// non-empty reason means the entry is unusable; err is an I/O failure.
func (s *Store) intact(m Manifest) (reason string, err error) {
	realm := cas.CacheRealm(string(m.Namespace))
	for _, b := range m.Blobs {
		info, e := s.blobs.Stat(realm, b.SHA256)
		if e == nil && info.Size() != b.Size {
			return "size_mismatch", nil
		}
		if e == nil {
			var rc interface{ Close() error }
			if rc, e = s.blobs.Open(realm, b.SHA256); e == nil {
				e = rc.Close()
			}
		}
		switch {
		case e == nil:
		case errors.Is(e, os.ErrNotExist):
			return "missing", nil
		case errors.Is(e, &cas.Error{}), errors.Is(e, cas.ErrInvalid):
			return "corrupt", nil
		default:
			return "", e
		}
	}
	return "", nil
}

// Lookup returns the entry stored under key for ns when access allows it and
// every blob is intact.
func (s *Store) Lookup(ctx context.Context, ns authz.Namespace, key string, access authz.Mode) (Manifest, bool, error) {
	if (access != authz.ModeRWOwn && access != authz.ModeROSnapshot) || !ns.Valid() || !hexKey.MatchString(key) {
		return Manifest{}, false, nil
	}
	list, err := s.be.CacheEntries(ctx, key)
	if err != nil || len(list) != 1 {
		return Manifest{}, false, err
	}
	m := list[0]
	if !bytes.Equal(m.Namespace, ns.Canonical()) {
		return Manifest{}, false, s.be.AppendAudit(ctx, "cache", "cache.lookup.namespace_mismatch", key, "")
	}
	if access == authz.ModeROSnapshot && !s.snapshotOK(m) {
		return Manifest{}, false, nil
	}
	reason, err := s.intact(m)
	if err != nil {
		return Manifest{}, false, err
	}
	if reason != "" {
		return Manifest{}, false, s.be.CacheDrop(ctx, key, reason)
	}
	return m, true, s.be.CacheTouch(ctx, key)
}

// snapshotOK applies authz.SnapshotEligible to the facts stored with m.
func (s *Store) snapshotOK(m Manifest) bool {
	ns, err := authz.DecodeNamespace(m.Namespace)
	if err != nil || s.opt.Project == nil {
		return false
	}
	return authz.SnapshotEligible(authz.EntryFacts{Namespace: ns, NoSecretsReceived: !m.Writer.SecretsReceived,
		NoPrivateDeps: !m.Writer.PrivateDeps, ScannedClean: m.Writer.ScannedClean}, s.opt.Project(ns.Project))
}

// VerifyReport counts a Verify pass.
type VerifyReport struct{ Checked, Dropped int }

// Verify re-hashes every entry and drops the unusable ones.
func (s *Store) Verify(ctx context.Context) (VerifyReport, error) {
	var r VerifyReport
	list, err := s.be.CacheEntries(ctx, "")
	for _, m := range list {
		if err != nil {
			break
		}
		r.Checked++
		var reason string
		if reason, err = s.intact(m); err == nil && reason != "" {
			r.Dropped++
			err = s.be.CacheDrop(ctx, m.Key, reason)
		}
	}
	return r, err
}
