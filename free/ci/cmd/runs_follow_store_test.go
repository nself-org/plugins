package main

// Tests of `nself ci runs follow` for a run with no event file whose store
// holds an event log (D-1061): the stream is the stored records, not a rebuild.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/events"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func logEvents(t *testing.T, home, run string, recs ...events.Record) {
	t.Helper()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Error(err)
		return
	}
	defer func() { _ = st.Close() }()
	for _, r := range recs {
		if _, err = st.AppendEvent(context.Background(), run, string(r.Type), r.Ref()); err != nil {
			t.Error(err)
		}
	}
}

func rec(typ, job string) events.Record {
	return events.Record{Type: model.EventType(typ), Job: job, Data: map[string]any{}}
}

func TestRunsFollowServesTheStoredEventLog(t *testing.T) {
	t.Run("a finished foreground run replays what happened, planned included", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-log", "passed", time.Now(), map[string]string{"build": "passed"})
		logEvents(t, home, "r-log", rec("queued", ""), rec("planned", ""), rec("started", "build"), rec("check_result", "build"), rec("finished", ""))
		code, out, _ := capture(t, func() int { return runFollow([]string{"r-log", "--json"}) })
		types, seqs := parsed(t, out)
		if code != 0 || strings.Join(types, ",") != "queued,planned,started,check_result,finished,result" || !increasing(seqs) || seqs[0] != 1 {
			t.Fatalf("exit %d %v %v\n%s", code, types, seqs, out)
		}
		code, out, _ = capture(t, func() int { return runFollow([]string{"r-log", "--from-seq", "4"}) })
		if types, _ = parsed(t, out); code != 0 || strings.Join(types, ",") != "check_result,finished,result" {
			t.Fatalf("resume: exit %d %v", code, types)
		}
	})
	t.Run("a live run shows its jobs as they start", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-live", "running", time.Now(), map[string]string{"build": "running"})
		logEvents(t, home, "r-live", rec("queued", ""), rec("planned", ""))
		done := make(chan struct{})
		go func() {
			defer close(done)
			time.Sleep(80 * time.Millisecond)
			logEvents(t, home, "r-live", rec("started", "build"))
			time.Sleep(80 * time.Millisecond)
			logEvents(t, home, "r-live", rec("check_result", "build"), rec("finished", ""))
			finishRun(t, home, "r-live", "passed")
		}()
		defer func() { <-done }()
		code, out, _ := capture(t, func() int { return runFollow([]string{"r-live", "--json"}) })
		types, seqs := parsed(t, out)
		if code != 0 || strings.Join(types, ",") != "queued,planned,started,check_result,finished,result" || !increasing(seqs) {
			t.Fatalf("exit %d %v %v", code, types, seqs)
		}
	})
	t.Run("a run that ended without its finished record is closed from the run state", func(t *testing.T) {
		home := runsHome(t)
		old := storedGrace
		storedGrace = 50 * time.Millisecond
		t.Cleanup(func() { storedGrace = old })
		seedRun(t, home, "r-cutlog", "failed", time.Now(), map[string]string{"build": "failed"})
		logEvents(t, home, "r-cutlog", rec("queued", ""), rec("started", "build"))
		code, out, _ := capture(t, func() int { return runFollow([]string{"r-cutlog", "--json"}) })
		types, seqs := parsed(t, out)
		if code != 0 || strings.Join(types, ",") != "queued,started,finished,result" || !increasing(seqs) {
			t.Fatalf("exit %d %v %v", code, types, seqs)
		}
	})
}
