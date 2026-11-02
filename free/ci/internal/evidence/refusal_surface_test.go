package evidence

// Purpose: pin that `evidence verify <rev>` answers the same document whether
// it had to run the pipeline or found the run already recorded (D-1519).
// A hook may append a later document to a run, like the record-pool gate's
// E809 refusal; that refusal must show on the first call as on every later one.

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// refusingEnv wraps the rig's run engine so every run also stores the later
// "ev-record-<run>" document with E809 appended, as recordHooks.AfterRun does.
func refusingEnv(t *testing.T, r *rig) Env {
	env := r.env()
	inner := env.Run
	env.Run = func(ctx context.Context, tree string) (model.Evidence, error) {
		ev, err := inner(ctx, tree)
		if err != nil {
			return ev, err
		}
		row, found, rerr := r.st.PipelineByID(ctx, ev.RunID)
		if rerr != nil || !found {
			t.Fatalf("run row %s: %v %v", ev.RunID, found, rerr)
		}
		later := ev
		later.Errors = append(append([]model.EvidenceError(nil), ev.Errors...), model.EvidenceError{Code: "E809", Detail: "not_a_record_pool"})
		body, merr := json.Marshal(later)
		ok(t, merr)
		ok(t, r.st.RecordRunEvidence(ctx, "ev-record-"+ev.RunID, ev.RunID, ev.Revision, row.InputDigest, body))
		return ev, nil // what the engine hands back is the run's own document, without the refusal
	}
	return env
}

func TestVerifyRevisionFirstAndLaterAnswersAgreeOnARecordedRefusal(t *testing.T) {
	r, _ := newRig(t)
	sha := commitFiles(t, r.dir, map[string]string{"a.txt": "b\n"}) // no evidence held for it: the first call runs
	env := refusingEnv(t, r)
	first, ran, err := VerifyRevision(context.Background(), env, sha)
	ok(t, err)
	if !ran || r.runs != 1 {
		t.Fatalf("first call: ran=%v runs=%d, want a run", ran, r.runs)
	}
	if errs := first.Report.Evidence.Errors; len(errs) != 1 || errs[0].Code != "E809" || errs[0].Detail != "not_a_record_pool" {
		t.Fatalf("the refusal vanished from the first answer: %+v", errs)
	}
	later, ran, err := VerifyRevision(context.Background(), env, sha)
	ok(t, err)
	if ran || r.runs != 1 {
		t.Fatalf("second call ran again: ran=%v runs=%d", ran, r.runs)
	}
	if !reflect.DeepEqual(first.Report, later.Report) || first.Exit != later.Exit {
		t.Fatalf("first and later answers differ:\nfirst: %+v\nlater: %+v", first.Report, later.Report)
	}
}

// Two documents of one run stored in the same instant still order the same
// way every time: the later-written record document sorts first.
func TestSortStoredTieBreaksOnID(t *testing.T) {
	l := []Stored{{ID: "ev-r1", Created: 5}, {ID: "ev-record-r1", Created: 5}, {ID: "ev-r0", Created: 5}}
	sortStored(l)
	if l[0].ID != "ev-record-r1" || l[1].ID != "ev-r1" || l[2].ID != "ev-r0" {
		t.Fatalf("order %v", []string{l[0].ID, l[1].ID, l[2].ID})
	}
}
