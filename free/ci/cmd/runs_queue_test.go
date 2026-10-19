package main

// Purpose: tests of `nself ci runs queue` (P7-SCHED-06): the order, band,
// position and expected wait of the queued attempts a store holds.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// queueFixture queues one attempt per entry, newest pipeline last.
func queueFixture(t *testing.T, home string, jobs [][3]string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	for _, j := range jobs { // project, trigger, job name
		pipe := "run-" + j[0] + "-" + j[2]
		if err = st.CreatePipeline(ctx, store.PipelineRow{ID: pipe, Revision: "rev", Project: j[0], Trigger: j[1], SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
			t.Fatal(err)
		}
		if err = st.CreateJob(ctx, store.JobRow{ID: "job-" + pipe, PipelineID: pipe, Name: j[2], Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
			t.Fatal(err)
		}
		if err = st.CreateAttempt(ctx, store.AttemptRow{ID: "att-" + pipe, JobID: "job-" + pipe, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunsQueueListsPositionBandAndWait(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	queueFixture(t, home, [][3]string{{"alpha", "push", "a1"}, {"alpha", "push", "a2"}, {"beta", "schedule", "nightly"}, {"beta", "push", "b1"}, {"alpha", "manual", "hand"}})
	code, out, errs := capture(t, func() int { return runRuns([]string{"queue", "--json"}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	var env struct {
		Data []queueView `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	var got []string
	for i, q := range env.Data {
		got = append(got, q.Band+":"+q.Project+"/"+q.Job)
		if q.Position != i+1 || q.WaitConfidence != "unknown" || q.ExpectedWaitMs < 0 {
			t.Fatalf("row %d: %+v", i, q)
		}
	}
	// interactive (manual) first; normal pushes alternate between the projects; the schedule is background
	want := "interactive:alpha/hand normal:alpha/a1 normal:beta/b1 normal:alpha/a2 background:beta/nightly"
	if strings.Join(got, " ") != want {
		t.Fatalf("order\n got %v\nwant %s", got, want)
	}
	if env.Data[0].ExpectedWaitMs != 0 || env.Data[4].ExpectedWaitMs <= env.Data[1].ExpectedWaitMs {
		t.Fatalf("expected wait must grow with the position: %+v", env.Data)
	}
}

func TestRunsQueueEmptyAndUsage(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	if code, out, _ := capture(t, func() int { return runRuns([]string{"queue"}) }); code != 0 || !strings.Contains(out, "no jobs are waiting") {
		t.Fatalf("empty: %d %q", code, out)
	}
	code, out, _ := capture(t, func() int { return runRuns([]string{"queue", "--json"}) })
	var env struct {
		Data []queueView `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); code != 0 || err != nil || env.Data == nil || len(env.Data) != 0 {
		t.Fatalf("empty json: %d %v %q", code, err, out)
	}
	if code, _, _ = capture(t, func() int { return runRuns([]string{"queue", "extra"}) }); code != 1 {
		t.Fatalf("usage exit %d", code)
	}
}

func TestQueueRowsRunAge(t *testing.T) {
	home := t.TempDir()
	queueFixture(t, home, [][3]string{{"alpha", "schedule", "old"}})
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	rows, err := queueRows(context.Background(), st, time.Now().Add(31*time.Minute), 1)
	// the store has no per-attempt enqueue time, so a long-lived run never shows as promoted
	if err != nil || len(rows) != 1 || rows[0].Band != "background" || rows[0].RunAgeMs < 31*60*1000 {
		t.Fatalf("run age without promotion: %+v %v", rows, err)
	}
}
