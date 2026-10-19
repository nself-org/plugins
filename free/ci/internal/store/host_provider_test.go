package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestHostProviderFacts(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	t0 := time.Unix(1_700_000_000, 0).UTC()
	put := func(scope, key, value string, at time.Time) bool {
		t.Helper()
		kept, err := s.ProviderFactPut(ctx, ProviderFact{Provider: "github", Scope: scope, Key: key, Value: json.RawMessage(value), ObservedAt: at})
		if err != nil {
			t.Fatal(err)
		}
		return kept
	}
	if !put("org", "repo/a", `{"n":1}`, t0) || !put("org", "repo/b", `2`, t0) || !put("org", "other/c", `3`, t0) || !put("else", "repo/z", `4`, t0) {
		t.Fatal("a fresh fact must be kept")
	}
	if put("org", "repo/a", `{"n":0}`, t0.Add(-time.Second)) {
		t.Fatal("an older observation must not replace a newer one")
	}
	if !put("org", "repo/a", `{"n":2}`, t0.Add(time.Second)) {
		t.Fatal("a newer observation must replace the stored one")
	}
	got, ok, err := s.ProviderFactGet(ctx, "github", "org", "repo/a")
	if err != nil || !ok || string(got.Value) != `{"n":2}` || !got.ObservedAt.Equal(t0.Add(time.Second)) {
		t.Fatalf("get: %+v %v %v", got, ok, err)
	}
	if _, ok, _ = s.ProviderFactGet(ctx, "gitlab", "org", "repo/a"); ok {
		t.Fatal("facts are per provider")
	}
	list, err := s.ProviderFactList(ctx, "github", "org", "repo/")
	if err != nil || len(list) != 2 || list[0].Key != "repo/a" || list[1].Key != "repo/b" {
		t.Fatalf("prefix list: %+v %v", list, err)
	}
	if n, err := s.ProviderFactCount(ctx, "github", "org", "repo/"); err != nil || n != 2 {
		t.Fatalf("prefix count %d %v", n, err)
	}
	// A prefix is literal: % and _ are not wildcards.
	if n, _ := s.ProviderFactCount(ctx, "github", "org", "%"); n != 0 {
		t.Fatalf("wildcard prefix matched %d facts", n)
	}
	if n, _ := s.ProviderFactCount(ctx, "github", "org", ""); n != 3 {
		t.Fatalf("empty prefix counts the whole scope, got %d", n)
	}
	if _, err = s.ProviderFactPut(ctx, ProviderFact{Provider: "github", Key: "k", Value: json.RawMessage(`{bad`)}); err == nil {
		t.Fatal("a non-JSON value must be refused")
	}
}

func TestHostProviderMarksAndIntents(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	t0 := time.Unix(1_700_000_000, 0).UTC()
	if _, ok, err := s.ProviderMarkGet(ctx, "reconcile", "org/repo"); ok || err != nil {
		t.Fatalf("unset mark: %v %v", ok, err)
	}
	for _, at := range []time.Time{t0, t0.Add(time.Minute), t0.Add(30 * time.Second)} {
		if err := s.ProviderMarkPut(ctx, "reconcile", "org/repo", at); err != nil {
			t.Fatal(err)
		}
	}
	if at, ok, err := s.ProviderMarkGet(ctx, "reconcile", "org/repo"); err != nil || !ok || !at.Equal(t0.Add(time.Minute)) {
		t.Fatalf("a mark never moves back: %v %v %v", at, ok, err)
	}
	first := DispatchIntent{AttemptID: "a1", Provider: "github", Nonce: "n1", RunID: "r1", Ref: "abc", Epoch: 5, CreatedAt: t0}
	if got, err := s.DispatchIntentPut(ctx, first); err != nil || got.RunID != "r1" {
		t.Fatalf("first put: %+v %v", got, err)
	}
	// Same epoch (a second carrier of the lease) and a lower epoch (a stale carrier) lose and learn the winner.
	for _, epoch := range []int64{5, 4} {
		got, err := s.DispatchIntentPut(ctx, DispatchIntent{AttemptID: "a1", Provider: "github", Nonce: "n2", RunID: "r2", Epoch: epoch})
		if err != nil || got.RunID != "r1" || got.Nonce != "n1" || got.Epoch != 5 {
			t.Fatalf("epoch %d must not repoint the attempt: %+v %v", epoch, got, err)
		}
	}
	got, ok, err := s.DispatchIntentGet(ctx, "a1")
	if err != nil || !ok || got.RunID != "r1" || got.Nonce != "n1" || got.Ref != "abc" || got.Epoch != 5 {
		t.Fatalf("the stored intent: %+v %v %v", got, ok, err)
	}
	// A retry (higher epoch) replaces it.
	if got, err = s.DispatchIntentPut(ctx, DispatchIntent{AttemptID: "a1", Provider: "github", Nonce: "n3", RunID: "r3", Epoch: 6}); err != nil || got.RunID != "r3" || got.Epoch != 6 {
		t.Fatalf("a retry must replace the intent: %+v %v", got, err)
	}
	// A cleanup of the old run cannot remove the retry's intent; a cleanup of the current one does.
	if err = s.DispatchIntentDelete(ctx, "a1", "r1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ = s.DispatchIntentGet(ctx, "a1"); !ok {
		t.Fatal("deleting the old run removed the retry's intent")
	}
	if err = s.DispatchIntentDelete(ctx, "a1", "r3"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ = s.DispatchIntentGet(ctx, "a1"); ok {
		t.Fatal("the intent of the current run was not deleted")
	}
	if _, ok, _ = s.DispatchIntentGet(ctx, "a2"); ok {
		t.Fatal("an undispatched attempt has no intent")
	}
	for i, at := range []time.Time{t0.Add(-48 * time.Hour), t0.Add(-time.Hour)} {
		if _, err = s.DispatchIntentPut(ctx, DispatchIntent{AttemptID: "old" + string(rune('a'+i)), Provider: "github", RunID: "r", CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.DispatchIntentPrune(ctx, t0.Add(-24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("prune removed %d (%v), want only the 48 h old intent", n, err)
	}
	if _, ok, _ = s.DispatchIntentGet(ctx, "oldb"); !ok {
		t.Fatal("the recent intent was pruned")
	}
	if _, err = s.DispatchIntentPut(ctx, DispatchIntent{AttemptID: "a3"}); err == nil {
		t.Fatal("an intent without a provider and a run must be refused")
	}
	if err = s.ProviderMarkPut(ctx, "m", "k", time.Time{}); err == nil {
		t.Fatal("a mark without a time must be refused")
	}
	if _, err = s.ProviderFactPut(ctx, ProviderFact{Provider: "github", Key: "k", Value: json.RawMessage(`1`)}); err == nil {
		t.Fatal("a fact without an observation time must be refused")
	}
}

// TestHostProviderMigratesFromPreviousVersion proves the set is expand only: a database that predates it
// gains the three tables on open and keeps the rows it had.
func TestHostProviderMigratesFromPreviousVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	ctx := context.Background()
	s, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	seed(t, s, "keep")
	// Turn it into a database from before this set existed.
	if _, err = s.writer.ExecContext(ctx, `DROP TABLE provider_fact; DROP TABLE provider_mark; DROP TABLE dispatch_intent; DELETE FROM migration_log WHERE set_name='host-provider'`); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, Options{})
	if err != nil {
		t.Fatalf("a previous-version database must migrate: %v", err)
	}
	defer func() { _ = s.Close() }()
	if err = s.ProviderMarkPut(ctx, "m", "k", time.Now()); err != nil {
		t.Fatal(err)
	}
	var pipelines, logged int
	if err = s.readers.QueryRowContext(ctx, `SELECT count(*) FROM pipeline`).Scan(&pipelines); err != nil || pipelines == 0 {
		t.Fatalf("existing rows must survive the migration: %d %v", pipelines, err)
	}
	if err = s.readers.QueryRowContext(ctx, `SELECT count(*) FROM migration_log WHERE set_name='host-provider'`).Scan(&logged); err != nil || logged != 1 {
		t.Fatalf("migration log: %d %v", logged, err)
	}
}
