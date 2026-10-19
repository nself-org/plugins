package run

// Tests of the stored event log (D-1061): every run, foreground or not, leaves
// its contract:ci.events records in the store, whatever renderer it uses.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/events"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestRunStoresItsEventLog(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("true"), "b": sh("false")}, gitFake{})
	res := r.run(t)
	rows, err := r.store(t).EventRows(context.Background(), res.Summary.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for i, row := range rows {
		if row.Seq != int64(i+1) {
			t.Fatalf("row %d has seq %d", i, row.Seq)
		}
		rec, err := events.ParseRecord(row.Type, row.Ref)
		if err != nil {
			t.Fatalf("row %d: %v", i, err)
		}
		types = append(types, string(rec.Type))
		if rec.Type == "finished" && rec.Data["result"] != string(res.Summary.Result) {
			t.Fatalf("finished record %+v, run result %s", rec.Data, res.Summary.Result)
		}
	}
	if len(types) < 7 || types[0] != "queued" || types[1] != "planned" || types[len(types)-1] != "finished" {
		t.Fatalf("event log %v", types)
	}
	for _, want := range []string{"started", "check_result"} {
		if n := countOf(types, want); n != 2 {
			t.Fatalf("%d %s records in %v, want one per job", n, want, types)
		}
	}
}

func countOf(s []string, v string) int {
	n := 0
	for _, x := range s {
		if x == v {
			n++
		}
	}
	return n
}
