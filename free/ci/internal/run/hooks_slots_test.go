package run

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

type slotLog struct {
	mu      sync.Mutex
	calls   []string
	detail  []PublishDetailRequest
	notify  []NotifyRequest
	gateSaw []report.Status
}

func (l *slotLog) add(s string) { l.mu.Lock(); defer l.mu.Unlock(); l.calls = append(l.calls, s) }
func (l *slotLog) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.calls, ",")
}

type fakeGate struct {
	l   *slotLog
	err error
}

func (g fakeGate) Gate(_ context.Context, _ PublishRequest, s report.Status) error {
	g.l.add("gate")
	g.l.mu.Lock()
	g.l.gateSaw = append(g.l.gateSaw, s)
	g.l.mu.Unlock()
	return g.err
}

type fakeDetail struct{ l *slotLog }

func (d fakeDetail) Detail(_ context.Context, r PublishDetailRequest) error {
	d.l.add("detail")
	d.l.mu.Lock()
	d.l.detail = append(d.l.detail, r)
	d.l.mu.Unlock()
	return nil
}

type fakeOffload struct {
	l   *slotLog
	res OffloadResult
	err error
}

func (o fakeOffload) Offload(_ context.Context, r OffloadRequest) (OffloadResult, error) {
	o.l.add("offload:" + r.Rev.SHA[:7])
	return o.res, o.err
}

type fakeRace struct{ l *slotLog }

func (f fakeRace) Race(_ context.Context, r RaceRequest) {
	if r.RunID == "" || r.Plan == nil {
		f.l.add("race:bad-request")
		return
	}
	f.l.add("race")
}

type fakeNotify struct {
	l     *slotLog
	err   error
	panic bool
}

func (n fakeNotify) Notify(_ context.Context, r NotifyRequest) error {
	n.l.add("notify")
	n.l.mu.Lock()
	n.l.notify = append(n.l.notify, r)
	n.l.mu.Unlock()
	if n.panic {
		panic("sink exploded")
	}
	return n.err
}

func allSlots(l *slotLog) AutoHooks {
	return AutoHooks{Gates: []PublishGate{fakeGate{l: l}}, Details: []PublishDetail{fakeDetail{l}}, Offload: fakeOffload{l: l}, Race: fakeRace{l}, Notify: fakeNotify{l: l}}
}

// withRegistered swaps Registered.Auto for one test. The tests that call it
// are not parallel, so the parallel tests of the package are still paused.
func withRegistered(t *testing.T, a AutoHooks) {
	t.Helper()
	old := Registered.Auto
	Registered.Auto = a
	t.Cleanup(func() { Registered.Auto = old })
}

func passRig(t *testing.T) *rig {
	t.Helper()
	return newRig(t, map[string]model.Job{"a": sh("true"), "b": sh("true", "a")}, gitFake{})
}

// TestHookSlotsNoop: with nothing registered, and with slots that do nothing,
// the run posts the same status and ends with the same verdict.
func TestHookSlotsNoop(t *testing.T) {
	// the notify hook (P7-AUTO-18), the App checks detail publisher (P7-AUTO-22) and the pools.record
	// publish gate (P7-AUTO-15) are the slots the shipped run package fills; all are inert without v1.5
	// and their operator keys or pool records
	_, notifyOK := Registered.Auto.Notify.(*notifyHook)
	detailOK := len(Registered.Auto.Details) == 1
	if detailOK {
		_, detailOK = Registered.Auto.Details[0].(*checksDetail)
	}
	gateOK := len(Registered.Auto.Gates) == 1
	if gateOK {
		_, gateOK = Registered.Auto.Gates[0].(RecordGate)
	}
	if !gateOK || Registered.Auto.Offload != nil || Registered.Auto.Race != nil || !notifyOK || !detailOK {
		t.Fatal("the shipped build must register no AUTO slot but the notify hook, the checks detail publisher and the record gate")
	}
	base := passRig(t)
	want := base.run(t)
	wantPosted := base.Rep.all()
	if want.Exit != 0 || len(wantPosted) != 1 {
		t.Fatalf("baseline: exit %d posted %+v", want.Exit, wantPosted)
	}

	l := &slotLog{}
	r := passRig(t)
	r.C.Hooks.Auto = allSlots(l)
	res := r.run(t)
	posted := r.Rep.all()
	if res.Exit != want.Exit || res.Summary.Result != want.Summary.Result || len(posted) != 1 {
		t.Fatalf("idle slots changed the run:\n got %+v %+v\nwant %+v %+v", res.Summary, posted, want.Summary, wantPosted)
	}
	// the evidence digest in the description differs per run (run id, times); everything else must match
	if g, w := stripDigest(posted[0].Description), stripDigest(wantPosted[0].Description); g != w || posted[0].State != wantPosted[0].State || posted[0].Revision != wantPosted[0].Revision || posted[0].TargetURL != wantPosted[0].TargetURL {
		t.Fatalf("idle slots changed the status: %+v vs %+v", posted[0], wantPosted[0])
	}
	if !reflect.DeepEqual(res.Summary.Notes, want.Summary.Notes) || len(res.Summary.Jobs) != len(want.Summary.Jobs) {
		t.Fatalf("idle slots changed notes or jobs: %v vs %v", res.Summary.Notes, want.Summary.Notes)
	}
	for i, j := range res.Summary.Jobs {
		if w := want.Summary.Jobs[i]; j.ID != w.ID || j.State != w.State || j.Reason != w.Reason || j.ExitCode != w.ExitCode {
			t.Fatalf("job %d changed: %+v vs %+v", i, j, w)
		}
	}
	if got := l.joined(); got != "offload:0123456,race,gate,detail,notify" {
		t.Fatalf("call order = %s", got)
	}
	if len(l.detail) != 1 || l.detail[0].RunID != res.RunID || l.detail[0].Status.State != report.Success || len(l.detail[0].Evidence.Checks) == 0 || l.detail[0].Revision != testSHA {
		t.Fatalf("detail request: %+v", l.detail)
	}
	if got := l.detail[0].Evidence.Runner.ID; got == "" || got == "local" {
		t.Fatalf("detail evidence must be the published one (runner id pseudonymised), got runner id %q", got)
	}
	if len(l.notify) != 1 || l.notify[0].Run.Result != "pass" || !strings.Contains(l.notify[0].Status, "status posted") {
		t.Fatalf("notify request: %+v", l.notify)
	}
}

// Registered slots reach a run through Resolved, and a hook on the Coordinator wins a single slot.
func TestHookSlotsResolve(t *testing.T) {
	l := &slotLog{}
	withRegistered(t, AutoHooks{})
	RegisterPublishGate(fakeGate{l: l})
	RegisterPublishDetail(fakeDetail{l})
	RegisterOffload(fakeOffload{l: l})
	RegisterRace(fakeRace{l})
	RegisterNotify(fakeNotify{l: l})
	if !RaceRegistered() {
		t.Fatal("RaceRegistered must report the slot")
	}
	got := (Hooks{}).Resolved().Auto
	if len(got.Gates) != 1 || len(got.Details) != 1 || got.Offload == nil || got.Race == nil || got.Notify == nil {
		t.Fatalf("resolved: %+v", got)
	}
	own := &fakeOffload{l: l, err: errors.New("own")}
	got = Hooks{Auto: AutoHooks{Offload: own, Gates: []PublishGate{fakeGate{l: l}}}}.Resolved().Auto
	if got.Offload != OffloadHook(own) || len(got.Gates) != 2 {
		t.Fatalf("a coordinator hook must keep its single slot and add to lists: %+v", got)
	}
}

// TestPublishGateComposes: a PublishGate vetoes a post, and the Publisher's own veto still stands.
func TestPublishGateComposes(t *testing.T) {
	// the attest Publisher stays installed when a gate registers
	withRegistered(t, AutoHooks{})
	RegisterPublishGate(fakeGate{l: &slotLog{}})
	if (Hooks{}).Resolved().Publisher == nil {
		t.Fatal("registering a PublishGate must not remove the attest Publisher")
	}

	// gate allows: posted once, and the gate saw the status the Publisher built
	l := &slotLog{}
	r := passRig(t)
	r.C.Hooks.Auto.Gates = []PublishGate{fakeGate{l: l}}
	r.run(t)
	if p := r.Rep.all(); len(p) != 1 || len(l.gateSaw) != 1 || l.gateSaw[0].Description != p[0].Description || p[0].Description == "" {
		t.Fatalf("allowed: posted %+v, gate saw %+v", p, l.gateSaw)
	}

	// gate vetoes: nothing posted, the reason is shown, details are not published
	l = &slotLog{}
	r = passRig(t)
	r.C.Hooks.Auto = AutoHooks{Gates: []PublishGate{fakeGate{l: l, err: errors.New("not_a_record_pool (E809)")}}, Details: []PublishDetail{fakeDetail{l}}}
	res := r.run(t)
	if len(r.Rep.all()) != 0 || !strings.Contains(res.Summary.Status, "publish gate refused, nothing posted: not_a_record_pool") || res.Exit != 0 {
		t.Fatalf("veto: status %q posted %+v exit %d", res.Summary.Status, r.Rep.all(), res.Exit)
	}
	if strings.Contains(l.joined(), "detail") {
		t.Fatal("a vetoed post publishes no detail")
	}

	// the Publisher vetoes and the gate allows: still nothing posted, and the gate is not asked
	l = &slotLog{}
	r = passRig(t)
	r.C.Hooks.Publisher = errPublisher{}
	r.C.Hooks.Auto.Gates = []PublishGate{fakeGate{l: l}}
	res = r.run(t)
	if len(r.Rep.all()) != 0 || !strings.Contains(res.Summary.Status, "publisher failed") || l.joined() != "" {
		t.Fatalf("publisher veto: status %q posted %+v calls %s", res.Summary.Status, r.Rep.all(), l.joined())
	}

	// two gates: the second one still vetoes after the first allowed
	l = &slotLog{}
	r = passRig(t)
	r.C.Hooks.Auto.Gates = []PublishGate{fakeGate{l: l}, fakeGate{l: l, err: errors.New("second")}}
	r.run(t)
	if len(r.Rep.all()) != 0 || l.joined() != "gate,gate" {
		t.Fatalf("two gates: posted %+v calls %s", r.Rep.all(), l.joined())
	}
}

// TestOffloadAndNotifyFailures: an offload error refuses the run before anything runs; a
// snapshot revision is only what nodes check out (the run records HEAD, dirty and unbound);
// a failing or panicking notify hook is a note.
func TestOffloadAndNotifyFailures(t *testing.T) {
	l := &slotLog{}
	r := passRig(t)
	r.C.Hooks.Auto.Offload = fakeOffload{l: l, err: errors.New("E807: tracked secret file")}
	if _, err := r.C.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "E807") {
		t.Fatalf("offload error must refuse the run: %v", err)
	}
	if len(r.Rep.all()) != 0 || strings.Contains(l.joined(), "race") {
		t.Fatalf("a refused run starts nothing: %s", l.joined())
	}

	released := 0
	r = passRig(t)
	r.C.Hooks.Auto.Offload = fakeOffload{l: l, res: OffloadResult{Rev: Revision{SHA: strings.Repeat("ab", 20), Dirty: true}, Release: func() { released++ }}}
	res := r.run(t)
	if released != 1 || len(r.Rep.all()) != 0 || !strings.Contains(res.Summary.Status, "not posted") {
		t.Fatalf("a snapshot revision is dev evidence and is released once: released %d status %q", released, res.Summary.Status)
	}
	if res.Summary.Doc.Revision != testSHA {
		t.Fatalf("the run recorded %q: a snapshot is unreachable after release, so the run records HEAD %s", res.Summary.Doc.Revision, testSHA)
	}

	for name, n := range map[string]fakeNotify{"error": {l: l, err: errors.New("sink down")}, "panic": {l: l, panic: true}} {
		r = passRig(t)
		r.C.Hooks.Auto.Notify = n
		res = r.run(t)
		found := false
		for _, note := range res.Summary.Notes {
			found = found || strings.HasPrefix(note, "notify hook:")
		}
		if res.Exit != 0 || !found || len(r.Rep.all()) != 1 {
			t.Errorf("%s: a notify failure must be a note only: exit %d notes %v", name, res.Exit, res.Summary.Notes)
		}
	}
}

func stripDigest(d string) string {
	if i := strings.Index(d, " (evidence "); i >= 0 {
		return d[:i]
	}
	return d
}

// A snapshot revision from the offload hook never hides a config outside the tree, and
// Options.Record is a plain flag that defaults to a dev run.
func TestOffloadKeepsConfigOutsideDirty(t *testing.T) {
	if (Options{}).Record {
		t.Fatal("a bare run is not a run of record")
	}
	r := passRig(t)
	r.C.ConfigOutside = true
	r.C.Opts.Record = true
	r.C.Hooks.Auto.Offload = fakeOffload{l: &slotLog{}, res: OffloadResult{Rev: Revision{SHA: testSHA}}} // same SHA as HEAD, so only line "ConfigOutside" can keep the run unbound
	res := r.run(t)
	if len(r.Rep.all()) != 0 || !strings.Contains(res.Summary.Status, "not posted") {
		t.Fatalf("config outside the tree must keep the run unbound: %q %+v", res.Summary.Status, r.Rep.all())
	}
}

type captureResolver struct{ got *PolicyRequest }

func (c captureResolver) Resolve(ctx context.Context, r PolicyRequest) (PolicyResult, error) {
	*c.got = r
	return defaultResolver{}.Resolve(ctx, r)
}

// PolicyRequest.Run carries Options.PolicyRun to the resolver; a bare run sends none.
func TestPolicyRequestCarriesRunValues(t *testing.T) {
	var got PolicyRequest
	r := passRig(t)
	r.C.Hooks.Policy = captureResolver{&got}
	r.run(t)
	if got.Run != nil {
		t.Fatalf("a bare run sends no per-run values: %v", got.Run)
	}
	r = passRig(t)
	r.C.Hooks.Policy = captureResolver{&got}
	r.C.Opts.PolicyRun = RunValues{"sched.where": json.RawMessage(`"all"`)}
	r.run(t)
	if string(got.Run["sched.where"]) != `"all"` {
		t.Fatalf("per-run values must reach the resolver: %v", got.Run)
	}
}

// The evidence a detail publisher sees has retry text scrubbed like the rest (hostnames and user names).
func TestPublicationScrubsRetryText(t *testing.T) {
	hosts, users := localNames()
	secret := "no space left on device at /Users/" + strings.Join(append(append([]string(nil), users...), hosts...), " ")
	if len(hosts)+len(users) == 0 {
		t.Skip("no local host or user name to plant")
	}
	ev := model.Evidence{Schema: "ci.evidence/v1", RepoID: "r1", Runner: model.RunnerRef{ID: "build-box", Provider: "local"},
		Retries: []model.Retry{{JobID: "a", Attempt: 1, Diagnosis: secret, Excerpt: secret}}}
	got, _, _ := publication(PublishRequest{Owner: "o", Repo: "r", Run: RunInfo{Evidence: ev}})
	for _, text := range []string{got.Retries[0].Excerpt, got.Retries[0].Diagnosis} {
		for _, name := range append(append([]string(nil), users...), hosts...) {
			if strings.Contains(text, name) {
				t.Fatalf("retry text still holds %q: %q", name, text)
			}
		}
	}
	if ev.Retries[0].Excerpt != secret {
		t.Fatal("the unpublished evidence must not be modified")
	}
}
