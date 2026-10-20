package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func offsetHook(got chan<- []protocol.Offset, ready chan<- struct{}, release <-chan struct{}) JobHook {
	return JobHook{Name: "offsets",
		After: func(ctx context.Context, l protocol.Lease, root JobRoot, out Outcome, up Uploader) error {
			src, ok := up.(OffsetSource)
			if !ok {
				close(got)
				return nil
			}
			if err := up.Digest(protocol.Digest{Stream: "artifact", SHA256: "abc", Size: 1}); err != nil {
				return err
			}
			close(ready)
			<-release
			var seen []protocol.Offset
			for {
				select {
				case o := <-src.Offsets():
					seen = append(seen, o)
				default:
					got <- seen
					return nil
				}
			}
		}}
}

func passRunner() Runner {
	return RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		return JobResult{Outcome: Outcome{State: "passed"}, Evidence: json.RawMessage(`{}`)}, nil
	})
}

// TestOffsetFramesReachTheHookOfTheirLeaseOnly proves the uploader is an OffsetSource and that an
// offset reaches only the hook of the lease and epoch it is fenced to.
func TestOffsetFramesReachTheHookOfTheirLeaseOnly(t *testing.T) {
	got, ready, release := make(chan []protocol.Offset, 1), make(chan struct{}), make(chan struct{})
	r := startRig(t, passRunner(), func(c *Config) { c.Hooks = []JobHook{offsetHook(got, ready, release)} })
	r.handshake()
	r.lease(testLease("l1", 3))
	<-ready
	r.car.pushFenced("offset", protocol.Offset{Stream: "artifact", SHA256: "abc", Offset: 99}, 2, "l1") // stale epoch
	r.car.pushFenced("offset", protocol.Offset{Stream: "artifact", SHA256: "abc", Offset: 98}, 3, "other")
	r.car.pushFenced("offset", protocol.Offset{Stream: "artifact", SHA256: "abc", Offset: 7}, 3, "l1")
	r.car.push("heartbeat", protocol.Heartbeat{Leases: []protocol.LeaseState{}}) // a later frame the loop handled proves the offsets were too
	time.Sleep(100 * time.Millisecond)
	close(release)
	seen := <-got
	if len(seen) != 1 || seen[0].Offset != 7 {
		t.Fatalf("hook saw %+v, want only the offset 7 of lease l1 epoch 3", seen)
	}
}

// TestOffsetFloodNeverBlocksTheSessionAndKeepsTheNewest proves a full buffer drops the oldest.
func TestOffsetFloodNeverBlocksTheSessionAndKeepsTheNewest(t *testing.T) {
	got, ready, release := make(chan []protocol.Offset, 1), make(chan struct{}), make(chan struct{})
	r := startRig(t, passRunner(), func(c *Config) { c.Hooks = []JobHook{offsetHook(got, ready, release)} })
	r.handshake()
	r.lease(testLease("l1", 1))
	<-ready
	for i := 1; i <= 3*offsetBuffer; i++ {
		r.car.pushFenced("offset", protocol.Offset{Stream: "artifact", SHA256: "abc", Offset: int64(i)}, 1, "l1")
	}
	time.Sleep(200 * time.Millisecond)
	close(release)
	seen := <-got
	if len(seen) != offsetBuffer || seen[len(seen)-1].Offset != int64(3*offsetBuffer) {
		t.Fatalf("hook saw %d answers ending at %v, want %d ending at %d", len(seen), seen[len(seen)-1], offsetBuffer, 3*offsetBuffer)
	}
	r.car.next(t, "digest")
	r.car.next(t, "result")
}

func hookFailing(err error) JobHook {
	return JobHook{Name: "artifacts", After: func(context.Context, protocol.Lease, JobRoot, Outcome, Uploader) error { return err }}
}

// resultEvidence runs one lease through a hook that fails and returns the evidence of the result.
func resultEvidence(t *testing.T, state string, hookErr error) string {
	t.Helper()
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		return JobResult{Outcome: Outcome{State: state}, Evidence: json.RawMessage(`{"schema":"ci.evidence/v1","result":"pass"}`)}, nil
	})
	r := startRig(t, runner, func(c *Config) { c.Hooks = []JobHook{hookFailing(hookErr)} })
	r.handshake()
	r.lease(testLease("l1", 1))
	return string(r.car.next(t, "result").Body.(protocol.Result).Evidence)
}

// TestFatalHookErrorTurnsAPassIntoAnError proves a hook that owes the coordinator something the pass
// depends on (the job's artifacts) cannot let the job pass when it fails, while an ordinary hook
// error stays a log line and a failed job is left as it is.
func TestFatalHookErrorTurnsAPassIntoAnError(t *testing.T) {
	fatal := fmt.Errorf("%w: only 1 of 2 artifacts arrived", ErrHookFatal)
	if ev := resultEvidence(t, "passed", fatal); !strings.Contains(ev, "agent_hook_failed") || !strings.Contains(ev, `"result":"error"`) {
		t.Fatalf("a fatal hook error left the evidence %s", ev)
	}
	if ev := resultEvidence(t, "passed", errors.New("cosmetic")); strings.Contains(ev, "agent_hook_failed") {
		t.Fatalf("an ordinary hook error changed the evidence: %s", ev)
	}
	if ev := resultEvidence(t, "failed", fatal); strings.Contains(ev, "agent_hook_failed") {
		t.Fatalf("a failed job's evidence was replaced: %s", ev)
	}
}
