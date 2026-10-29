package selftest_test

// Purpose: the audit pipeline template (P7-AUTO-20) is a valid pipeline config
// whose only trigger is `schedule` (so it runs in the background band), and the
// watcher's idle tick, which starts it, starts nothing on a host that is not ready.
// Inputs: free/ci/templates/audit-pipeline.yml; a watcher with stub ports and no
// watched repositories. Outputs: pass or fail. Constraints: no network, no clock.

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/background"
	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/watch"
)

const auditTemplate = "../../templates/audit-pipeline.yml"

// stubs satisfy the watcher's required ports; a pass with no repositories never calls them.
type stubs struct{ t *testing.T }

func (s stubs) Scan(context.Context, string) (watch.Scan, error) {
	s.t.Fatal("no repository is watched, nothing may be scanned")
	return watch.Scan{}, nil
}
func (s stubs) PostClaim(context.Context, report.Status) (time.Time, error) {
	s.t.Fatal("an idle pass must not claim")
	return time.Time{}, nil
}
func (s stubs) ListStatuses(context.Context, string, string, string) ([]report.LedgerStatus, time.Time, error) {
	return nil, time.Time{}, nil
}
func (s stubs) Post(context.Context, report.Status) error {
	s.t.Fatal("an idle pass must not post a status")
	return nil
}
func (s stubs) Run(context.Context, watch.Job) (watch.Outcome, error) {
	s.t.Fatal("an idle pass must not run a commit")
	return watch.Outcome{}, nil
}

func TestAuditTemplateValid(t *testing.T) {
	data, err := os.ReadFile(auditTemplate)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Parse(auditTemplate, data)
	if err != nil {
		t.Fatalf("the template must pass the pipeline-config schema: %v", err)
	}
	p, ok := loaded.Config.Pipelines["audit"]
	if !ok {
		t.Fatal("the template must define the audit pipeline")
	}
	if len(p.Triggers) != 1 || string(p.Triggers[0]) != "schedule" || p.Schedule == "" || p.MaxAge == "" {
		t.Fatalf("audit must be scheduled only, with a schedule and a max_age: %+v", p)
	}
	if !background.IsBackgroundKind(string(p.Triggers[0])) {
		t.Fatal("a scheduled pipeline must run in the background band")
	}
	want := map[string]string{"audit-govulncheck": "govulncheck", "audit-pnpm": "pnpm", "audit-race": "go"}
	if len(p.Jobs) != len(want) {
		t.Fatalf("audit jobs = %v", p.Jobs)
	}
	for _, id := range p.Jobs {
		j, ok := loaded.Config.Jobs[id]
		if !ok || len(j.Run) == 0 || j.Run[0] != want[id] {
			t.Fatalf("job %s must run %s: %+v", id, want[id], j)
		}
	}
	if !slices.Contains(loaded.Config.Jobs["audit-race"].Run, "-race") {
		t.Fatal("the race job must pass -race")
	}

	// The drill: the idle tick is what starts a due background pipeline; a host that
	// is not ready never reaches it, a ready host with nothing to run does.
	for _, ready := range []bool{false, true} {
		started := 0
		s := stubs{t}
		w, err := watch.New(watch.Config{Host: "mini1"}, watch.Ports{Scanner: s, Ledger: s, Reporter: s, Runner: s,
			Ready: func(context.Context) (bool, []string) {
				if ready {
					return true, nil
				}
				return false, []string{"disk.low"}
			},
			Idle: func(context.Context) (bool, error) { started++; return true, nil },
			Now:  func() time.Time { return time.Date(2026, 10, 20, 2, 41, 0, 0, time.UTC) }})
		if err != nil {
			t.Fatal(err)
		}
		rep := w.Pass(context.Background())
		if ready && (started != 1 || !rep.Idle) {
			t.Fatalf("a ready idle host must start one background pipeline: started=%d %+v", started, rep)
		}
		if !ready && (started != 0 || rep.Idle) {
			t.Fatalf("a host that is not ready must start nothing: started=%d %+v", started, rep)
		}
	}
}
