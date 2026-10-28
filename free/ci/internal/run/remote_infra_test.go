package run

// Purpose: D-1103 tests: a result a node reports as an infra failure, or a cancel the operator did
// not ask for, ends the attempt lost (the CI-40 retry path) and never finalizing, which has no way
// back to the queue.
// Inputs:  a private store, a leased remote attempt, the remote sink.
// Outputs: assertions on the attempt state and the run's rendezvous channels.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func evWith(result string, checks ...model.Check) json.RawMessage {
	raw, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: testRev, Attempt: &model.AttemptRef{ID: "run1.jobA.a1", N: 1}, Result: model.Result(result), Checks: checks})
	return raw
}

// lostCause is the cause the run was told, failing at once when it was told nothing.
func lostCause(t *testing.T, ra *remoteAttempt) error {
	t.Helper()
	select {
	case cause := <-ra.lost:
		return cause
	default:
		t.Fatal("the run was not told the carrier is lost")
		return nil
	}
}

func TestRemoteInfraResultTakesTheLostPath(t *testing.T) {
	cases := map[string]struct {
		raw       json.RawMessage
		cancelled bool // the run asked the carrier to stop
		lost      bool
	}{
		"error result":             {raw: evWith("error"), lost: true},
		"fail with an infra check": {raw: evWith("fail", model.Check{ID: "c", JobID: "j", Result: "fail", FailureClass: "infra"}), lost: true},
		"error with a code check":  {raw: evWith("error", model.Check{ID: "c", JobID: "j", Result: "error", FailureClass: "code"}), lost: false},
		"fail with a code check":   {raw: evWith("fail", model.Check{ID: "c", JobID: "j", Result: "fail", FailureClass: "code"}), lost: false},
		"unreadable evidence":      {raw: json.RawMessage(`"x"`), lost: true},
		"cancel nobody asked for":  {raw: evWith("cancelled"), lost: true},
		"cancel the run asked for": {raw: evWith("cancelled"), cancelled: true, lost: false},
		"pass":                     {raw: evWith("pass"), lost: false},
	}
	for name, c := range cases {
		e := newRemoteEnv(t)
		ra := e.sink.expect(e.id, testRev)
		if c.cancelled {
			e.sink.cancelCarrier(e.id)
		}
		if err := e.sink.Result(context.Background(), protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: c.raw}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := "finalizing"
		if c.lost {
			want = "lost"
		}
		if got := e.state(t); got != want {
			t.Errorf("%s: state %s, want %s", name, got, want)
		}
		select {
		case cause := <-ra.lost:
			if !c.lost {
				t.Errorf("%s: reported lost (%v) but the result is the run's to settle", name, cause)
			}
		default:
			if c.lost {
				t.Errorf("%s: the run was not told the carrier is lost", name)
			}
		}
		if c.lost {
			select {
			case ev := <-ra.result:
				t.Errorf("%s: a lost attempt also delivered a result %q", name, ev.Result)
			default:
			}
		}
	}
}

func TestRemoteInfraResultIsFencedAndSingle(t *testing.T) {
	e := newRemoteEnv(t)
	e.sink.expect(e.id, testRev)
	ctx := context.Background()
	if err := e.sink.Result(ctx, protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch) + 1, Evidence: evWith("error")}); err == nil {
		t.Fatal("an infra result for another epoch was accepted")
	}
	if got := e.state(t); got == "lost" {
		t.Fatalf("a stale infra result moved the attempt to %s", got)
	}
	if err := e.sink.Result(ctx, protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: evWith("error")}); err != nil {
		t.Fatal(err)
	}
	if err := e.sink.Result(ctx, protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: evWith("error")}); err == nil {
		t.Fatal("a second infra result for the same lease was accepted")
	}
}

// A node-reported infra failure counts against the node; a cancel it chose itself does not.
func TestRemoteInfraResultCountsAgainstTheNode(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id, testRev)
	if err := e.sink.Result(context.Background(), protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: evWith("error")}); err != nil {
		t.Fatal(err)
	}
	if cause := lostCause(t, ra); !nodeFault(cause) {
		t.Fatalf("cause %v is not a node fault", cause)
	}
	e2 := newRemoteEnv(t)
	ra2 := e2.sink.expect(e2.id, testRev)
	if err := e2.sink.Result(context.Background(), protocol.Result{LeaseID: e2.lease, Epoch: uint64(e2.epoch), Evidence: evWith("cancelled")}); err != nil {
		t.Fatal(err)
	}
	if cause := lostCause(t, ra2); nodeFault(cause) {
		t.Fatalf("a node-initiated cancel %v must not trip the node's breaker", cause)
	}
}

// A frame that carries a signature the node registry does not verify is an infra failure of the node,
// even when its document says pass; an unsigned frame keeps the old acceptance (P7-TRUST-20).
func TestRemoteResultSignatureMustVerifyWhenPresent(t *testing.T) {
	for name, c := range map[string]struct {
		sig  protocol.Signature
		want string
	}{
		"unsigned":                 {want: "finalizing"},
		"signed by an unknown key": {sig: protocol.Signature{KeyID: "ci-node-0123456789abcdef", Value: "AAAA"}, want: "lost"},
		"key id without a value":   {sig: protocol.Signature{KeyID: "ci-node-0123456789abcdef"}, want: "lost"},
	} {
		e := newRemoteEnv(t)
		ra := e.sink.expect(e.id, testRev)
		if err := e.sink.Result(context.Background(), protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: evWith("pass"), Signature: c.sig}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := e.state(t); got != c.want {
			t.Errorf("%s: state %s, want %s", name, got, c.want)
		}
		if c.want == "lost" {
			if cause := lostCause(t, ra); !nodeFault(cause) {
				t.Errorf("%s: cause %v must count against the node", name, cause)
			}
		}
	}
}
