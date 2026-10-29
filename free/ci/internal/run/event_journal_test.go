package run

import (
	"context"
	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"strings"
	"testing"
	"time"
)

func TestCoordinatorJournalsForegroundProgress(t *testing.T) {
	r := newRig(t, map[string]model.Job{"build": sh("echo private-log-content")}, gitFake{})
	res := r.run(t)
	st := r.store(t)
	rows, err := st.RunEvents(context.Background(), res.Summary.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for i, b := range rows {
		rec, ok := detach.ParseLine(b)
		if !ok || rec.Seq != int64(i+1) {
			t.Fatalf("record %d: %s", i, b)
		}
		if strings.Contains(string(b), "private-log-content") {
			t.Fatal("journal retained log bytes")
		}
		types = append(types, rec.Type)
	}
	if !strings.Contains(strings.Join(types, ","), "started") || len(types) == 0 || types[len(types)-1] != "result" {
		t.Fatalf("missing progress/evidence: %v", types)
	}
}

func TestJournalMakesProgressVisibleBeforeRunEnds(t *testing.T) {
	r := newRig(t, map[string]model.Job{"build": sh("true")}, gitFake{})
	st := r.store(t)
	renderer := stampRun(journal(context.Background(), st, "live", NewRenderer(r.Out, Options{})), "live")
	renderer.Event(Event{Type: "started", Job: "build"})
	deadline := time.Now().Add(3 * time.Second)
	for {
		records, err := st.RunEvents(context.Background(), "live")
		if err != nil {
			t.Fatal(err)
		}
		if len(records) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live progress missing: %s", records)
		}
		time.Sleep(time.Millisecond)
	}
	renderer.Done(Summary{RunID: "live"})
}
