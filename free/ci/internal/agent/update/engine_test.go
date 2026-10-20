package update

// Purpose: the state machine against a fake node: order of the steps, every refusal before any change, drain waiting, and the
//          rollback on each failure after the swap.
// Constraints: no disk, no network, no processes.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

type fakeSource struct {
	m   verify.Manifest
	err error
}

func (f fakeSource) Prepare(context.Context, lifecycle.Platform) (Prepared, error) {
	return Prepared{Path: "/staged", Manifest: f.m, Cleanup: func() {}}, f.err
}

type recAudit struct {
	steps, details []string
	failOn         string
}

func (r *recAudit) Step(_ context.Context, step, node, detail string) error {
	if step == r.failOn {
		return errors.New("audit store down")
	}
	r.steps = append(r.steps, step)
	r.details = append(r.details, detail)
	return nil
}

type fakeNode struct {
	version, target string
	online          bool
	busy            []int
	recs            []session.CoordinatorRecord
	installErr      error
	swapOnInstall   bool
	helloVer        string // what the new agent says; "" = target
	helloErr        error
	probeErr        error
	restoreErr      error
	oldHelloErr     error
	versionErr      error
	noDowngrade     bool
	calls           []string
}

func newFake(from, to string) *fakeNode {
	return &fakeNode{version: from, target: to, online: true, swapOnInstall: true}
}

func (n *fakeNode) rec(s string)                 { n.calls = append(n.calls, s) }
func (n *fakeNode) ID() string                   { return "n1" }
func (n *fakeNode) Platform() lifecycle.Platform { return hostPlatform }
func (n *fakeNode) Version(context.Context) (string, error) {
	if n.versionErr != nil && n.version == n.target { // a swapped binary that cannot start
		return "", n.versionErr
	}
	return n.version, nil
}
func (n *fakeNode) Coordinators(context.Context) ([]session.CoordinatorRecord, error) {
	return n.recs, nil
}
func (n *fakeNode) Drain(context.Context) (func(context.Context) error, error) {
	n.rec("drain")
	if !n.online {
		return nil, nil
	}
	return func(context.Context) error { n.rec("enable"); return nil }, nil
}
func (n *fakeNode) Busy(context.Context) (int, error) {
	n.rec("busy")
	if len(n.busy) == 0 {
		return 0, nil
	}
	b := n.busy[0]
	if len(n.busy) > 1 {
		n.busy = n.busy[1:]
	}
	return b, nil
}
func (n *fakeNode) Install(context.Context, InstallInput) error {
	n.rec("install")
	if n.swapOnInstall {
		n.version = n.target
	}
	return n.installErr
}
func (n *fakeNode) Restart(context.Context) error { n.rec("restart"); return nil }
func (n *fakeNode) Hello(context.Context, time.Duration) (string, error) {
	n.rec("hello")
	if n.version != n.target { // the previous binary after a restore
		return n.version, n.oldHelloErr
	}
	if n.helloVer != "" {
		return n.helloVer, n.helloErr
	}
	return n.target, n.helloErr
}
func (n *fakeNode) Probe(context.Context) error { n.rec("probe"); return n.probeErr }
func (n *fakeNode) Restore(context.Context) error {
	n.rec("restore")
	if n.restoreErr == nil {
		n.version, n.probeErr = "1.4.0", nil // the previous binary is healthy
	}
	return n.restoreErr
}

type pullFake struct{ *fakeNode }

func (pullFake) CanDowngrade() bool { return false }

func manifest(v string, pmin, pmax int) verify.Manifest {
	return verify.Manifest{Version: v, Target: "linux-amd64", SHA256: strings.Repeat("a", 64), ProtocolMin: pmin, ProtocolMax: pmax}
}

func run(n *fakeNode, a *recAudit, rq Request, m verify.Manifest) (Result, error) {
	e := Engine{Source: fakeSource{m: m}, Audit: a}
	rq.Poll = time.Millisecond
	if rq.Version == "" {
		rq.Version = "1.5.0"
	}
	return e.Run(context.Background(), n, rq)
}

func TestEngineHappyPathOrder(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	res, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if err != nil || res.Noop || res.RolledBack || res.From != "1.4.0" || res.To != "1.5.0" {
		t.Fatalf("res %+v err %v", res, err)
	}
	if got, want := strings.Join(n.calls, ","), "drain,busy,install,restart,hello,probe,enable"; got != want {
		t.Fatalf("calls %s want %s", got, want)
	}
	if got, want := strings.Join(a.steps, ","), "verify,drain,wait,install,restart,hello,probe,enable"; got != want {
		t.Fatalf("audit %s want %s", got, want)
	}
}

func TestEngineNoopWhenAlreadyThere(t *testing.T) {
	n, a := newFake("1.5.0", "1.5.0"), &recAudit{}
	res, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if err != nil || !res.Noop || len(n.calls) != 0 {
		t.Fatalf("res %+v err %v calls %v", res, err, n.calls)
	}
}

func TestEngineRefusals(t *testing.T) {
	rec := func(min, max int, minAgent string) []session.CoordinatorRecord {
		return []session.CoordinatorRecord{{Coordinator: session.Coordinator{ID: "c1"}, ProtocolMin: min, ProtocolMax: max, MinAgentVersion: minAgent}}
	}
	cases := []struct {
		name, from, to string
		recs           []session.CoordinatorRecord
		m              verify.Manifest
		allow          bool
		reason         string
	}{
		{"ceiling: release needs a newer protocol", "1.4.0", "2.0.0", rec(1, 1, ""), manifest("2.0.0", 2, 2), false, ReasonCeiling},
		{"ceiling: release too old for the coordinator", "1.4.0", "1.5.0", rec(2, 2, ""), manifest("1.5.0", 1, 1), false, ReasonCeiling},
		{"ceiling: below min agent version", "1.4.0", "1.5.0", rec(1, 1, "1.6.0"), manifest("1.5.0", 1, 1), true, ReasonCeiling},
		{"downgrade without the flag", "1.5.0", "1.4.0", nil, manifest("1.4.0", 1, 1), false, ReasonDowngrade},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, a := newFake(c.from, c.to), &recAudit{}
			n.recs = c.recs
			_, err := run(n, a, Request{Version: c.to, AllowDowngrade: c.allow}, c.m)
			var e *Error
			if !errors.As(err, &e) || e.Reason != c.reason || !strings.HasPrefix(err.Error(), "E666:") {
				t.Fatalf("err %v, want E666 %s", err, c.reason)
			}
			if len(n.calls) != 0 {
				t.Fatalf("a refused upgrade touched the node: %v", n.calls)
			}
			if a.steps[len(a.steps)-1] != StepRefused {
				t.Fatalf("no refusal row: %v", a.steps)
			}
		})
	}
}

func TestEngineDowngradeWithFlagIsAudited(t *testing.T) {
	n, a := newFake("1.5.0", "1.4.0"), &recAudit{}
	n.version = "1.5.0"
	res, err := run(n, a, Request{Version: "1.4.0", AllowDowngrade: true}, manifest("1.4.0", 1, 1))
	if err != nil || res.Direction != Downgrade {
		t.Fatalf("res %+v err %v", res, err)
	}
	if !strings.Contains(strings.Join(a.steps, ","), StepDowngrade) {
		t.Fatalf("no downgrade row: %v", a.steps)
	}
}

func TestEngineWaitsForRunningJobsThenInstalls(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	n.busy = []int{2, 1, 0}
	if _, err := run(n, a, Request{DrainWait: time.Second}, manifest("1.5.0", 1, 1)); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(n.calls[:5], ","), "drain,busy,busy,busy,install"; got != want {
		t.Fatalf("calls %s want %s", got, want)
	}
}

func TestEngineRunningJobNeverInterrupted(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	n.busy = []int{1}
	_, err := run(n, a, Request{DrainWait: 20 * time.Millisecond}, manifest("1.5.0", 1, 1))
	if ReasonOf(err) != ReasonRunningJob {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(strings.Join(n.calls, ","), "install") || n.calls[len(n.calls)-1] != "enable" {
		t.Fatalf("calls %v", n.calls)
	}
}

func TestEngineNodeNotOnlineIsNotEnabled(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	n.online = false
	if _, err := run(n, a, Request{}, manifest("1.5.0", 1, 1)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(n.calls, ","), "enable") {
		t.Fatalf("enabled a node the admin had drained: %v", n.calls)
	}
}

func TestEngineInstallFailureBeforeSwapReenables(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	n.swapOnInstall, n.installErr = false, errors.New("E656: copy failed")
	res, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if err == nil || res.RolledBack || strings.Contains(strings.Join(n.calls, ","), "restore") || n.calls[len(n.calls)-1] != "enable" {
		t.Fatalf("err %v rolledBack %v calls %v", err, res.RolledBack, n.calls)
	}
}

func TestEngineInstallErrorAfterSwapRollsBack(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	n.installErr = errors.New("E656: connection dropped after the swap")
	res, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if err == nil || !res.RolledBack || n.version != "1.4.0" {
		t.Fatalf("err %v res %+v version %s", err, res, n.version)
	}
}

func TestEngineHealthFailuresRestoreThePreviousBinary(t *testing.T) {
	cases := map[string]func(*fakeNode){
		"hello error":   func(n *fakeNode) { n.helloErr = errors.New("no hello within 60s") },
		"hello wrong":   func(n *fakeNode) { n.helloVer = "1.4.9" },
		"probe failure": func(n *fakeNode) { n.probeErr = errors.New("probe failed") },
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
			set(n)
			res, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
			if err == nil || !res.RolledBack || n.version != "1.4.0" || !strings.HasPrefix(err.Error(), "E656:") {
				t.Fatalf("err %v res %+v version %s", err, res, n.version)
			}
			if n.calls[len(n.calls)-1] != "enable" || !strings.Contains(strings.Join(a.steps, ","), StepRollback) {
				t.Fatalf("calls %v audit %v", n.calls, a.steps)
			}
		})
	}
}

func TestEngineFailedRollbackLeavesNodeDrained(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	n.probeErr, n.restoreErr = errors.New("probe failed"), errors.New("mv failed")
	_, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if err == nil || !strings.Contains(err.Error(), "stays drained") || strings.Contains(strings.Join(n.calls, ","), "enable") {
		t.Fatalf("err %v calls %v", err, n.calls)
	}
}

func TestEngineStopsWhenAnAuditRowCannotBeWritten(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{failOn: StepDrain}
	_, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if err == nil || strings.Contains(strings.Join(n.calls, ","), "install") {
		t.Fatalf("err %v calls %v", err, n.calls)
	}
}

func TestEngineUnverifiedReleaseTouchesNothing(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	e := Engine{Source: fakeSource{err: fmt.Errorf("E665: bad_signature")}, Audit: a}
	if _, err := e.Run(context.Background(), n, Request{Version: "1.5.0"}); err == nil || len(n.calls) != 0 || strings.Join(a.steps, ",") != StepFailed {
		t.Fatalf("err %v calls %v audit %v", err, n.calls, a.steps)
	}
}

func TestEngineBrokenBinaryThatCannotAnswerVersionIsRolledBack(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	n.installErr, n.versionErr = errors.New("E656: the new agent was not installed"), errors.New("segfault")
	res, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if err == nil || !res.RolledBack || n.version != "1.4.0" || n.calls[len(n.calls)-1] != "enable" {
		t.Fatalf("err %v res %+v version %s calls %v", err, res, n.version, n.calls)
	}
}

func TestEngineRefusedInstallBecauseAJobStartedChangesNothing(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	n.swapOnInstall, n.installErr = false, refuse(ReasonRunningJob, "a job started during the copy")
	res, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if ReasonOf(err) != ReasonRunningJob || res.RolledBack || strings.Contains(strings.Join(n.calls, ","), "restore") || n.calls[len(n.calls)-1] != "enable" {
		t.Fatalf("err %v res %+v calls %v", err, res, n.calls)
	}
}

func TestEngineRollbackDoesNotWaitForItsAuditRow(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{failOn: StepRollback}
	n.probeErr = errors.New("probe failed")
	res, err := run(n, a, Request{}, manifest("1.5.0", 1, 1))
	if err == nil || !res.RolledBack || n.version != "1.4.0" {
		t.Fatalf("a failing audit store left the broken binary: err %v version %s", err, n.version)
	}
}

func TestEnginePullNodeDowngradeIsRefusedBeforeTheDrain(t *testing.T) {
	n, a := newFake("1.5.0", "1.4.0"), &recAudit{}
	e := Engine{Source: fakeSource{m: manifest("1.4.0", 1, 1)}, Audit: a}
	_, err := e.Run(context.Background(), pullFake{n}, Request{Version: "1.4.0", AllowDowngrade: true, Poll: time.Millisecond})
	if ReasonOf(err) != ReasonDowngrade || len(n.calls) != 0 {
		t.Fatalf("err %v calls %v", err, n.calls)
	}
}

func TestEngineFailureRowsHoldTheCodeOnly(t *testing.T) {
	n, a := newFake("1.4.0", "1.5.0"), &recAudit{}
	e := Engine{Source: fakeSource{err: errors.New("E665: remote command on admin@secret-host failed")}, Audit: a}
	if _, err := e.Run(context.Background(), n, Request{Version: "1.5.0"}); err == nil || strings.Join(a.steps, ",") != StepFailed || strings.Contains(strings.Join(a.details, ";"), "secret-host") {
		t.Fatalf("err %v audit %v %v", err, a.steps, a.details)
	}
}
