package store

// Purpose: the durable side of the coordinator PKI and pairing (P7-NODE-07, migration set node-pki):
//          sealed CA keys, issued certificates, the deny list, pairing offers, pinned node keys, token keys.
// Constraints: the store never sees a KEK or an unsealed key, and never a pairing code (only its hash);
//          multi-statement changes are ONE transaction; a deny-list row is never removed; an offer is
//          claimed in one transaction so two redemptions of one code cannot both win; audit details
//          carry ids and counts only.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"strconv"
)

//go:embed migrations/node-pki/*.sql
var pkiMigrations embed.FS

func init() {
	if err := Register("node-pki", pkiMigrations); err != nil {
		panic(err)
	}
}

// PKIAuthority is a sealed CA (root, node, user, coordinator).
type PKIAuthority struct {
	Kind               string
	CertDER, KeySealed []byte
	NotAfter           int64
}

// PKICert is an issued certificate; CapAfter > 0 is the hard end of an ephemeral lineage (no renewal past it).
type PKICert struct {
	Serial, Kind, NodeID, Subject, OfferID  string
	NotBefore, NotAfter, CapAfter, IssuedAt int64
}

// PKIDeniedRow is a deny-list entry; RowID grows, so a reader asks "after N".
type PKIDeniedRow struct {
	RowID          int64
	Serial, NodeID string
}

// PKIOffer is a pairing offer; CodeHash is SHA-256 of the code.
type PKIOffer struct {
	ID, Label, Intent, NodeID, Serial                   string
	CodeHash                                            []byte
	CertTTL, ExpiresAt, CreatedAt, Attempts, RedeemedAt int64
	Locked, Withdrawn, Tainted                          bool
}

// CodeClaim says what presenting a code did.
type CodeClaim string

const (
	CodeOK        CodeClaim = "ok"
	CodeUnknown   CodeClaim = "invalid"
	CodeExpired   CodeClaim = "expired"
	CodeLocked    CodeClaim = "locked"
	CodeWithdrawn CodeClaim = "withdrawn"
	CodeReused    CodeClaim = "reused"
	CodeNoIntent  CodeClaim = "no_intent"
)

const MaxCodeAttempts = 5

// PKIAuthoritiesInsert stores a full sealed hierarchy unless a root exists (racing bootstraps keep one).
func (s *Store) PKIAuthoritiesInsert(ctx context.Context, as []PKIAuthority) (inserted bool, err error) {
	err = s.write(ctx, func(tx *sql.Tx) (e error) {
		var n int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM pki_authority`).Scan(&n); e != nil || n > 0 {
			return e
		}
		for _, a := range as {
			if _, e = tx.ExecContext(ctx, `INSERT INTO pki_authority(kind,cert_der,key_sealed,not_after,created_at) VALUES(?,?,?,?,?)`, a.Kind, a.CertDER, a.KeySealed, a.NotAfter, s.now().UnixNano()); e != nil {
				return e
			}
		}
		inserted = true
		return nil
	})
	return inserted && err == nil, err
}

// PKIAuthorities returns every stored authority.
func (s *Store) PKIAuthorities(ctx context.Context) ([]PKIAuthority, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT kind,cert_der,key_sealed,not_after FROM pki_authority ORDER BY kind`)
	return pkiCollect(rows, err, func(r *sql.Rows) (a PKIAuthority, e error) {
		return a, r.Scan(&a.Kind, &a.CertDER, &a.KeySealed, &a.NotAfter)
	})
}

func (s *Store) PKICertPut(ctx context.Context, c PKICert) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO pki_cert(serial,kind,node_id,subject,offer_id,not_before,not_after,cap_after,issued_at) VALUES(?,?,?,?,?,?,?,?,?)`, c.Serial, c.Kind, c.NodeID, c.Subject, c.OfferID, c.NotBefore, c.NotAfter, c.CapAfter, s.now().UnixNano())
		return err
	})
}

const certCols = `serial,kind,node_id,subject,offer_id,not_before,not_after,cap_after,issued_at`

func scanCert(r interface{ Scan(...any) error }) (c PKICert, err error) {
	err = r.Scan(&c.Serial, &c.Kind, &c.NodeID, &c.Subject, &c.OfferID, &c.NotBefore, &c.NotAfter, &c.CapAfter, &c.IssuedAt)
	return c, err
}

// PKICertGet returns one certificate by serial (sql.ErrNoRows when unknown).
func (s *Store) PKICertGet(ctx context.Context, serial string) (PKICert, error) {
	return scanCert(s.readers.QueryRowContext(ctx, `SELECT `+certCols+` FROM pki_cert WHERE serial=?`, serial))
}

// PKICertFind is PKICertGet with found=false for an unknown serial instead of an error.
func (s *Store) PKICertFind(ctx context.Context, serial string) (c PKICert, found bool, err error) {
	c, err = s.PKICertGet(ctx, serial)
	if errors.Is(err, sql.ErrNoRows) {
		return PKICert{}, false, nil
	}
	return c, err == nil, err
}

func (s *Store) PKICertsOfNode(ctx context.Context, nodeID string) ([]PKICert, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT `+certCols+` FROM pki_cert WHERE node_id=? AND kind='node' ORDER BY issued_at,serial`, nodeID)
	return pkiCollect(rows, err, func(r *sql.Rows) (PKICert, error) { return scanCert(r) })
}

// PKIDeny denies serials and audits it in one transaction; it returns how many were new.
func (s *Store) PKIDeny(ctx context.Context, actor, nodeID, reason string, serials ...string) (added int64, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		added = 0
		for _, sn := range serials {
			res, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO pki_denied(serial,node_id,reason,denied_at) VALUES(?,?,?,?)`, sn, nodeID, reason, s.now().UnixNano())
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			added += n
		}
		return s.appendAudit(ctx, tx, actor, "pki.deny", nodeID, "serials="+strconv.FormatInt(added, 10)+" reason="+reason)
	})
	return added, err
}

func (s *Store) PKIDenied(ctx context.Context, rowID int64) ([]PKIDeniedRow, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT rowid,serial,node_id FROM pki_denied WHERE rowid>? ORDER BY rowid`, rowID)
	return pkiCollect(rows, err, func(r *sql.Rows) (d PKIDeniedRow, e error) { return d, r.Scan(&d.RowID, &d.Serial, &d.NodeID) })
}

// PKIIsDenied reads the deny list directly (handshakes never trust a cache).
func (s *Store) PKIIsDenied(ctx context.Context, serial string) (bool, error) {
	var n int
	err := s.readers.QueryRowContext(ctx, `SELECT count(*) FROM pki_denied WHERE serial=?`, serial).Scan(&n)
	return n > 0, err
}

// PKINodeKeyGet returns the pin (ok false when none).
func (s *Store) PKINodeKeyGet(ctx context.Context, nodeID string) (keyID, public string, ok bool, err error) {
	err = s.readers.QueryRowContext(ctx, `SELECT key_id,public FROM pki_node_key WHERE node_id=?`, nodeID).Scan(&keyID, &public)
	return keyID, public, err == nil, ignoreNoRows(err)
}

// PKINodeKeyRotate sets a pending pin (key id only) and audits it (rotate --node-key).
func (s *Store) PKINodeKeyRotate(ctx context.Context, actor, nodeID, keyID string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO pki_node_key(node_id,key_id,public,pinned_at) VALUES(?,?,'',?)`, nodeID, keyID, s.now().UnixNano()); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, actor, "pki.node_key.rotate", nodeID, "new_key_id="+keyID)
	})
}

func (s *Store) PKIOfferPut(ctx context.Context, o PKIOffer) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO pki_offer(id,code_hash,label,intent,cert_ttl_ns,expires_at,created_at) VALUES(?,?,?,?,?,?,?)`, o.ID, o.CodeHash, o.Label, o.Intent, o.CertTTL, o.ExpiresAt, s.now().UnixNano())
		return err
	})
}

const offerCols = `id,code_hash,label,intent,cert_ttl_ns,expires_at,created_at,attempts,locked,withdrawn,tainted,redeemed_at,node_id,serial`

func scanOffer(r interface{ Scan(...any) error }) (o PKIOffer, err error) {
	err = r.Scan(&o.ID, &o.CodeHash, &o.Label, &o.Intent, &o.CertTTL, &o.ExpiresAt, &o.CreatedAt, &o.Attempts, &o.Locked, &o.Withdrawn, &o.Tainted, &o.RedeemedAt, &o.NodeID, &o.Serial)
	return o, err
}

// PKIOfferClaim presents a code hash. A live offer is consumed (CodeOK). A redeemed code taints its offer
// (CodeReused: the caller revokes). An unknown code is a wrong attempt on every live offer (locks at
// MaxCodeAttempts). intentOnly (the burst listener) leaves an offer without a live intent untouched, and
// a wrong code there counts against intent-bound offers only.
func (s *Store) PKIOfferClaim(ctx context.Context, hash []byte, intentOnly bool) (o PKIOffer, res CodeClaim, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		var err error
		o, err = scanOffer(tx.QueryRowContext(ctx, `SELECT `+offerCols+` FROM pki_offer WHERE code_hash=?`, hash))
		if errors.Is(err, sql.ErrNoRows) {
			res = CodeUnknown
			// A wrong code on the burst listener (internet-facing) counts against intent-bound offers only, so
			// probing it cannot lock the operator's foreground offers.
			_, err = tx.ExecContext(ctx, `UPDATE pki_offer SET attempts=attempts+1, locked=CASE WHEN attempts+1>=? THEN 1 ELSE locked END WHERE redeemed_at=0 AND withdrawn=0 AND locked=0 AND expires_at>? AND (?=0 OR intent<>'')`, MaxCodeAttempts, now, intentOnly)
			return err
		}
		if err != nil {
			return err
		}
		switch {
		case intentOnly && (o.Intent == "" || o.Withdrawn): // the burst listener serves live intents only
			res = CodeNoIntent
		case o.RedeemedAt > 0:
			res = CodeReused
			_, err = tx.ExecContext(ctx, `UPDATE pki_offer SET tainted=1 WHERE id=?`, o.ID)
		case o.Withdrawn:
			res = CodeWithdrawn
		case o.Locked:
			res = CodeLocked
		case o.ExpiresAt <= now:
			res = CodeExpired
		default:
			res = CodeOK
			_, err = tx.ExecContext(ctx, `UPDATE pki_offer SET redeemed_at=? WHERE id=?`, now, o.ID)
		}
		return err
	})
	return o, res, err
}

// PKIOfferBind records the node and certificate of an approved redemption; tainted means the code was
// presented again or the offer withdrawn meanwhile (the caller revokes at once).
func (s *Store) PKIOfferBind(ctx context.Context, id, nodeID, serial string) (tainted bool, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, `UPDATE pki_offer SET node_id=?,serial=? WHERE id=?`, nodeID, serial, id); e != nil {
			return e
		}
		return tx.QueryRowContext(ctx, `SELECT tainted OR withdrawn FROM pki_offer WHERE id=?`, id).Scan(&tainted)
	})
	return tainted, err
}

// PKIOfferWithdraw ends the offer with this code hash and returns it (sql.ErrNoRows when unknown).
func (s *Store) PKIOfferWithdraw(ctx context.Context, hash []byte) (o PKIOffer, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		var e error
		if o, e = scanOffer(tx.QueryRowContext(ctx, `SELECT `+offerCols+` FROM pki_offer WHERE code_hash=?`, hash)); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE pki_offer SET withdrawn=1 WHERE id=?`, o.ID)
		return e
	})
	return o, err
}

// PKIOffersLive lists offers that can still be redeemed now; intentOnly keeps those bound to an intent.
func (s *Store) PKIOffersLive(ctx context.Context, intentOnly bool) ([]PKIOffer, error) {
	q := `SELECT ` + offerCols + ` FROM pki_offer WHERE redeemed_at=0 AND withdrawn=0 AND locked=0 AND expires_at>?`
	if intentOnly {
		q += ` AND intent<>''`
	}
	rows, err := s.readers.QueryContext(ctx, q+` ORDER BY created_at,id`, s.now().UnixNano())
	return pkiCollect(rows, err, func(r *sql.Rows) (PKIOffer, error) { return scanOffer(r) })
}

func pkiCollect[T any](rows *sql.Rows, err error, scan func(*sql.Rows) (T, error)) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func ignoreNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
