package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// cacheSeed creates a queued attempt whose pipeline carries the facts' project and trust.
func cacheSeed(t *testing.T, s *Store, id string, f entries.Facts) {
	t.Helper()
	ctx := context.Background()
	p := PipelineRow{ID: "p" + id, Revision: "rev", Project: f.Project, Trigger: "local", SourceTrust: f.Trust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for _, err := range []error{s.CreatePipeline(ctx, p), s.CreateJob(ctx, JobRow{ID: "j" + id, PipelineID: p.ID, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}), s.CreateAttempt(ctx, AttemptRow{ID: id, JobID: "j" + id, N: 1}, "")} {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func cacheStep(t *testing.T, s *Store, id string, from, to model.JobState, epoch int64) {
	t.Helper()
	if err := s.Transition(context.Background(), id, from, to, TransitionDetail{Epoch: epoch, Operator: from == "failed"}); err != nil {
		t.Fatal(err)
	}
}

// cacheFinalizing creates attempt id, records the plan while running and moves
// it to finalizing with the given result; it returns the running and finalizing epochs.
func cacheFinalizing(t *testing.T, s *Store, id string, f entries.Facts, keys []string, passed bool) (running, finalizing int64) {
	t.Helper()
	ctx := context.Background()
	cacheSeed(t, s, id, f)
	cacheStep(t, s, id, "queued", "leased", 0)
	cacheStep(t, s, id, "leased", "running", 1)
	if err := s.CacheRecordPlan(ctx, id, 2, f, keys); err != nil {
		t.Fatal(err)
	}
	cacheStep(t, s, id, "running", "finalizing", 2)
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
	cacheSeed(t, s, "run", cacheTrusted)
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
	cacheSeed(t, s, "bare", cacheTrusted)
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
	cacheSeed(t, s, "p", cacheTrusted)
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
	cacheSeed(t, s, "q", cacheTrusted)
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

// TestCacheEntriesCommitNeverReplacesAnotherNamespace: the same key string in a
// second namespace must not delete or overwrite the first namespace's entry.
func TestCacheEntriesCommitNeverReplacesAnotherNamespace(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	other := entries.Facts{Project: "other", Trust: "owner", Protection: "protected"}
	_, fin := cacheFinalizing(t, s, "first", cacheTrusted, []string{cacheKey}, true)
	ns, _ := entries.NamespaceOf(cacheTrusted)
	if err := s.CacheCommit(ctx, entries.CommitRequest{Attempt: "first", Epoch: fin, Claimed: ns, Manifest: cacheManifest(t, cacheTrusted, cacheKey, 10)}); err != nil {
		t.Fatal(err)
	}
	_, fin = cacheFinalizing(t, s, "second", other, []string{cacheKey}, true)
	ons, _ := entries.NamespaceOf(other)
	err := s.CacheCommit(ctx, entries.CommitRequest{Attempt: "second", Epoch: fin, Claimed: ons, Manifest: cacheManifest(t, other, cacheKey, 20)})
	if err == nil {
		t.Fatal("a foreign namespace replaced an entry")
	}
	list, lerr := s.CacheEntries(ctx, cacheKey)
	if lerr != nil || len(list) != 1 || list[0].Writer.AttemptID != "first" || list[0].Bytes() != 10 {
		t.Fatalf("the original entry was disturbed: %+v %v", list, lerr)
	}
}

func reasonOf(t *testing.T, err error) string {
	t.Helper()
	var r *entries.Refused
	if !errors.As(err, &r) {
		t.Fatalf("not E711: %v", err)
	}
	return r.Reason
}

func cacheReq(t *testing.T, id string, epoch int64, f entries.Facts, key string, size int64) entries.CommitRequest {
	t.Helper()
	ns, _ := entries.NamespaceOf(f)
	return entries.CommitRequest{Attempt: id, Epoch: epoch, Claimed: ns, Manifest: cacheManifest(t, f, key, size)}
}

func TestCacheEntriesPlanMustMatchThePipeline(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	cacheSeed(t, s, "m", cacheTrusted)
	cacheStep(t, s, "m", "queued", "leased", 0)
	cacheStep(t, s, "m", "leased", "running", 1)
	for name, f := range map[string]entries.Facts{
		"other project": {Project: "other", Trust: "owner", Protection: "protected"},
		"higher trust":  {Project: "proj", Trust: "internal", Protection: "protected"},
	} {
		if got := reasonOf(t, s.CacheRecordPlan(ctx, "m", 2, f, nil)); got != "namespace" {
			t.Errorf("%s: %s", name, got)
		}
	}
	if cacheAuditCount(t, s, "cache.plan.refused", "m", "reason=namespace") != 2 {
		t.Fatal("each refused plan needs an audit row")
	}
}

// TestCacheEntriesResultAndPlanDoNotSurviveARerun: a pass recorded in one run
// never authorises a save in the next run of the same attempt id.
func TestCacheEntriesResultAndPlanDoNotSurviveARerun(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	const oldKey, newKey = cacheKey, "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2"
	cacheFinalizing(t, s, "r", cacheTrusted, []string{oldKey}, true)
	cacheStep(t, s, "r", "finalizing", "failed", 3)
	cacheStep(t, s, "r", "failed", "queued", 4)
	cacheStep(t, s, "r", "queued", "leased", 5)
	cacheStep(t, s, "r", "leased", "running", 6)
	cacheStep(t, s, "r", "running", "finalizing", 7)
	// the coordinator forgot RecordPlan and RecordResult for the new run
	if got := reasonOf(t, s.CacheCommit(ctx, cacheReq(t, "r", 8, cacheTrusted, oldKey, 10))); got != "attempt_state" {
		t.Fatalf("stale pass accepted: %s", got)
	}
	if got := reasonOf(t, s.CacheRecordResult(ctx, "r", 7, true)); got != "epoch" {
		t.Fatalf("result at a stale epoch: %s", got)
	}
	// a second run that plans afresh replaces the old key set
	cacheSeed(t, s, "r2", cacheTrusted)
	cacheStep(t, s, "r2", "queued", "leased", 0)
	cacheStep(t, s, "r2", "leased", "running", 1)
	if err := s.CacheRecordPlan(ctx, "r2", 2, cacheTrusted, []string{oldKey}); err != nil {
		t.Fatal(err)
	}
	if err := s.CacheRecordPlan(ctx, "r2", 2, cacheTrusted, []string{newKey}); err != nil {
		t.Fatal(err)
	}
	cacheStep(t, s, "r2", "running", "finalizing", 2)
	if err := s.CacheRecordResult(ctx, "r2", 3, true); err != nil {
		t.Fatal(err)
	}
	if got := reasonOf(t, s.CacheCommit(ctx, cacheReq(t, "r2", 3, cacheTrusted, oldKey, 10))); got != "not_planned" {
		t.Fatalf("replaced plan still authorises the old key: %s", got)
	}
	if err := s.CacheCommit(ctx, cacheReq(t, "r2", 3, cacheTrusted, newKey, 10)); err != nil {
		t.Fatal(err)
	}
	// a failing result drops the planned set
	cacheFinalizing(t, s, "bad", cacheTrusted, []string{oldKey}, true)
	if err := s.CacheRecordResult(ctx, "bad", 3, false); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.readers.QueryRow("SELECT count(*) FROM cache_planned WHERE attempt_id='bad'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("planned keys after a failing result: %d %v", n, err)
	}
}

// TestCacheEntriesUseTheLeaseFence: an attempt with a durable lease fence (the
// remote path) is gated on that fence, not on the transition counter.
func TestCacheEntriesUseTheLeaseFence(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	cacheSeed(t, s, "f", cacheTrusted)
	fence, err := s.LeaseGrant(ctx, "host", Demand{AttemptID: "f", LeaseID: "lease-f", RunnerID: "node", CPU: 1, MemMB: 1, CapacityCPU: 10, CapacityMemMB: 10, TTL: time.Minute}, 1)
	if err != nil || fence <= 1<<32 {
		t.Fatalf("fence %d %v", fence, err)
	}
	cacheStep(t, s, "f", "leased", "running", 1)
	if got := reasonOf(t, s.CacheRecordPlan(ctx, "f", 2, cacheTrusted, []string{cacheKey})); got != "epoch" {
		t.Fatalf("the transition counter must not pass for a fenced attempt: %s", got)
	}
	if err = s.CacheRecordPlan(ctx, "f", fence, cacheTrusted, []string{cacheKey}); err != nil {
		t.Fatal(err)
	}
	if err = s.LeaseResult(ctx, "lease-f", fence); err != nil {
		t.Fatal(err)
	}
	if err = s.CacheRecordResult(ctx, "f", fence, true); err != nil {
		t.Fatal(err)
	}
	if got := reasonOf(t, s.CacheCommit(ctx, cacheReq(t, "f", 3, cacheTrusted, cacheKey, 10))); got != "epoch" {
		t.Fatalf("commit with the transition counter: %s", got)
	}
	if got := reasonOf(t, s.CacheCommit(ctx, cacheReq(t, "f", fence-1, cacheTrusted, cacheKey, 10))); got != "epoch" {
		t.Fatalf("commit with an older fence: %s", got)
	}
	if err = s.CacheCommit(ctx, cacheReq(t, "f", fence, cacheTrusted, cacheKey, 10)); err != nil {
		t.Fatal(err)
	}
}

func TestCacheEntriesCheckRunsOnlyAfterAuthorization(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	_, fin := cacheFinalizing(t, s, "c", cacheTrusted, []string{cacheKey}, true)
	calls := 0
	req := cacheReq(t, "c", fin, cacheTrusted, cacheKey, 10)
	req.Check = func() error { calls++; return &entries.Refused{Reason: "disk_floor"} }
	stale := req
	stale.Epoch = fin - 1
	if reasonOf(t, s.CacheCommit(ctx, stale)) != "epoch" || calls != 0 {
		t.Fatalf("Check ran before the epoch gate (%d calls)", calls)
	}
	foreign := req
	foreign.Manifest.Key = strings.Repeat("c", 64)
	if reasonOf(t, s.CacheCommit(ctx, foreign)) != "not_planned" || calls != 0 {
		t.Fatalf("Check ran before the planned-key check (%d calls)", calls)
	}
	if got := reasonOf(t, s.CacheCommit(ctx, req)); got != "disk_floor" || calls != 1 {
		t.Fatalf("Check refusal: %s (%d calls)", got, calls)
	}
	if cacheAuditCount(t, s, "cache.commit.refused", "c", "reason=disk_floor") != 1 {
		t.Fatal("a Check refusal needs an audit row")
	}
	boom := errors.New("boom")
	req.Check = func() error { return boom }
	if err := s.CacheCommit(ctx, req); !errors.Is(err, boom) {
		t.Fatalf("a plain Check error must abort the commit: %v", err)
	}
	if list, _ := s.CacheEntries(ctx, ""); len(list) != 0 {
		t.Fatal("an aborted commit left an entry")
	}
	req.Check = nil
	if err := s.CacheCommit(ctx, req); err != nil {
		t.Fatalf("the attempt must still be able to save: %v", err)
	}
}
