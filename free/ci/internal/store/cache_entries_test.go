package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var cacheTrusted = entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}

const cacheKey = "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"

func TestCacheEntriesMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	count := func(s *Store, q string) (n int) {
		t.Helper()
		if err := s.readers.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var version int
	for i := 0; i < 2; i++ {
		s, err := Open(path, Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"cache_attempt", "cache_planned", "cache_entry", "cache_entry_blob", "cache_use"} {
			if count(s, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='"+table+"'") != 1 {
				t.Fatalf("table %s missing", table)
			}
		}
		if count(s, "SELECT count(*) FROM migration_log WHERE set_name='cache-entries' AND filename='0001_init.sql'") != 1 {
			t.Fatal("migration not logged exactly once")
		}
		v := count(s, "SELECT schema_version FROM schema_meta")
		if i == 1 && v != version {
			t.Fatalf("reopen changed schema_version %d -> %d", version, v)
		}
		version = v
		_ = s.Close()
	}
}

// cacheFinalizing creates attempt id, records the plan while running and moves
// it to finalizing with the given result; it returns the running and finalizing epochs.
func cacheFinalizing(t *testing.T, s *Store, id string, f entries.Facts, keys []string, passed bool) (running, finalizing int64) {
	t.Helper()
	ctx := context.Background()
	seed(t, s, id)
	step := func(from, to model.JobState, epoch int64) {
		t.Helper()
		if err := s.Transition(ctx, id, from, to, TransitionDetail{Epoch: epoch}); err != nil {
			t.Fatal(err)
		}
	}
	step("queued", "leased", 0)
	step("leased", "running", 1)
	if err := s.CacheRecordPlan(ctx, id, 2, f, keys); err != nil {
		t.Fatal(err)
	}
	step("running", "finalizing", 2)
	if err := s.CacheRecordResult(ctx, id, 3, passed); err != nil {
		t.Fatal(err)
	}
	return 2, 3
}

func cacheManifest(t *testing.T, f entries.Facts, key string, size int64) entries.Manifest {
	t.Helper()
	ns, err := entries.NamespaceOf(f)
	if err != nil {
		t.Fatal(err)
	}
	return entries.Manifest{Namespace: ns.Canonical(), Key: key, Spec: "go", Blobs: []entries.Blob{{Path: "mod.tar.gz", SHA256: strings.Repeat("b", 64), Size: size}}}
}

func cacheAuditCount(t *testing.T, s *Store, action, target, detail string) (n int) {
	t.Helper()
	if err := s.readers.QueryRow("SELECT count(*) FROM audit WHERE action=? AND target=? AND detail=?", action, target, detail).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCacheEntriesCommitRefusals(t *testing.T) {
	ctx := context.Background()
	other := entries.Facts{Project: "other", Trust: "owner", Protection: "protected"}
	pr1 := entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "1"}
	trustedNS, _ := entries.NamespaceOf(cacheTrusted)
	pr2NS, _ := authz.CacheNamespace("proj", "untrusted", authz.Unprotected, "fork/example", "2")
	type tc struct {
		name   string
		facts  entries.Facts
		passed bool
		mutate func(*entries.CommitRequest, int64)
		reason string // E711 reason; "" when denied is set
		denied authz.Reason
	}
	cases := []tc{
		{name: "stale epoch", passed: true, reason: "epoch", mutate: func(c *entries.CommitRequest, fin int64) { c.Epoch = fin - 1 }},
		{name: "failing result", passed: false, reason: "attempt_state"},
		{name: "foreign namespace in manifest", passed: true, reason: "namespace", mutate: func(c *entries.CommitRequest, _ int64) {
			c.Manifest.Namespace = cacheManifest(t, other, cacheKey, 10).Namespace
		}},
		{name: "unplanned key", passed: true, reason: "not_planned", mutate: func(c *entries.CommitRequest, _ int64) { c.Manifest.Key = strings.Repeat("c", 64) }},
		{name: "size cap", passed: true, reason: "size_cap", mutate: func(c *entries.CommitRequest, _ int64) { c.MaxEntryBytes = 9 }},
		{name: "untrusted commit to trusted namespace", facts: pr1, passed: true, denied: authz.ReasonNamespace, mutate: func(c *entries.CommitRequest, _ int64) {
			c.Claimed, c.Manifest.Namespace = trustedNS, trustedNS.Canonical()
		}},
		{name: "PR 1 commit to PR 2 namespace", facts: pr1, passed: true, denied: authz.ReasonNamespace, mutate: func(c *entries.CommitRequest, _ int64) {
			c.Claimed, c.Manifest.Namespace = pr2NS, pr2NS.Canonical()
		}},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testStore(t)
			id := "att" + string(rune('a'+i))
			f := c.facts
			if f == (entries.Facts{}) {
				f = cacheTrusted
			}
			_, fin := cacheFinalizing(t, s, id, f, []string{cacheKey}, c.passed)
			ns, _ := entries.NamespaceOf(f)
			req := entries.CommitRequest{Attempt: id, Epoch: fin, Claimed: ns, Manifest: cacheManifest(t, f, cacheKey, 10)}
			if c.mutate != nil {
				c.mutate(&req, fin)
			}
			err := s.CacheCommit(ctx, req)
			var refused *entries.Refused
			var denied *authz.Denied
			switch {
			case c.denied != "":
				if !errors.As(err, &denied) || denied.Reason != c.denied {
					t.Fatalf("want E684 %s, got %v", c.denied, err)
				}
				if cacheAuditCount(t, s, "cache.commit.denied", id, "reason="+string(c.denied)) != 1 {
					t.Fatal("no audit row for the denial")
				}
			default:
				if !errors.As(err, &refused) || refused.Reason != c.reason {
					t.Fatalf("want E711 %s, got %v", c.reason, err)
				}
				if cacheAuditCount(t, s, "cache.commit.refused", id, "reason="+c.reason) != 1 {
					t.Fatal("no audit row for the refusal")
				}
			}
			if list, err := s.CacheEntries(ctx, ""); err != nil || len(list) != 0 {
				t.Fatalf("a refused commit left %d entries (%v)", len(list), err)
			}
		})
	}
}

func TestCacheEntriesCommitStates(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	ns, _ := entries.NamespaceOf(cacheTrusted)
	req := func(id string, epoch int64) entries.CommitRequest {
		return entries.CommitRequest{Attempt: id, Epoch: epoch, Claimed: ns, Manifest: cacheManifest(t, cacheTrusted, cacheKey, 10)}
	}
	reason := func(err error) string {
		var r *entries.Refused
		if !errors.As(err, &r) {
			t.Fatalf("not E711: %v", err)
		}
		return r.Reason
	}
	// running: plan recorded but the attempt is not finalizing yet
	seed(t, s, "run")
	for _, st := range [][3]any{{model.JobState("queued"), model.JobState("leased"), int64(0)}, {model.JobState("leased"), model.JobState("running"), int64(1)}} {
		if err := s.Transition(ctx, "run", st[0].(model.JobState), st[1].(model.JobState), TransitionDetail{Epoch: st[2].(int64)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CacheRecordPlan(ctx, "run", 2, cacheTrusted, []string{cacheKey}); err != nil {
		t.Fatal(err)
	}
	if got := reason(s.CacheCommit(ctx, req("run", 2))); got != "attempt_state" {
		t.Fatalf("running: %s", got)
	}
	// unknown attempt, and an attempt with no recorded facts
	if got := reason(s.CacheCommit(ctx, req("ghost", 0))); got != "attempt_state" {
		t.Fatalf("unknown: %s", got)
	}
	seed(t, s, "bare")
	if got := reason(s.CacheCommit(ctx, req("bare", 0))); got != "attempt_state" {
		t.Fatalf("bare: %s", got)
	}
	// terminal: a finished attempt cannot write
	_, fin := cacheFinalizing(t, s, "done", cacheTrusted, []string{cacheKey}, true)
	if err := s.Transition(ctx, "done", "finalizing", "passed", TransitionDetail{Epoch: fin}); err != nil {
		t.Fatal(err)
	}
	if got := reason(s.CacheCommit(ctx, req("done", fin+1))); got != "attempt_state" {
		t.Fatalf("terminal: %s", got)
	}
	// success, then a replay of the same save
	_, fin = cacheFinalizing(t, s, "ok", cacheTrusted, []string{cacheKey}, true)
	if err := s.CacheCommit(ctx, req("ok", fin)); err != nil {
		t.Fatal(err)
	}
	list, err := s.CacheEntries(ctx, cacheKey)
	if err != nil || len(list) != 1 {
		t.Fatalf("entries %v %v", list, err)
	}
	w := list[0].Writer
	if w.AttemptID != "ok" || w.Epoch != fin || w.SourceTrust != "owner" || w.SecretsReceived || w.PrivateDeps || list[0].Bytes() != 10 {
		t.Fatalf("writer facts %+v", w)
	}
	if got := reason(s.CacheCommit(ctx, req("ok", fin))); got != "not_planned" {
		t.Fatalf("replay: %s", got)
	}
}

func TestCacheEntriesPlanAndResultRefusals(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	reason := func(err error) string {
		var r *entries.Refused
		if !errors.As(err, &r) {
			return "not E711: " + err.Error()
		}
		return r.Reason
	}
	seed(t, s, "p")
	if got := reason(s.CacheRecordPlan(ctx, "p", 0, cacheTrusted, nil)); got != "attempt_state" {
		t.Fatalf("plan while queued: %s", got)
	}
	for _, st := range [][3]any{{model.JobState("queued"), model.JobState("leased"), int64(0)}, {model.JobState("leased"), model.JobState("running"), int64(1)}} {
		if err := s.Transition(ctx, "p", st[0].(model.JobState), st[1].(model.JobState), TransitionDetail{Epoch: st[2].(int64)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := reason(s.CacheRecordPlan(ctx, "p", 1, cacheTrusted, nil)); got != "epoch" {
		t.Fatalf("stale plan epoch: %s", got)
	}
	if err := s.CacheRecordPlan(ctx, "p", 2, entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected"}, nil); err == nil {
		t.Fatal("an untrusted plan without head repo and PR was accepted")
	}
	if err := s.CacheRecordPlan(ctx, "p", 2, cacheTrusted, []string{cacheKey}); err != nil {
		t.Fatal(err)
	}
	if err := s.CacheRecordPlan(ctx, "p", 2, cacheTrusted, []string{cacheKey}); err != nil {
		t.Fatalf("an identical second plan must be idempotent: %v", err)
	}
	if got := reason(s.CacheRecordPlan(ctx, "p", 2, entries.Facts{Project: "proj", Trust: "owner", Protection: "unprotected"}, nil)); got != "namespace" {
		t.Fatalf("changed facts: %s", got)
	}
	if got := reason(s.CacheRecordResult(ctx, "p", 2, true)); got != "attempt_state" {
		t.Fatalf("result while running: %s", got)
	}
	if err := s.Transition(ctx, "p", "running", "finalizing", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	if got := reason(s.CacheRecordResult(ctx, "p", 2, true)); got != "epoch" {
		t.Fatalf("stale result epoch: %s", got)
	}
	seed(t, s, "q")
	for _, st := range [][3]any{{model.JobState("queued"), model.JobState("leased"), int64(0)}, {model.JobState("leased"), model.JobState("running"), int64(1)}, {model.JobState("running"), model.JobState("finalizing"), int64(2)}} {
		if err := s.Transition(ctx, "q", st[0].(model.JobState), st[1].(model.JobState), TransitionDetail{Epoch: st[2].(int64)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := reason(s.CacheRecordResult(ctx, "q", 3, true)); got != "not_planned" {
		t.Fatalf("result with no plan: %s", got)
	}
}

var _ entries.Backend = (*Store)(nil)
