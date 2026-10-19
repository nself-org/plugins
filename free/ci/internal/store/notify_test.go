package store

import (
	"context"
	"testing"
	"time"
)

func TestNotifyClaimOncePerKey(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	for i, want := range []bool{true, false} {
		if got, err := s.NotifyClaim(ctx, "r", "sha1", "nself-ci", "failure"); err != nil || got != want {
			t.Fatalf("claim %d = %v %v, want %v", i, got, err, want)
		}
	}
	for _, c := range [][3]string{{"sha2", "nself-ci", "failure"}, {"sha1", "other", "failure"}, {"sha1", "nself-ci", "recovery"}} {
		if got, _ := s.NotifyClaim(ctx, "r", c[0], c[1], c[2]); !got {
			t.Fatalf("%v is a different key and must claim", c)
		}
	}
	if err := s.NotifyRelease(ctx, "r", "sha1", "nself-ci", "failure"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.NotifyClaim(ctx, "r", "sha1", "nself-ci", "failure"); !got {
		t.Fatal("a released claim can be taken again")
	}
}

func TestNotifyObserveReturnsPrevious(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	for i, c := range []struct{ ref, result, want string }{
		{"main", "fail", ""}, {"main", "pass", "fail"}, {"main", "pass", "pass"}, {"topic", "pass", ""},
	} {
		got, err := s.NotifyObserve(ctx, "r", c.ref, "nself-ci", "sha", c.result)
		if err != nil || got != c.want {
			t.Fatalf("step %d: previous = %q %v, want %q", i, got, err, c.want)
		}
	}
}

func TestNotifyDigestClaimAndStats(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	s, err := Open(t.TempDir()+"/state.db", Options{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	if got, _ := s.NotifyDigestClaim(ctx, "2026-10-19"); !got {
		t.Fatal("first claim of a day wins")
	}
	if got, _ := s.NotifyDigestClaim(ctx, "2026-10-19"); got {
		t.Fatal("second claim of the same day loses")
	}
	if at, err := s.NotifyLastDigest(ctx); err != nil || at != now.UnixNano() {
		t.Fatalf("last digest = %d %v", at, err)
	}
	if err := s.NotifyDigestRelease(ctx, "2026-10-19"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.NotifyDigestClaim(ctx, "2026-10-19"); !got {
		t.Fatal("a released day can be claimed again")
	}
	seed(t, s, "a")
	for _, st := range []string{"passed", "failed", "error"} {
		id := "p" + st
		if err := s.CreatePipeline(ctx, PipelineRow{ID: id, Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
			t.Fatal(err)
		}
		if err := s.FinishPipeline(ctx, id, st); err != nil {
			t.Fatal(err)
		}
	}
	d, err := s.NotifyDigestStats(ctx, now.Add(-time.Hour).UnixNano(), now.Add(time.Hour).UnixNano())
	if err != nil || d != (DigestStats{Runs: 3, Passed: 1, Failed: 1, Errored: 1, Queued: 1}) {
		t.Fatalf("stats = %+v %v", d, err)
	}
	if d, _ = s.NotifyDigestStats(ctx, now.Add(time.Hour).UnixNano(), now.Add(2*time.Hour).UnixNano()); d.Runs != 0 {
		t.Fatalf("a window with no finished run counts none: %+v", d)
	}
}

func TestNotifySinkCounters(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	for _, ok := range []bool{true, true, false} {
		if err := s.NotifyRecordDelivery(ctx, "ops-hook", ok); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.NotifyRecordDelivery(ctx, "a-first", true)
	got, err := s.NotifySinkStats(ctx)
	if err != nil || len(got) != 2 || got[0].Sink != "a-first" || got[1].Delivered != 2 || got[1].Failed != 1 || got[1].LastFailedAt == 0 || got[0].LastFailedAt != 0 {
		t.Fatalf("stats = %+v %v", got, err)
	}
}

func TestNotifyRetentionAndQueueScope(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	s, err := Open(t.TempDir()+"/state.db", Options{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	_, _ = s.NotifyClaim(ctx, "r", "old", "c", "failure")
	_, _ = s.NotifyObserve(ctx, "r", "main", "c", "old", "fail")
	now = now.Add(notifyRetention + time.Hour)
	if got, _ := s.NotifyClaim(ctx, "r", "old", "c", "failure"); !got {
		t.Fatal("a claim older than the retention is dropped")
	}
	if prev, _ := s.NotifyObserve(ctx, "r", "main", "c", "new", "pass"); prev != "" {
		t.Fatalf("a last-result row older than the retention is dropped, got %q", prev)
	}
	// the digest's queue counts jobs of unfinished runs only
	seed(t, s, "a")
	if err = s.FinishPipeline(ctx, "pa", "failed"); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.NotifyDigestStats(ctx, 0, now.Add(time.Hour).UnixNano()); d.Queued != 0 {
		t.Fatalf("a closed run's queued job is not queue: %+v", d)
	}
}
