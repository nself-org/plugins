package run

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/pools/record"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func recordTestGate(f RecordFacts, err error) RecordGate {
	return RecordGate{Facts: func(context.Context, PublishRequest) (RecordFacts, error) { return f, err }}
}

func TestRecordGateFailsClosed(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	for _, g := range []RecordGate{{}, recordTestGate(RecordFacts{}, errors.New("unreadable")), recordTestGate(RecordFacts{Record: true}, nil),
		recordTestGate(RecordFacts{Record: true, Runners: []pools.Runner{{ID: "unknown"}}}, nil)} {
		if err := g.Gate(context.Background(), PublishRequest{}, report.Status{}); err == nil || !strings.Contains(err.Error(), "E809: not_a_record_pool") {
			t.Fatalf("gate did not fail closed: %v", err)
		}
	}
	f := RecordFacts{Record: true, Runners: []pools.Runner{{ID: "local", Source: pools.SourceLocal}}}
	if err := recordTestGate(f, nil).Gate(context.Background(), PublishRequest{}, report.Status{}); err != nil {
		t.Fatal(err)
	}
	// A forced placement contributes alongside an otherwise valid local attempt.
	f.Runners = append(f.Runners, pools.Runner{ID: "team", Source: pools.SourceCoordinator, Assigned: []string{"pool.team"}})
	if err := recordTestGate(f, nil).Gate(context.Background(), PublishRequest{}, report.Status{}); err == nil {
		t.Fatal("forced foreign placement posted")
	}
}

func TestDevRunNoRecord(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	f := RecordFacts{Runners: []pools.Runner{{ID: "local", Source: pools.SourceLocal}}}
	err := recordTestGate(f, nil).Gate(context.Background(), PublishRequest{}, report.Status{})
	if err == nil || !strings.Contains(err.Error(), "dev run") {
		t.Fatalf("dev posted: %v", err)
	}
}

func TestReportOwnerTeamNotPosted(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	f := RecordFacts{Record: true, ReportOwner: report.OwnerTeam, Role: report.RolePersonal,
		Runners: []pools.Runner{{ID: "local", Source: pools.SourceLocal}}}
	err := recordTestGate(f, nil).Gate(context.Background(), PublishRequest{}, report.Status{})
	if err == nil || !strings.Contains(err.Error(), report.SuppressedReason) {
		t.Fatalf("personal process posted a team result: %v", err)
	}
	f.Role = report.RoleTeam
	if err := recordTestGate(f, nil).Gate(context.Background(), PublishRequest{}, report.Status{}); err != nil {
		t.Fatal(err)
	}
}

func TestRecordGateLegacyDormant(t *testing.T) {
	t.Setenv("NSELF_V15", "0")
	if err := (RecordGate{}).Gate(context.Background(), PublishRequest{}, report.Status{}); err != nil {
		t.Fatal(err)
	}
}

func TestNonRecordRunnerExcludedBeforeAdmission(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	reg, err := trust.NewRegistry(record.Spec())
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("pin", trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{record.Key: json.RawMessage(`["team"]`)}})
	if err != nil {
		t.Fatal(err)
	}
	f := RecordFacts{Record: true, Policy: eff, Runners: []pools.Runner{
		{ID: "vps", Source: pools.SourceRegistry, Assigned: []string{"pool.mine"}},
		{ID: "team", Source: pools.SourceCoordinator, Assigned: []string{"pool.team"}},
	}}
	excluded := record.Excluded(record.FromPolicy(eff), f.Runners)
	if len(excluded["team"]) != 0 || excluded["vps"][0].Detail != record.Reason {
		t.Fatal(excluded)
	}
	w := sched.World{Exclusions: excluded, Runners: []sched.Runner{{ID: "vps"}}}
	got := sched.Place(w, []sched.Job{{Key: "j", Pipeline: sched.Pipeline{Record: true}}})
	if len(got.Placements) != 0 {
		t.Fatal("excluded runner admitted")
	}
	// A forced placement must also be stopped by the independent publish gate.
	if err := recordTestGate(f, nil).Gate(context.Background(), PublishRequest{}, report.Status{}); err == nil {
		t.Fatal("forced placement published")
	}
}

type recordReporter struct{ posted int }

func (r *recordReporter) Post(context.Context, report.Status) error { r.posted++; return nil }

func TestRecordGateStopsPostStatus(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	reporter := &recordReporter{}
	outbox := &report.Outbox{Store: st, Reporter: reporter, Owner: "team", Repo: "private"}
	sha := strings.Repeat("a", 40)
	seq, err := outbox.Reserve(context.Background(), sha)
	if err != nil {
		t.Fatal(err)
	}
	facts := RecordFacts{Record: true, Runners: []pools.Runner{{ID: "team", Source: pools.SourceCoordinator}}}
	f := finishIn{Rev: Revision{SHA: sha}, Hooks: Hooks{Publisher: attestPublisher{}, Auto: AutoHooks{Gates: []PublishGate{recordTestGate(facts, nil)}}},
		Post: statusPost{Reporter: reporter, Owner: "team", Repo: "private", Outbox: outbox, Seq: seq}}
	ev := model.Evidence{Binding: model.Binding{State: "bound"}, Result: "pass"}
	line, _ := (&Coordinator{}).postStatus(context.Background(), f, RunInfo{Revision: sha, Result: "pass", Evidence: ev}, ev)
	if reporter.posted != 0 || !strings.Contains(line, record.Reason) {
		t.Fatalf("forced placement posted: %s", line)
	}
	entries, err := st.OutboxUnposted(context.Background())
	if err != nil || len(entries) != 0 {
		t.Fatal("refused status became deliverable", err)
	}
}

func TestRecordGatePinnedContext(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	g := recordTestGate(RecordFacts{Record: true, Runners: []pools.Runner{{ID: "local", Source: pools.SourceLocal}}}, nil)
	g.Contexts = &report.ResultContexts{Read: func(context.Context, string, string) (bool, error) { return true, nil }}
	req := PublishRequest{Owner: "team", Repo: "private"}
	if err := g.Gate(context.Background(), req, report.Status{}); err == nil {
		t.Fatal("pinned result used nself-ci")
	}
	if err := g.Gate(context.Background(), req, report.Status{StatusContext: report.WatchContext}); err != nil {
		t.Fatal(err)
	}
}

func TestRecordRefusalPersistsEvidence(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err = st.CreatePipeline(ctx, store.PipelineRow{ID: "dev", Project: "project", Revision: "sha", Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	h := &recordHooks{}
	h.sessions.Store("dev", &Session{Store: st, RunID: "dev", Record: false})
	ev := model.Evidence{InputDigests: model.InputDigests{Config: "cfg"}, Result: "pass"}
	err = h.AfterRun(ctx, RunInfo{RunID: "dev", Revision: "sha", Evidence: ev})
	if err == nil || !strings.Contains(err.Error(), record.Reason) {
		t.Fatal(err)
	}
	docs, err := st.EvidenceByRevision(ctx, "sha", "cfg")
	if err != nil || len(docs) != 1 {
		t.Fatalf("refusal evidence missing: %v %v", docs, err)
	}
	var got model.Evidence
	if err := json.Unmarshal(docs[0].Body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Errors) != 1 || got.Errors[0].Code != record.Code || got.Errors[0].Detail != record.Reason {
		t.Fatal("refusal reason missing", got.Errors)
	}
}

func TestRecordRefusalAnnotatesRunEvidence(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err = st.CreatePipeline(ctx, store.PipelineRow{ID: "dev", Project: "project", Revision: "sha", Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	h := &recordHooks{}
	hooks := []AfterRunHook{h}
	if got := annotations(hooks, "dev"); len(got) != 0 {
		t.Fatal("annotated before any refusal", got)
	}
	h.sessions.Store("dev", &Session{Store: st, RunID: "dev", Record: false})
	if err = h.AfterRun(ctx, RunInfo{RunID: "dev", Revision: "sha", Evidence: model.Evidence{InputDigests: model.InputDigests{Config: "cfg"}, Result: "pass"}}); err == nil {
		t.Fatal("a dev run was not refused")
	}
	got := annotations(hooks, "dev")
	if len(got) != 1 || got[0].Code != record.Code || got[0].Detail != record.Reason {
		t.Fatal("the run's evidence misses the stored refusal", got)
	}
	if again := annotations(hooks, "dev"); len(again) != 0 {
		t.Fatal("refusal reported twice", again)
	}
}
