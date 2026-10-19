package main

// Purpose: tests of `nself ci runs outbox` (P7-HOST-02): the suppressed entry
// is listed with its reason, a parked entry with its attempt and next time,
// and a machine without a store has an empty list.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const outboxSHA = "0123456789abcdef0123456789abcdef01234567"

type failingReporter struct{}

func (failingReporter) Post(context.Context, report.Status) error { return context.DeadlineExceeded }

func TestRunsOutboxListsSuppressedAndParked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	d := &report.Sender{Store: st, Registry: report.NewRegistry(failingReporter{}),
		Now:  func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) },
		Rand: func() float64 { return 0.5 },
		Owner: func(_ context.Context, s report.Status) string {
			if s.Repo == "teamed" {
				return report.OwnerTeam
			}
			return report.OwnerPersonal
		},
	}
	for _, repo := range []string{"teamed", "mine"} {
		seq, _ := d.Reserve(ctx, "o", repo, outboxSHA)
		if err = d.Complete(ctx, seq, report.Status{Owner: "o", Repo: repo, Revision: outboxSHA, State: report.Success, Description: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	code, out, errs := capture(t, func() int { return runRuns([]string{"outbox", "--json"}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	var env struct {
		Data []outboxView `json:"data"`
	}
	if err = json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	got := map[string]outboxView{}
	for _, v := range env.Data {
		got[v.Repo] = v
	}
	if v := got["o/teamed"]; v.State != "suppressed" || v.Reason != report.SuppressedReason {
		t.Fatalf("suppressed entry: %+v", v)
	}
	if v := got["o/mine"]; v.State != "failed" || v.Attempts != 1 || v.NextAttemptAt != "2026-10-19T12:00:30Z" || v.LastError == "" {
		t.Fatalf("parked entry: %+v", v)
	}
	if code, _, _ = capture(t, func() int { return runRuns([]string{"outbox", "--bogus"}) }); code != 1 {
		t.Fatalf("usage exit = %d", code)
	}
}

func TestRunsOutboxWithoutStoreIsEmpty(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	code, out, _ := capture(t, func() int { return runRuns([]string{"outbox", "--json"}) })
	var env struct {
		Data []outboxView `json:"data"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &env) != nil || len(env.Data) != 0 {
		t.Fatalf("exit %d, %s", code, out)
	}
}
