package drill

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type fakeSource struct {
	def    model.PipelineDef
	passes map[string]time.Time
}

func (f fakeSource) Pipeline(context.Context, string) (model.PipelineDef, error) { return f.def, nil }
func (f fakeSource) LastPasses(context.Context, string, []string) (map[string]time.Time, error) {
	return f.passes, nil
}

var epoch = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func report(t *testing.T, src fakeSource) evidence.PipelineReport {
	t.Helper()
	rep, err := Reporter{Source: src, Now: func() time.Time { return epoch }}.Report(context.Background(), "drills")
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func jobStatus(t *testing.T, rep evidence.PipelineReport, job string) JobStatus {
	t.Helper()
	for _, s := range rep.Detail.(Detail).Jobs {
		if s.Job == job {
			return s
		}
	}
	t.Fatalf("no status for %s", job)
	return JobStatus{}
}

// TestOverdue: a job whose last pass is 37 h old in a 36 h pipeline is
// overdue with its reason (the command maps Overdue to exit 10).
func TestOverdue(t *testing.T) {
	def := model.PipelineDef{MaxAge: "36h", Jobs: []string{"drill-old", "drill-new", "drill-never"}}
	rep := report(t, fakeSource{def: def, passes: map[string]time.Time{
		"drill-old": epoch.Add(-37 * time.Hour),
		"drill-new": epoch.Add(-35*time.Hour - 59*time.Minute),
	}})
	if !rep.Overdue {
		t.Fatal("a pipeline with an overdue job must be overdue")
	}
	old := jobStatus(t, rep, "drill-old")
	if !old.Overdue || old.Code != CodeOverdue || !strings.Contains(old.Reason, "37h0m0s") || !strings.Contains(old.Reason, "36h0m0s") {
		t.Errorf("37 h old: %+v", old)
	}
	if fresh := jobStatus(t, rep, "drill-new"); fresh.Overdue || fresh.LastPass == nil {
		t.Errorf("just inside max_age: %+v", fresh)
	}
	never := jobStatus(t, rep, "drill-never")
	if !never.Overdue || never.LastPass != nil || !strings.Contains(never.Reason, "no passing run") {
		t.Errorf("never passed: %+v", never)
	}
}

func TestOverdueEdges(t *testing.T) {
	// Exactly max_age is not overdue; no max_age is never overdue.
	rep := report(t, fakeSource{def: model.PipelineDef{MaxAge: "36h", Jobs: []string{"a"}}, passes: map[string]time.Time{"a": epoch.Add(-36 * time.Hour)}})
	if rep.Overdue {
		t.Error("a pass exactly max_age old is not overdue")
	}
	rep = report(t, fakeSource{def: model.PipelineDef{Jobs: []string{"a"}}})
	if rep.Overdue || jobStatus(t, rep, "a").Overdue {
		t.Error("a pipeline with no max_age is never overdue")
	}
	for _, bad := range []string{"soon", "-1h", "0"} {
		_, err := Reporter{Source: fakeSource{def: model.PipelineDef{MaxAge: bad}}, Now: time.Now}.Report(context.Background(), "drills")
		var de *Error
		if !errors.As(err, &de) || de.Code != CodeInvalid {
			t.Errorf("max_age %q: err = %v, want E640", bad, err)
		}
	}
}

// TestOverdueFromStore records a real run of the fixture repo and reads its
// age back through the registered local source.
func TestOverdueFromStore(t *testing.T) {
	dir := fixtureRepo(t)
	t.Setenv("NSELF_CI_REPO", dir)
	p, _ := planDrills(t, dir)
	res := runDrills(t, dir, p)
	finished, err := time.Parse(time.RFC3339, *res.Summary.Doc.Timings.FinishedAt)
	if err != nil {
		t.Fatal(err)
	}
	at := func(d time.Duration) evidence.PipelineReport {
		r, err := Reporter{Source: localSource{}, Now: func() time.Time { return finished.Add(d) }}.Report(context.Background(), "drills")
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	rep := at(time.Hour)
	if s := jobStatus(t, rep, "drill-pass"); s.Overdue || s.LastPass == nil {
		t.Errorf("an hour after its pass: %+v", s)
	}
	for _, id := range []string{"drill-fail-nested", "drill-invalid"} {
		if s := jobStatus(t, rep, id); !s.Overdue || s.LastPass != nil {
			t.Errorf("%s never passed: %+v", id, s)
		}
	}
	if s := jobStatus(t, at(37*time.Hour), "drill-pass"); !s.Overdue || !strings.Contains(s.Reason, "37h") {
		t.Errorf("37 h after its pass: %+v", s)
	}
}

func TestReporterIsRegistered(t *testing.T) {
	found := false
	for _, n := range evidence.ReporterNames() {
		found = found || n == "*"
	}
	if !found {
		t.Fatal("the drill reporter must be registered at init for `ci evidence --pipeline`")
	}
}
