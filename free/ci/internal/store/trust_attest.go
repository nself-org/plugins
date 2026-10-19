package store

// Purpose: the durable side of runner-key evidence signing (P7-TRUST-20, Epic D13): the replay set of
// accepted (attempt, key id, signature) triples, the revoked evidence keys and the quarantine index of
// runs whose evidence a revoked key signed.
// Inputs:  attempt ids, key ids, signature digests, a node id to revoke.
// Outputs: AttestLedger, the store side of attest.Ledger and attest.RevokeLedger: the attempt facts a
//          verifier compares with, the nodes pinning a key id, the use records a revocation re-evaluates
//          (gates, posted statuses, promoted releases).
// Constraints: expand-only migration set trust-attest, no foreign key into another set; one transaction
//          per mutation; a signature digest already consumed under another attempt is refused (E686); a
//          revoked key stays revoked (the id is derived from the public key, so re-pairing the same key
//          changes nothing); SQL stays in this file.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

//go:embed migrations/trust-attest/*.sql
var trustAttestMigrations embed.FS

func init() {
	if err := Register("trust-attest", trustAttestMigrations); err != nil {
		panic(err)
	}
}

// AttestLedger implements attest.Ledger and attest.RevokeLedger over a Store.
type AttestLedger struct{ S *Store }

var (
	_ attest.Ledger       = AttestLedger{}
	_ attest.RevokeLedger = AttestLedger{}
)

// AttemptFacts reads the lease holder (the live lease, else the admission record that survives the lease)
// and the pipeline and job the attempt belongs to; ok is false for an unknown attempt or one with no
// recorded holder. RepoID is derived the way the run's evidence derives it (run.shortDigest of the
// project: the first 8 bytes of its sha256 in hex); TestAttestRepoIDMatchesTheRunDerivation pins it.
func (l AttestLedger) AttemptFacts(ctx context.Context, attemptID string) (a attest.AttemptFacts, ok bool, err error) {
	var project string
	err = l.S.readers.QueryRowContext(ctx, `SELECT coalesce((SELECT runner_id FROM lease WHERE attempt_id=a.id),(SELECT runner_id FROM sched_admitted WHERE attempt_id=a.id),''),
 p.id,p.project,p.revision,p.source_trust,p.policy_digest,p.selection_mode,j.name
 FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.id=?`, attemptID).
		Scan(&a.RunnerID, &a.RunID, &project, &a.Revision, &a.SourceTrust, &a.PolicyDigest, &a.SelectionMode, &a.Job)
	if errors.Is(err, sql.ErrNoRows) {
		return attest.AttemptFacts{}, false, nil
	}
	sum := sha256.Sum256([]byte(project))
	a.RepoID = hex.EncodeToString(sum[:8])
	return a, err == nil && a.RunnerID != "", err
}

// NodesByKey lists the nodes that pin the 16-hex key id (a pending pin has no key and is skipped; a row
// whose key does not hash to the id is skipped; a node revoked in the registry or by RevokeNode is skipped), each with the signing_eligible fact of its registry record.
func (l AttestLedger) NodesByKey(ctx context.Context, nodeKeyID string) ([]attest.KeyNode, error) {
	rows, err := l.S.readers.QueryContext(ctx, `SELECT k.node_id,k.public,coalesce(c.doc_json,'') FROM pki_node_key k LEFT JOIN capability_snapshot c ON c.node_id=k.node_id LEFT JOIN node n ON n.id=k.node_id
 WHERE k.key_id=? AND k.public<>'' AND coalesce(n.state,'')<>'revoked' AND NOT EXISTS (SELECT 1 FROM attest_revoked_node r WHERE r.node_id=k.node_id) ORDER BY k.node_id`, nodeKeyID)
	all, err := pkiCollect(rows, err, func(r *sql.Rows) (n attest.KeyNode, e error) {
		var public string
		var capability []byte
		if e = r.Scan(&n.NodeID, &public, &capability); e != nil {
			return n, e
		}
		if raw, derr := base64.StdEncoding.Strict().DecodeString(public); derr == nil && len(raw) == ed25519.PublicKeySize {
			if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:])[:16] == nodeKeyID {
				n.Public = raw
			}
		}
		n.SigningEligible = signingEligible(capability)
		return n, nil
	})
	out := all[:0]
	for _, n := range all {
		if n.Public != nil {
			out = append(out, n)
		}
	}
	return out, err
}

// signingEligible reads separation.signing_eligible from a capability document; anything unreadable is false.
func signingEligible(capability []byte) bool {
	var c struct {
		Separation struct {
			SigningEligible struct {
				Value *bool `json:"value"`
			} `json:"signing_eligible"`
		} `json:"separation"`
	}
	if json.Unmarshal(capability, &c) != nil {
		return false
	}
	v := c.Separation.SigningEligible.Value
	return v != nil && *v
}

// Replayed reports whether (keyID, sigDigest) was consumed under an attempt other than attemptID.
func (l AttestLedger) Replayed(ctx context.Context, attemptID, keyID, sigDigest string) (bool, error) {
	var n int
	err := l.S.readers.QueryRowContext(ctx, `SELECT count(*) FROM attest_use WHERE key_id=? AND sig_digest=? AND attempt_id<>?`, keyID, sigDigest, attemptID).Scan(&n)
	return n > 0, err
}

// Consume records u. The same triple again is a no-op (a document verified twice); the same signature
// under another attempt is attest.ErrReplay.
func (l AttestLedger) Consume(ctx context.Context, u attest.Use) error {
	if u.AttemptID == "" || u.KeyID == "" || u.SigDigest == "" || u.NodeID == "" {
		return coded("E607", "an evidence use needs an attempt, a key, a signature digest and a node")
	}
	s := l.S
	return s.write(ctx, func(tx *sql.Tx) error {
		// A revocation that committed after the verifier's check wins: the use is refused here, so every
		// use row that exists was written before the key was revoked and is quarantined with it.
		var revoked int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM attest_revoked WHERE key_id=?`, u.KeyID).Scan(&revoked); err != nil {
			return err
		}
		if revoked > 0 {
			return attest.ErrRevoked
		}
		var other int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM attest_use WHERE key_id=? AND sig_digest=? AND attempt_id<>?`, u.KeyID, u.SigDigest, u.AttemptID).Scan(&other); err != nil {
			return err
		}
		if other > 0 {
			return attest.ErrReplay
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO attest_use(attempt_id,key_id,sig_digest,node_id,run_id,repo_id,revision,job,used_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			u.AttemptID, u.KeyID, u.SigDigest, u.NodeID, u.RunID, u.RepoID, u.Revision, u.Job, s.now().UnixNano())
		return err
	})
}

// Revoked reports whether the evidence key id (ci-node-<16 hex>) is revoked.
func (l AttestLedger) Revoked(ctx context.Context, keyID string) (bool, error) {
	var n int
	err := l.S.readers.QueryRowContext(ctx, `SELECT count(*) FROM attest_revoked WHERE key_id=?`, keyID).Scan(&n)
	return n > 0, err
}

// Quarantined reports whether a revoked key signed evidence of the run.
func (l AttestLedger) Quarantined(ctx context.Context, runID string) (bool, error) {
	var n int
	err := l.S.readers.QueryRowContext(ctx, `SELECT count(*) FROM attest_quarantine WHERE run_id=?`, runID).Scan(&n)
	return n > 0, err
}

// RevokeNode marks the node revoked and revokes, in one transaction, every evidence key it pins (a
// pending pin included) or ever used; it quarantines the runs those keys signed and returns the
// re-evaluation set. A node with no key yet is still marked, so a key pinned for it later is refused.
func (l AttestLedger) RevokeNode(ctx context.Context, nodeID, reason string) (attest.Affected, error) {
	var out attest.Affected
	if nodeID == "" {
		return out, coded("E607", "a revocation needs a node")
	}
	s := l.S
	err := s.write(ctx, func(tx *sql.Tx) error {
		out = attest.Affected{}
		now := s.now().UnixNano()
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO attest_revoked_node(node_id,reason,revoked_at) VALUES(?,?,?)`, nodeID, reason, now); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT 'ci-node-'||key_id FROM pki_node_key WHERE node_id=? UNION SELECT key_id FROM attest_use WHERE node_id=? UNION SELECT key_id FROM attest_revoked WHERE node_id=? ORDER BY 1`, nodeID, nodeID, nodeID)
		if out.Keys, err = pkiCollect(rows, err, func(r *sql.Rows) (k string, e error) { return k, r.Scan(&k) }); err != nil {
			return err
		}
		for _, k := range out.Keys {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO attest_revoked(key_id,node_id,reason,revoked_at) VALUES(?,?,?,?)`, k, nodeID, reason, now); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO attest_quarantine(run_id,key_id,quarantined_at) SELECT DISTINCT run_id,key_id,? FROM attest_use WHERE key_id=? AND run_id<>''`, now, k); err != nil {
				return err
			}
		}
		if len(out.Keys) == 0 {
			return nil
		}
		return affected(ctx, tx, out.Keys, &out)
	})
	return out, err
}

func affected(ctx context.Context, tx *sql.Tx, keyIDs []string, out *attest.Affected) (err error) {
	ids, _ := json.Marshal(keyIDs)
	gates, err := tx.QueryContext(ctx, `SELECT DISTINCT repo_id,revision,job FROM attest_use WHERE key_id IN (SELECT value FROM json_each(?)) ORDER BY repo_id,revision,job`, string(ids))
	if out.Gates, err = pkiCollect(gates, err, func(r *sql.Rows) (g attest.GateRef, e error) { return g, r.Scan(&g.RepoID, &g.Revision, &g.Job) }); err != nil {
		return err
	}
	posted, err := tx.QueryContext(ctx, `SELECT DISTINCT revision,context FROM outbox WHERE state='posted' AND revision IN (SELECT revision FROM attest_use WHERE key_id IN (SELECT value FROM json_each(?))) ORDER BY revision,context`, string(ids))
	if out.Statuses, err = pkiCollect(posted, err, func(r *sql.Rows) (st attest.StatusKey, e error) { return st, r.Scan(&st.Revision, &st.Context) }); err != nil {
		return err
	}
	rel, err := tx.QueryContext(ctx, `SELECT run_id,project,revision,manifest_sha256 FROM cache_release WHERE state='published' AND run_id IN (SELECT run_id FROM attest_quarantine WHERE key_id IN (SELECT value FROM json_each(?))) ORDER BY run_id`, string(ids))
	out.Releases, err = pkiCollect(rel, err, func(r *sql.Rows) (a attest.ReleaseRef, e error) {
		return a, r.Scan(&a.RunID, &a.Project, &a.Revision, &a.ManifestSHA256)
	})
	return err
}
