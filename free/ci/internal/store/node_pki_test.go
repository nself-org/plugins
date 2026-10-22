package store

// Purpose: tests of the node-pki store set (P7-NODE-07): migration, authorities, certificates, the deny
//          list, node key pins, token keys and the pairing-offer state machine.
// Inputs:  a real SQLite store in t.TempDir() with an injected clock.
// Constraints: no sleeps; the clock is moved by the test.

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

type pkiClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *pkiClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *pkiClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func pkiTestStore(t *testing.T) (*Store, *pkiClock) {
	t.Helper()
	clk := &pkiClock{t: time.Unix(1_800_000_000, 0)}
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{Clock: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, clk
}

func pkiOffer(id string, hash byte, ttl time.Duration, clk *pkiClock) PKIOffer {
	return PKIOffer{ID: id, CodeHash: bytes.Repeat([]byte{hash}, 32), Label: "lbl-" + id, CertTTL: int64(30 * time.Minute), ExpiresAt: clk.now().Add(ttl).UnixNano()}
}

func pkiHash(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func pkiAudit(t *testing.T, s *Store, action string) (rows [][3]string) {
	t.Helper()
	r, err := s.readers.Query(`SELECT actor,target,detail FROM audit WHERE action=? ORDER BY seq`, action)
	if err != nil {
		t.Fatal(err)
	}
	defer func(cleanup func() error) { _ = cleanup() }(r.Close)
	for r.Next() {
		var a [3]string
		if err = r.Scan(&a[0], &a[1], &a[2]); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, a)
	}
	return rows
}

func TestPKIMigrationSetRegistered(t *testing.T) {
	s, _ := pkiTestStore(t)
	var n int
	if err := s.readers.QueryRow(`SELECT count(*) FROM migration_log WHERE set_name='node-pki' AND filename='0001_pki.sql'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("node-pki migration not applied: %d %v", n, err)
	}
	for _, tbl := range []string{"pki_authority", "pki_cert", "pki_denied", "pki_offer", "pki_node_key", "pki_token_key"} {
		if err := s.readers.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s missing: %d %v", tbl, n, err)
		}
	}
	if err := Register("node-pki", pkiMigrations); err == nil {
		t.Fatal("node-pki registered twice")
	}
}

func TestPKIAuthoritiesInsertIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	first := []PKIAuthority{
		{Kind: "root", CertDER: []byte{1}, KeySealed: []byte{2}, NotAfter: 11},
		{Kind: "node", CertDER: []byte{3}, KeySealed: []byte{4}, NotAfter: 12},
	}
	got, err := s.PKIAuthorities(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("fresh store: %v %v", got, err)
	}
	ok, err := s.PKIAuthoritiesInsert(ctx, first)
	if err != nil || !ok {
		t.Fatalf("first insert: %v %v", ok, err)
	}
	ok, err = s.PKIAuthoritiesInsert(ctx, []PKIAuthority{{Kind: "root", CertDER: []byte{9}, KeySealed: []byte{9}, NotAfter: 99}, {Kind: "user", CertDER: []byte{9}, KeySealed: []byte{9}}})
	if err != nil || ok {
		t.Fatalf("second insert should be a no-op: %v %v", ok, err)
	}
	got, err = s.PKIAuthorities(ctx)
	if err != nil || !reflect.DeepEqual(got, []PKIAuthority{first[1], first[0]}) { // ORDER BY kind: node, root
		t.Fatalf("authorities: %+v %v", got, err)
	}
	var created int64
	if err = s.readers.QueryRow(`SELECT created_at FROM pki_authority WHERE kind='root'`).Scan(&created); err != nil || created != clk.now().UnixNano() {
		t.Fatalf("created_at %d %v", created, err)
	}
	// a failing row rolls the whole set back and reports not inserted
	s2, _ := pkiTestStore(t)
	ok, err = s2.PKIAuthoritiesInsert(ctx, []PKIAuthority{first[0], first[0]})
	if err == nil || ok {
		t.Fatalf("duplicate kind: ok=%v err=%v", ok, err)
	}
	if got, _ = s2.PKIAuthorities(ctx); len(got) != 0 {
		t.Fatalf("partial insert survived: %+v", got)
	}
}

func TestPKICertPutGet(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	c1 := PKICert{Serial: "aa", Kind: "node", NodeID: "n1", Subject: "nself-ci:node/n1", OfferID: "o1", NotBefore: 1, NotAfter: 2, CapAfter: 3}
	if err := s.PKICertPut(ctx, c1); err != nil {
		t.Fatal(err)
	}
	clk.advance(time.Second)
	c2 := PKICert{Serial: "01", Kind: "node", NodeID: "n1", Subject: "nself-ci:node/n1", NotBefore: 4, NotAfter: 5}
	if err := s.PKICertPut(ctx, c2); err != nil {
		t.Fatal(err)
	}
	if err := s.PKICertPut(ctx, PKICert{Serial: "ff", Kind: "user", NodeID: "u1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PKICertPut(ctx, c1); err == nil {
		t.Fatal("duplicate serial accepted")
	}
	got, err := s.PKICertGet(ctx, "aa")
	c1.IssuedAt = time.Unix(1_800_000_000, 0).UnixNano()
	if err != nil || got != c1 {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err = s.PKICertGet(ctx, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown serial: %v", err)
	}
	list, err := s.PKICertsOfNode(ctx, "n1")
	if err != nil || len(list) != 2 || list[0].Serial != "aa" || list[1].Serial != "01" || list[1].IssuedAt != clk.now().UnixNano() {
		t.Fatalf("of node (issued_at order): %+v %v", list, err)
	}
	if list, err = s.PKICertsOfNode(ctx, "ghost"); err != nil || len(list) != 0 {
		t.Fatalf("ghost: %+v %v", list, err)
	}
	// only node certificates belong to a node: a user certificate with the same id is not listed
	if err = s.PKICertPut(ctx, PKICert{Serial: "uu", Kind: "user", NodeID: "n1"}); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.PKICertsOfNode(ctx, "n1"); len(list) != 2 {
		t.Fatalf("a user certificate was listed under a node id: %+v", list)
	}
	// equal issued_at falls back to serial order
	for _, sn := range []string{"b2", "b1"} {
		if err = s.PKICertPut(ctx, PKICert{Serial: sn, Kind: "node", NodeID: "n2"}); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ = s.PKICertsOfNode(ctx, "n2"); len(list) != 2 || list[0].Serial != "b1" {
		t.Fatalf("tie order: %+v", list)
	}
}

func TestPKIDenyCountsAuditsAndNeverRemoves(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	added, err := s.PKIDeny(ctx, "cli:admin", "n1", "first", "s1", "s2", "s2")
	if err != nil || added != 2 {
		t.Fatalf("added %d %v", added, err)
	}
	added, err = s.PKIDeny(ctx, "cli:admin", "n1", "second", "s2", "s3")
	if err != nil || added != 1 {
		t.Fatalf("added %d %v (s2 already denied)", added, err)
	}
	added, err = s.PKIDeny(ctx, "cli:admin", "n1", "none")
	if err != nil || added != 0 {
		t.Fatalf("empty deny: %d %v", added, err)
	}
	want := [][3]string{{"cli:admin", "n1", "serials=2 reason=first"}, {"cli:admin", "n1", "serials=1 reason=second"}, {"cli:admin", "n1", "serials=0 reason=none"}}
	if got := pkiAudit(t, s, "pki.deny"); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit rows: %v", got)
	}
	rows, err := s.PKIDenied(ctx, 0)
	if err != nil || len(rows) != 3 || rows[0].Serial != "s1" || rows[1].Serial != "s2" || rows[2].Serial != "s3" || rows[0].NodeID != "n1" {
		t.Fatalf("denied rows: %+v %v", rows, err)
	}
	if rows[0].RowID >= rows[1].RowID || rows[1].RowID >= rows[2].RowID {
		t.Fatalf("row ids not increasing: %+v", rows)
	}
	if later, _ := s.PKIDenied(ctx, rows[1].RowID); len(later) != 1 || later[0].Serial != "s3" {
		t.Fatalf("after: %+v", later)
	}
	if later, _ := s.PKIDenied(ctx, rows[2].RowID); len(later) != 0 {
		t.Fatalf("after last: %+v", later)
	}
	var reason string
	var at int64
	if err = s.readers.QueryRow(`SELECT reason,denied_at FROM pki_denied WHERE serial='s2'`).Scan(&reason, &at); err != nil || reason != "first" || at != clk.now().UnixNano() {
		t.Fatalf("the first denial must stay: %q %d %v", reason, at, err)
	}
	for sn, want := range map[string]bool{"s1": true, "s3": true, "zz": false} {
		if got, err := s.PKIIsDenied(ctx, sn); err != nil || got != want {
			t.Fatalf("IsDenied(%s) = %v %v", sn, got, err)
		}
	}
	// an invalid audit (no actor) rolls the deny back
	if _, err = s.PKIDeny(ctx, "", "n1", "x", "s9"); err == nil {
		t.Fatal("deny without actor accepted")
	}
	if got, _ := s.PKIIsDenied(ctx, "s9"); got {
		t.Fatal("deny survived a failed audit")
	}
}

func TestPKINodeKeyPutGetRotate(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	if _, _, ok, err := s.PKINodeKeyGet(ctx, "n1"); ok || err != nil {
		t.Fatalf("no pin: ok=%v err=%v", ok, err)
	}
	if err := s.PKINodeKeyPut(ctx, "n1", "kid1", "pub1"); err != nil {
		t.Fatal(err)
	}
	if k, p, ok, err := s.PKINodeKeyGet(ctx, "n1"); !ok || err != nil || k != "kid1" || p != "pub1" {
		t.Fatalf("get: %q %q %v %v", k, p, ok, err)
	}
	if err := s.PKINodeKeyPut(ctx, "n1", "kid2", "pub2"); err != nil {
		t.Fatal(err)
	}
	if k, p, _, _ := s.PKINodeKeyGet(ctx, "n1"); k != "kid2" || p != "pub2" {
		t.Fatalf("replace: %q %q", k, p)
	}
	clk.advance(time.Minute)
	if err := s.PKINodeKeyRotate(ctx, "cli:admin", "n1", "kid3"); err != nil {
		t.Fatal(err)
	}
	if k, p, ok, _ := s.PKINodeKeyGet(ctx, "n1"); !ok || k != "kid3" || p != "" {
		t.Fatalf("rotate must leave a pending pin: %q %q %v", k, p, ok)
	}
	var at int64
	if err := s.readers.QueryRow(`SELECT pinned_at FROM pki_node_key WHERE node_id='n1'`).Scan(&at); err != nil || at != clk.now().UnixNano() {
		t.Fatalf("pinned_at %d %v", at, err)
	}
	want := [][3]string{{"cli:admin", "n1", "new_key_id=kid3"}}
	if got := pkiAudit(t, s, "pki.node_key.rotate"); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit: %v", got)
	}
	if err := s.PKINodeKeyRotate(ctx, "", "n1", "kid4"); err == nil {
		t.Fatal("rotate without actor accepted")
	}
	if k, _, _, _ := s.PKINodeKeyGet(ctx, "n1"); k != "kid3" {
		t.Fatalf("failed rotate changed the pin: %q", k)
	}
}

func TestPKITokenKeys(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	if got, err := s.PKITokenKeys(ctx); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	if err := s.PKITokenKeyPut(ctx, "old", "p-old", []byte("sealed-old")); err != nil {
		t.Fatal(err)
	}
	clk.advance(time.Second)
	if err := s.PKITokenKeyPut(ctx, "new", "p-new", []byte("sealed-new")); err != nil {
		t.Fatal(err)
	}
	if err := s.PKITokenKeyPut(ctx, "new", "x", nil); err == nil {
		t.Fatal("duplicate kid accepted")
	}
	got, err := s.PKITokenKeys(ctx)
	want := []PKITokenKey{{KID: "new", Public: "p-new", Sealed: []byte("sealed-new")}, {KID: "old", Public: "p-old", Sealed: []byte("sealed-old")}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("keys (newest first): %+v %v", got, err)
	}
	if _, err = s.writer.Exec(`UPDATE pki_token_key SET retired_at=5 WHERE kid='old'`); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.PKITokenKeys(ctx); got[0].Retired || !got[1].Retired {
		t.Fatalf("retired flags: %+v", got)
	}
}

func TestPKIOfferPutAndClaimFields(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	o := pkiOffer("o1", 1, 10*time.Minute, clk)
	o.Intent = "intent-1"
	if err := s.PKIOfferPut(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := s.PKIOfferPut(ctx, PKIOffer{ID: "o2", CodeHash: o.CodeHash}); err == nil {
		t.Fatal("duplicate code hash accepted")
	}
	got, res, err := s.PKIOfferClaim(ctx, o.CodeHash, false)
	want := o
	want.CreatedAt = clk.now().UnixNano()
	if err != nil || res != CodeOK || !reflect.DeepEqual(got, want) {
		t.Fatalf("claim: %v %v\n got %+v\nwant %+v", res, err, got, want)
	}
	var redeemed int64
	if err = s.readers.QueryRow(`SELECT redeemed_at FROM pki_offer WHERE id='o1'`).Scan(&redeemed); err != nil || redeemed != clk.now().UnixNano() {
		t.Fatalf("redeemed_at %d %v", redeemed, err)
	}
}

func TestPKIOfferClaimOutcomes(t *testing.T) {
	ctx := context.Background()
	type tc struct {
		name       string
		setup      func(t *testing.T, s *Store, clk *pkiClock)
		intentOnly bool
		want       CodeClaim
		tainted    bool // after the claim, the offer's tainted flag
		consumed   bool // after the claim, redeemed_at > 0
	}
	put := func(t *testing.T, s *Store, o PKIOffer) {
		t.Helper()
		if err := s.PKIOfferPut(ctx, o); err != nil {
			t.Fatal(err)
		}
	}
	update := func(t *testing.T, s *Store, set string) {
		t.Helper()
		if _, err := s.writer.Exec(`UPDATE pki_offer SET ` + set + ` WHERE id='o1'`); err != nil {
			t.Fatal(err)
		}
	}
	live := func(t *testing.T, s *Store, clk *pkiClock) PKIOffer {
		o := pkiOffer("o1", 1, 10*time.Minute, clk)
		put(t, s, o)
		return o
	}
	tests := []tc{
		{name: "ok", setup: func(t *testing.T, s *Store, clk *pkiClock) { live(t, s, clk) }, want: CodeOK, consumed: true},
		{name: "ok with intent, intentOnly", setup: func(t *testing.T, s *Store, clk *pkiClock) {
			o := pkiOffer("o1", 1, time.Minute, clk)
			o.Intent = "i"
			put(t, s, o)
		}, intentOnly: true, want: CodeOK, consumed: true},
		{name: "expired", setup: func(t *testing.T, s *Store, clk *pkiClock) { live(t, s, clk); clk.advance(11 * time.Minute) }, want: CodeExpired},
		{name: "expired exactly now", setup: func(t *testing.T, s *Store, clk *pkiClock) { live(t, s, clk); clk.advance(10 * time.Minute) }, want: CodeExpired},
		{name: "one ns before expiry", setup: func(t *testing.T, s *Store, clk *pkiClock) { live(t, s, clk); clk.advance(10*time.Minute - 1) }, want: CodeOK, consumed: true},
		{name: "locked", setup: func(t *testing.T, s *Store, clk *pkiClock) { live(t, s, clk); update(t, s, "locked=1") }, want: CodeLocked},
		{name: "withdrawn", setup: func(t *testing.T, s *Store, clk *pkiClock) { live(t, s, clk); update(t, s, "withdrawn=1") }, want: CodeWithdrawn},
		{name: "withdrawn beats locked", setup: func(t *testing.T, s *Store, clk *pkiClock) {
			live(t, s, clk)
			update(t, s, "withdrawn=1, locked=1")
		}, want: CodeWithdrawn},
		{name: "locked beats expired", setup: func(t *testing.T, s *Store, clk *pkiClock) {
			live(t, s, clk)
			update(t, s, "locked=1")
			clk.advance(time.Hour)
		}, want: CodeLocked},
		{name: "reused taints", setup: func(t *testing.T, s *Store, clk *pkiClock) { live(t, s, clk); update(t, s, "redeemed_at=5") }, want: CodeReused, tainted: true, consumed: true},
		{name: "reused beats withdrawn", setup: func(t *testing.T, s *Store, clk *pkiClock) {
			live(t, s, clk)
			update(t, s, "redeemed_at=5, withdrawn=1")
		}, want: CodeReused, tainted: true, consumed: true},
		{name: "no intent leaves the offer alone", setup: func(t *testing.T, s *Store, clk *pkiClock) { live(t, s, clk) }, intentOnly: true, want: CodeNoIntent},
		{name: "withdrawn intent", setup: func(t *testing.T, s *Store, clk *pkiClock) {
			o := pkiOffer("o1", 1, time.Minute, clk)
			o.Intent = "i"
			put(t, s, o)
			update(t, s, "withdrawn=1")
		}, intentOnly: true, want: CodeNoIntent},
		{name: "redeemed without intent is not tainted by intentOnly", setup: func(t *testing.T, s *Store, clk *pkiClock) {
			live(t, s, clk)
			update(t, s, "redeemed_at=5")
		}, intentOnly: true, want: CodeNoIntent, consumed: true},
		{name: "redeemed with intent, intentOnly, taints", setup: func(t *testing.T, s *Store, clk *pkiClock) {
			o := pkiOffer("o1", 1, time.Minute, clk)
			o.Intent = "i"
			put(t, s, o)
			update(t, s, "redeemed_at=5")
		}, intentOnly: true, want: CodeReused, tainted: true, consumed: true},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			s, clk := pkiTestStore(t)
			c.setup(t, s, clk)
			_, res, err := s.PKIOfferClaim(ctx, pkiHash(1), c.intentOnly)
			if err != nil || res != c.want {
				t.Fatalf("claim = %q %v, want %q", res, err, c.want)
			}
			var tainted bool
			var redeemed, attempts int64
			if err = s.readers.QueryRow(`SELECT tainted,redeemed_at,attempts FROM pki_offer WHERE id='o1'`).Scan(&tainted, &redeemed, &attempts); err != nil {
				t.Fatal(err)
			}
			if tainted != c.tainted || (redeemed > 0) != c.consumed || attempts != 0 {
				t.Fatalf("tainted=%v redeemed=%d attempts=%d", tainted, redeemed, attempts)
			}
		})
	}
}

func TestPKIOfferNoIntentThenRedeemable(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	if err := s.PKIOfferPut(ctx, pkiOffer("o1", 1, time.Minute, clk)); err != nil {
		t.Fatal(err)
	}
	if _, res, _ := s.PKIOfferClaim(ctx, pkiHash(1), true); res != CodeNoIntent {
		t.Fatalf("res %q", res)
	}
	if _, res, _ := s.PKIOfferClaim(ctx, pkiHash(1), false); res != CodeOK {
		t.Fatalf("offer was consumed by the refused claim: %q", res)
	}
	if _, res, _ := s.PKIOfferClaim(ctx, pkiHash(1), false); res != CodeReused {
		t.Fatalf("second claim: %q", res)
	}
}

func TestPKIOfferWrongCodesLockLiveOffers(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	for i, id := range []string{"live", "redeemed", "withdrawn", "locked", "expired"} {
		o := pkiOffer(id, byte(i+1), 10*time.Minute, clk)
		if id == "expired" {
			o.ExpiresAt = clk.now().UnixNano() // expires exactly now
		}
		if err := s.PKIOfferPut(ctx, o); err != nil {
			t.Fatal(err)
		}
	}
	for id, set := range map[string]string{"redeemed": "redeemed_at=3", "withdrawn": "withdrawn=1", "locked": "locked=1"} {
		if _, err := s.writer.Exec(`UPDATE pki_offer SET `+set+` WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
	}
	attempts := func(id string) (n int64, locked bool) {
		if err := s.readers.QueryRow(`SELECT attempts,locked FROM pki_offer WHERE id=?`, id).Scan(&n, &locked); err != nil {
			t.Fatal(err)
		}
		return
	}
	for i := 1; i <= MaxCodeAttempts-1; i++ {
		_, res, err := s.PKIOfferClaim(ctx, pkiHash(0xEE), false)
		if err != nil || res != CodeUnknown {
			t.Fatalf("wrong code %d: %q %v", i, res, err)
		}
		if n, locked := attempts("live"); n != int64(i) || locked {
			t.Fatalf("after %d wrong codes: attempts=%d locked=%v", i, n, locked)
		}
	}
	// the right code still works on attempt five
	s2, clk2 := pkiTestStore(t)
	_ = s2.PKIOfferPut(ctx, pkiOffer("x", 1, time.Minute, clk2))
	for i := 0; i < MaxCodeAttempts-1; i++ {
		_, _, _ = s2.PKIOfferClaim(ctx, pkiHash(0xEE), false)
	}
	if _, res, _ := s2.PKIOfferClaim(ctx, pkiHash(1), false); res != CodeOK {
		t.Fatalf("four wrong codes must not lock: %q", res)
	}
	// the fifth wrong code locks
	if _, res, _ := s.PKIOfferClaim(ctx, pkiHash(0xEE), false); res != CodeUnknown {
		t.Fatalf("res %q", res)
	}
	if n, locked := attempts("live"); n != MaxCodeAttempts || !locked {
		t.Fatalf("fifth wrong code: attempts=%d locked=%v", n, locked)
	}
	for _, id := range []string{"redeemed", "withdrawn", "locked", "expired"} {
		if n, _ := attempts(id); n != 0 {
			t.Fatalf("%s got %d wrong attempts", id, n)
		}
	}
	if _, res, _ := s.PKIOfferClaim(ctx, pkiHash(1), false); res != CodeLocked {
		t.Fatalf("right code on a locked offer: %q", res)
	}
	// a locked offer is no longer counted
	if _, _, err := s.PKIOfferClaim(ctx, pkiHash(0xEE), false); err != nil {
		t.Fatal(err)
	}
	if n, _ := attempts("live"); n != MaxCodeAttempts {
		t.Fatalf("locked offer kept counting: %d", n)
	}
}

func TestPKIOfferBindAndWithdraw(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	if err := s.PKIOfferPut(ctx, pkiOffer("o1", 1, time.Minute, clk)); err != nil {
		t.Fatal(err)
	}
	if err := s.PKIOfferPut(ctx, pkiOffer("o2", 2, time.Minute, clk)); err != nil {
		t.Fatal(err)
	}
	_, _, _ = s.PKIOfferClaim(ctx, pkiHash(1), false)
	tainted, err := s.PKIOfferBind(ctx, "o1", "node-1", "serial-1")
	if err != nil || tainted {
		t.Fatalf("bind: tainted=%v %v", tainted, err)
	}
	var node, serial string
	if err = s.readers.QueryRow(`SELECT node_id,serial FROM pki_offer WHERE id='o1'`).Scan(&node, &serial); err != nil || node != "node-1" || serial != "serial-1" {
		t.Fatalf("bound %q %q %v", node, serial, err)
	}
	if err = s.readers.QueryRow(`SELECT node_id FROM pki_offer WHERE id='o2'`).Scan(&node); err != nil || node != "" {
		t.Fatalf("bind touched another offer: %q", node)
	}
	_, res, _ := s.PKIOfferClaim(ctx, pkiHash(1), false)
	if res != CodeReused {
		t.Fatalf("res %q", res)
	}
	if tainted, err = s.PKIOfferBind(ctx, "o1", "node-1", "serial-1"); err != nil || !tainted {
		t.Fatalf("bind after reuse: tainted=%v %v", tainted, err)
	}
	if tainted, err = s.PKIOfferBind(ctx, "o2", "node-2", "serial-2"); err != nil || tainted {
		t.Fatalf("other offer must stay clean: %v %v", tainted, err)
	}

	// withdraw
	w, err := s.PKIOfferWithdraw(ctx, pkiHash(2))
	if err != nil || w.ID != "o2" || w.Withdrawn || w.Label != "lbl-o2" {
		t.Fatalf("withdraw returns the offer as it was: %+v %v", w, err)
	}
	if _, res, _ = s.PKIOfferClaim(ctx, pkiHash(2), false); res != CodeWithdrawn {
		t.Fatalf("claim after withdraw: %q", res)
	}
	_, err = s.PKIOfferWithdraw(ctx, pkiHash(0x77))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown withdraw: %v", err)
	}
	// a bind that lands after the offer was withdrawn reports tainted (the caller revokes at once)
	if err = s.PKIOfferPut(ctx, pkiOffer("o3", 3, time.Minute, clk)); err != nil {
		t.Fatal(err)
	}
	_, _, _ = s.PKIOfferClaim(ctx, pkiHash(3), false)
	if tainted, err = s.PKIOfferBind(ctx, "o3", "node-3", "serial-3"); err != nil || tainted {
		t.Fatalf("bind before withdraw: %v %v", tainted, err)
	}
	if _, err = s.PKIOfferWithdraw(ctx, pkiHash(3)); err != nil {
		t.Fatal(err)
	}
	if tainted, err = s.PKIOfferBind(ctx, "o3", "node-3", "serial-3"); err != nil || !tainted {
		t.Fatalf("bind after withdraw: tainted=%v %v", tainted, err)
	}
	var taintCol bool
	if err = s.readers.QueryRow(`SELECT tainted FROM pki_offer WHERE id='o3'`).Scan(&taintCol); err != nil || taintCol {
		t.Fatalf("withdrawn must not set the tainted column: %v %v", taintCol, err)
	}
	// withdrawing after redemption returns the redeemed offer so the caller can revoke its certificate
	w, err = s.PKIOfferWithdraw(ctx, pkiHash(1))
	if err != nil || w.NodeID != "node-1" || w.Serial != "serial-1" || w.RedeemedAt == 0 {
		t.Fatalf("withdraw of a redeemed offer: %+v %v", w, err)
	}
}

func TestPKIOffersLive(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	mk := func(id string, hash byte, intent string, ttl time.Duration) {
		o := pkiOffer(id, hash, ttl, clk)
		o.Intent = intent
		if err := s.PKIOfferPut(ctx, o); err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Millisecond)
	}
	mk("b-live-intent", 1, "i1", time.Hour)
	mk("a-live-plain", 2, "", time.Hour)
	mk("c-redeemed", 3, "i3", time.Hour)
	mk("d-withdrawn", 4, "i4", time.Hour)
	mk("e-locked", 5, "i5", time.Hour)
	mk("f-expired", 6, "i6", time.Second)
	mk("g-expires-now", 7, "i7", 0) // expires_at == created: already dead
	mk("h-short", 8, "i8", 5*time.Second)
	for id, set := range map[string]string{"c-redeemed": "redeemed_at=1", "d-withdrawn": "withdrawn=1", "e-locked": "locked=1"} {
		if _, err := s.writer.Exec(`UPDATE pki_offer SET `+set+` WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
	}
	clk.advance(2 * time.Second)
	ids := func(intentOnly bool) (out []string) {
		got, err := s.PKIOffersLive(ctx, intentOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range got {
			out = append(out, o.ID)
		}
		return out
	}
	if got := ids(false); !reflect.DeepEqual(got, []string{"b-live-intent", "a-live-plain", "h-short"}) { // created_at order
		t.Fatalf("live: %v", got)
	}
	if got := ids(true); !reflect.DeepEqual(got, []string{"b-live-intent", "h-short"}) {
		t.Fatalf("live with intent: %v", got)
	}
	got, _ := s.PKIOffersLive(ctx, true)
	if got[0].Intent != "i1" || got[0].Label != "lbl-b-live-intent" || !bytes.Equal(got[0].CodeHash, pkiHash(1)) || got[0].CertTTL != int64(30*time.Minute) {
		t.Fatalf("fields: %+v", got[0])
	}
	clk.advance(10 * time.Second)
	if got := ids(true); !reflect.DeepEqual(got, []string{"b-live-intent"}) {
		t.Fatalf("after h-short expired: %v", got)
	}
	// created_at ties break on id
	s2, clk2 := pkiTestStore(t)
	for i, id := range []string{"z", "y"} {
		_ = s2.PKIOfferPut(ctx, pkiOffer(id, byte(i+1), time.Hour, clk2))
	}
	if l, _ := s2.PKIOffersLive(ctx, false); len(l) != 2 || l[0].ID != "y" {
		t.Fatalf("tie order: %+v", l)
	}
}

func TestPKIOfferConcurrentClaimsHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	s, clk := pkiTestStore(t)
	if err := s.PKIOfferPut(ctx, pkiOffer("o1", 1, time.Minute, clk)); err != nil {
		t.Fatal(err)
	}
	const n = 16
	results := make([]CodeClaim, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, res, err := s.PKIOfferClaim(ctx, pkiHash(1), false)
			if err != nil {
				res = CodeClaim("error: " + err.Error())
			}
			results[i] = res
		}()
	}
	close(start)
	wg.Wait()
	ok, reused := 0, 0
	for _, r := range results {
		switch r {
		case CodeOK:
			ok++
		case CodeReused:
			reused++
		default:
			t.Fatalf("unexpected outcome %q", r)
		}
	}
	if ok != 1 || reused != n-1 {
		t.Fatalf("ok=%d reused=%d", ok, reused)
	}
}
