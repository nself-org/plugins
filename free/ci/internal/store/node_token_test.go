package store

// Purpose: tests of the node-token store set (P7-NODE-08): the migration, first-use binding, durability across a
//          reopen, the session compare and swap, and pruning.
// Inputs:  a real SQLite store in t.TempDir() with an injected clock (pkiTestStore).
// Constraints: no sleeps; the clock is moved by the test.

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tokRow(jti, serial, session string, exp int64) TokenBinding {
	return TokenBinding{JTI: jti, Serial: serial, Session: session, NodeID: "node-a", Exp: exp}
}

func TestTokenMigrationApplied(t *testing.T) {
	s, _ := pkiTestStore(t)
	var n int
	if err := s.readers.QueryRow(`SELECT count(*) FROM migration_log WHERE set_name='node-token' AND filename='0001_token.sql'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("node-token migration not applied: %d %v", n, err)
	}
	if err := Register("node-token", tokenMigrations); err == nil {
		t.Fatal("node-token registered twice")
	}
}

func TestTokenBindFirstWriterWins(t *testing.T) {
	s, clk := pkiTestStore(t)
	ctx := context.Background()
	got, created, err := s.TokenBind(ctx, tokRow("j1", "s1", "sess1", 100))
	if err != nil || !created || got.Serial != "s1" || got.Session != "sess1" || got.BoundAt != clk.now().Unix() || got.Exp != 100 || got.NodeID != "node-a" {
		t.Fatalf("first bind: %+v created=%v err=%v", got, created, err)
	}
	clk.advance(time.Minute)
	got, created, err = s.TokenBind(ctx, tokRow("j1", "s2", "sess2", 999))
	if err != nil || created || got.Serial != "s1" || got.Session != "sess1" || got.Exp != 100 {
		t.Fatalf("second bind must return the stored row: %+v created=%v err=%v", got, created, err)
	}
}

func TestTokenBindRefusesIncompleteRows(t *testing.T) {
	s, _ := pkiTestStore(t)
	for _, b := range []TokenBinding{tokRow("", "s", "x", 1), tokRow("j", "", "x", 1), tokRow("j", "s", "", 1), {JTI: "j", Serial: "s", Session: "x"}} {
		if _, created, err := s.TokenBind(context.Background(), b); err == nil || created {
			t.Fatalf("bind of %+v: created=%v err=%v", b, created, err)
		}
	}
}

func TestTokenCheck(t *testing.T) {
	s, _ := pkiTestStore(t)
	ctx := context.Background()
	if _, found, err := s.TokenCheck(ctx, "none"); err != nil || found {
		t.Fatalf("unknown jti: found=%v err=%v", found, err)
	}
	if _, _, err := s.TokenBind(ctx, tokRow("j1", "s1", "sess1", 5)); err != nil {
		t.Fatal(err)
	}
	b, found, err := s.TokenCheck(ctx, "j1")
	if err != nil || !found || b.JTI != "j1" || b.Serial != "s1" || b.Session != "sess1" {
		t.Fatalf("check: %+v found=%v err=%v", b, found, err)
	}
}

func TestTokenBindingSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.TokenBind(context.Background(), tokRow("j1", "s1", "sess1", 5)); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	b, found, err := s.TokenCheck(context.Background(), "j1")
	if err != nil || !found || b.Serial != "s1" || b.Session != "sess1" {
		t.Fatalf("binding lost across a reopen: %+v found=%v err=%v", b, found, err)
	}
}

func TestTokenRebindCompareAndSwap(t *testing.T) {
	s, _ := pkiTestStore(t)
	ctx := context.Background()
	if _, _, err := s.TokenBind(ctx, tokRow("j1", "s1", "sess1", 5)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ jti, serial, from, to string }{
		{"j1", "s9", "sess1", "sess2"}, // another certificate
		{"j1", "s1", "sess9", "sess2"}, // the caller saw another session
		{"nope", "s1", "sess1", "sess2"},
		{"j1", "s1", "sess1", ""},
	} {
		if moved, _ := s.TokenRebind(ctx, c.jti, c.serial, c.from, c.to); moved {
			t.Fatalf("rebind %+v moved the row", c)
		}
	}
	if moved, err := s.TokenRebind(ctx, "j1", "s1", "sess1", "sess2"); err != nil || !moved {
		t.Fatalf("rebind: moved=%v err=%v", moved, err)
	}
	if moved, _ := s.TokenRebind(ctx, "j1", "s1", "sess1", "sess3"); moved {
		t.Fatal("a second rebind from the old session moved the row")
	}
	if b, _, _ := s.TokenCheck(ctx, "j1"); b.Session != "sess2" {
		t.Fatalf("session after rebind: %q", b.Session)
	}
}

func TestTokenRebindRaceHasOneWinner(t *testing.T) {
	s, _ := pkiTestStore(t)
	ctx := context.Background()
	if _, _, err := s.TokenBind(ctx, tokRow("j1", "s1", "sess1", 5)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for _, to := range []string{"a", "b", "c", "d"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if moved, _ := s.TokenRebind(ctx, "j1", "s1", "sess1", to); moved {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won != 1 {
		t.Fatalf("%d rebinds won, want 1", won)
	}
}

func TestTokenSpend(t *testing.T) {
	s, _ := pkiTestStore(t)
	ctx := context.Background()
	if err := s.TokenSpend(ctx, "ghost"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := s.TokenCheck(ctx, "ghost"); found {
		t.Fatal("spending an unbound token created a row")
	}
	b, _, _ := s.TokenBind(ctx, tokRow("j1", "s1", "sess1", 5))
	if b.Spent {
		t.Fatal("a fresh binding is spent")
	}
	if err := s.TokenSpend(ctx, "j1"); err != nil {
		t.Fatal(err)
	}
	if b, _, _ = s.TokenCheck(ctx, "j1"); !b.Spent {
		t.Fatal("binding not spent")
	}
	if b, created, _ := s.TokenBind(ctx, tokRow("j1", "s1", "sess1", 5)); created || !b.Spent {
		t.Fatalf("a re-bind must report the spent row: %+v created=%v", b, created)
	}
}

func TestTokenPrune(t *testing.T) {
	s, _ := pkiTestStore(t)
	ctx := context.Background()
	now := int64(10_000)
	for jti, exp := range map[string]int64{"old": now - TokenPruneGrace - 1, "edge": now - TokenPruneGrace, "live": now + 60} {
		if _, _, err := s.TokenBind(ctx, tokRow(jti, "s", "x", exp)); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.TokenPrune(ctx, now)
	if err != nil || n != 1 {
		t.Fatalf("prune removed %d (%v), want 1", n, err)
	}
	for jti, want := range map[string]bool{"old": false, "edge": true, "live": true} {
		if _, found, _ := s.TokenCheck(ctx, jti); found != want {
			t.Fatalf("%s found=%v want %v", jti, found, want)
		}
	}
}
