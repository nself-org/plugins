package report

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const sha2 = "fedcba9876543210fedcba9876543210fedcba98"

func TestBackoffSchedule(t *testing.T) {
	for _, c := range []struct {
		n    int
		rnd  float64
		want time.Duration
	}{
		{0, 0.5, 30 * time.Second}, {1, 0.5, time.Minute}, {4, 0.5, 8 * time.Minute}, {5, 0.5, 10 * time.Minute},
		{40, 0.5, 10 * time.Minute}, {0, 0, 24 * time.Second}, {0, 1, 36 * time.Second}, {5, 1, 8 * time.Minute},
		{5, 0, 8 * time.Minute}, {5, 0.75, 9 * time.Minute}, {40, 0.9, 8*time.Minute + 24*time.Second},
	} {
		if got := Backoff(c.n, c.rnd); got != c.want {
			t.Fatalf("Backoff(%d, %v) = %v, want %v", c.n, c.rnd, got, c.want)
		}
	}
}

// TestRetryCollapse proves "with the fake reporter failing 3 times then
// recovering, the entry posts once after recovery with the final state;
// intermediate pending states for the same key are collapsed".
func TestRetryCollapse(t *testing.T) {
	ctx, rep := context.Background(), &recorder{failFirst: 3}
	d, clk := newSender(t, rep)
	first, _ := d.Reserve(ctx, "o", "r", sha)
	final, _ := d.Reserve(ctx, "o", "r", sha)
	if err := d.Complete(ctx, first, status(Pending, "running")); err != nil { // post 1 fails: parked 30 s
		t.Fatal(err)
	}
	if err := d.Complete(ctx, final, status(Success, "all green")); err != nil { // collapses the pending one; post 2 fails
		t.Fatal(err)
	}
	next := func(want time.Duration, calls int) {
		t.Helper()
		list, err := d.Store.OutboxList(ctx)
		if err != nil || len(list) != 1 || list[0].State != "failed" || rep.calls != calls {
			t.Fatalf("parked entry: %+v %v (calls %d, want %d)", list, err, rep.calls, calls)
		}
		if got := time.Duration(list[0].NextAttemptAt - clk.now().UnixNano()); got != want {
			t.Fatalf("next attempt in %v, want %v", got, want)
		}
	}
	next(30*time.Second, 2)
	if n, _ := d.Drain(ctx); n != 0 || rep.calls != 2 {
		t.Fatalf("nothing is due before the backoff: %d, %d calls", n, rep.calls)
	}
	clk.advance(31 * time.Second) // post 3 fails: parked 60 s
	if n, _ := d.Drain(ctx); n != 0 {
		t.Fatalf("third attempt must fail: %d", n)
	}
	next(time.Minute, 3)
	clk.advance(61 * time.Second) // post 4 succeeds
	if n, err := d.Drain(ctx); n != 1 || err != nil {
		t.Fatalf("recovery: %d %v", n, err)
	}
	if len(rep.posted) != 1 || rep.posted[0].State != Success || rep.posted[0].Description != "all green" {
		t.Fatalf("the reporter must see one post, the final state: %+v", rep.posted)
	}
	if left, _ := d.Store.OutboxList(ctx); len(left) != 0 {
		t.Fatalf("outbox not empty after recovery: %+v", left)
	}
}

// TestReverseOrder proves "two pipelines for one SHA complete newer-first
// then older; the posted final state is the newer one", also when the newer
// post is parked for a retry while the older one completes.
func TestReverseOrder(t *testing.T) {
	ctx, rep := context.Background(), &recorder{}
	d, _ := newSender(t, rep)
	older, _ := d.Reserve(ctx, "o", "r", sha)
	newer, _ := d.Reserve(ctx, "o", "r", sha)
	if err := d.Complete(ctx, newer, status(Success, "new")); err != nil {
		t.Fatal(err)
	}
	if err := d.Complete(ctx, older, status(Failure, "old")); err != nil {
		t.Fatal(err)
	}
	if len(rep.posted) != 1 || rep.posted[0].Description != "new" {
		t.Fatalf("posted %+v", rep.posted)
	}

	rep2 := &recorder{failFirst: 1}
	d2, clk := newSender(t, rep2)
	o2, _ := d2.Reserve(ctx, "o", "r", sha2)
	n2, _ := d2.Reserve(ctx, "o", "r", sha2)
	s := func(st State, desc string) Status { x := status(st, desc); x.Revision = sha2; return x }
	_ = d2.Complete(ctx, n2, s(Success, "new")) // parked
	_ = d2.Complete(ctx, o2, s(Failure, "old")) // must not overtake the parked newer one
	if len(rep2.posted) != 0 {
		t.Fatalf("the older status posted past a parked newer one: %+v", rep2.posted)
	}
	clk.advance(time.Minute)
	if _, err := d2.Drain(ctx); err != nil || len(rep2.posted) != 1 || rep2.posted[0].Description != "new" {
		t.Fatalf("after the retry: %+v %v", rep2.posted, err)
	}
}

// TestReportOwner proves "with providers.report_owner team, a PERSONAL
// process records the entry as suppressed and posts nothing; with RoleTeam
// set the same entry posts" and that the outbox lists the reason.
func TestReportOwner(t *testing.T) {
	ctx := context.Background()
	team := func(context.Context, Status) string { return OwnerTeam }
	t.Cleanup(func() { SetProcessRole(RolePersonal) })

	SetProcessRole(RolePersonal)
	rep := &recorder{}
	d, _ := newSender(t, rep)
	d.Owner = team
	seq, _ := d.Reserve(ctx, "o", "r", sha)
	if err := d.Complete(ctx, seq, status(Success, "ok")); err != nil {
		t.Fatal(err)
	}
	list, err := d.Store.OutboxList(ctx)
	if err != nil || len(rep.posted) != 0 || len(list) != 1 || list[0].State != "suppressed" || list[0].Reason != SuppressedReason {
		t.Fatalf("personal process: posted %+v, outbox %+v %v", rep.posted, list, err)
	}

	// The same store, drained again by a TEAM process: the suppressed entry posts.
	SetProcessRole(RoleTeam)
	if n, err := d.Drain(ctx); err != nil || n != 1 || len(rep.posted) != 1 {
		t.Fatalf("same store under RoleTeam: n=%d posted=%+v %v", n, rep.posted, err)
	}
	if left, _ := d.Store.OutboxList(ctx); len(left) != 0 {
		t.Fatalf("revived entry still listed: %+v", left)
	}

	rep2 := &recorder{}
	d2, _ := newSender(t, rep2)
	d2.Owner = team
	seq, _ = d2.Reserve(ctx, "o", "r", sha)
	if err = d2.Complete(ctx, seq, status(Success, "ok")); err != nil || len(rep2.posted) != 1 {
		t.Fatalf("team process: posted %+v %v", rep2.posted, err)
	}

	SetProcessRole(RolePersonal) // a personal project: a personal process posts
	rep3 := &recorder{}
	d3, _ := newSender(t, rep3)
	seq, _ = d3.Reserve(ctx, "o", "r", sha)
	if err = d3.Complete(ctx, seq, status(Success, "ok")); err != nil || len(rep3.posted) != 1 {
		t.Fatalf("personal owner: posted %+v %v", rep3.posted, err)
	}
}

// TestRepost proves "report.Repost with a fake revocation set of two keys
// whose last posted state is success posts error with the revocation reason
// and a new seq for both; an older completion arriving later does not
// overwrite it".
func TestRepost(t *testing.T) {
	ctx, rep := context.Background(), &recorder{}
	d, _ := newSender(t, rep)
	revs := []string{sha, sha2}
	late := map[string]int64{}
	var keys []Key
	for _, rev := range revs {
		late[rev], _ = d.Reserve(ctx, "o", "r", rev) // an older pipeline still running
		seq, _ := d.Reserve(ctx, "o", "r", rev)
		s := status(Success, "ok")
		s.Revision = rev
		if err := d.Complete(ctx, seq, s); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, KeyFor("o", "r", rev))
	}
	rep.posted = nil
	if err := d.Repost(ctx, keys, Error, "node key revoked: n1"); err != nil {
		t.Fatal(err)
	}
	if len(rep.posted) != 2 {
		t.Fatalf("want one error post per key, got %+v", rep.posted)
	}
	for _, p := range rep.posted {
		if p.State != Error || p.Description != "node key revoked: n1" {
			t.Fatalf("replacement = %+v", p)
		}
	}
	for _, rev := range revs {
		s := status(Failure, "old pipeline")
		s.Revision = rev
		if err := d.Complete(ctx, late[rev], s); err != nil {
			t.Fatal(err)
		}
	}
	if len(rep.posted) != 2 {
		t.Fatalf("an older completion overwrote the replacement: %+v", rep.posted)
	}
	if err := d.Repost(ctx, []Key{{Revision: sha, Context: "other"}}, Error, "x"); err == nil {
		t.Fatal("a key that is not a status key is refused")
	}
	if err := d.Repost(ctx, keys[:1], State("bogus"), "x"); err == nil {
		t.Fatal("an invalid state is refused")
	}
}

// TestDrainParksUnreadableEntries: a payload nobody can read is parked where
// `ci runs outbox` shows it, and it does not stop the other streams.
func TestDrainParksUnreadableEntries(t *testing.T) {
	ctx, rep := context.Background(), &recorder{}
	d, _ := newSender(t, rep)
	if _, err := d.Store.OutboxEnqueue(ctx, sha2, key("o", "bad"), []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	seq, _ := d.Reserve(ctx, "o", "r", sha)
	if err := d.Complete(ctx, seq, status(Success, "ok")); err != nil {
		t.Fatal(err)
	}
	list, err := d.Store.OutboxList(ctx)
	if err != nil || len(rep.posted) != 1 || len(list) != 1 || list[0].State != "suppressed" || list[0].Reason != "unreadable payload" {
		t.Fatalf("posted %+v, outbox %+v %v", rep.posted, list, err)
	}
}

// TestDrainRevivesWhatAnUnreadableEntrySuperseded: the readable status below
// an unreadable one still reaches the host.
func TestDrainRevivesWhatAnUnreadableEntrySuperseded(t *testing.T) {
	ctx, rep := context.Background(), &recorder{}
	d, _ := newSender(t, rep)
	good, _ := json.Marshal(status(Failure, "readable"))
	if _, err := d.Store.OutboxEnqueue(ctx, sha, key("o", "r"), good); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Store.OutboxEnqueue(ctx, sha, key("o", "r"), []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	if n, err := d.Drain(ctx); err != nil || n != 1 || len(rep.posted) != 1 || rep.posted[0].Description != "readable" {
		t.Fatalf("n=%d posted=%+v %v", n, rep.posted, err)
	}
	if list, _ := d.Store.OutboxList(ctx); len(list) != 1 || list[0].Reason != "unreadable payload" {
		t.Fatalf("the unreadable entry stays visible: %+v", list)
	}
}

func TestCauseHasNoControlCharacters(t *testing.T) {
	got := cause(errors.New("boom \x1b[31mred\x1b[0m\rtail\nsecond line"))
	if strings.ContainsAny(got, "\x1b\r\n") || !strings.Contains(got, "boom") || strings.Contains(got, "second") {
		t.Fatalf("cause = %q", got)
	}
	if n := len([]rune(cause(errors.New(strings.Repeat("é", 500))))); n != 200 {
		t.Fatalf("cause length = %d runes", n)
	}
}
