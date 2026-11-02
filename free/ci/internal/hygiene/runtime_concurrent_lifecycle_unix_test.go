//go:build unix

package hygiene

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

func TestConcurrentBatchMatchesWarmSerialCaches(t *testing.T) {
	for _, docker := range []runtimeState{runtimeUp, runtimeDown, runtimeAbsent} {
		for _, podman := range []runtimeState{runtimeUp, runtimeDown, runtimeAbsent} {
			t.Run(fmt.Sprintf("docker-%d-podman-%d", docker, podman), func(t *testing.T) {
				log := concurrentFakeRuntimes(t, "up", "up", "")
				gotInner, wantInner := &ExecRuntime{}, &ExecRuntime{}
				for i, bin := range []string{"docker", "podman"} {
					state := []runtimeState{docker, podman}[i]
					gotInner.recordProbe(bin, state)
					wantInner.recordProbe(bin, state)
				}
				got := ConcurrentBatchRuntime(gotInner)
				want := BatchRuntime(serialReferenceRuntime{wantInner})
				for _, kind := range Kinds {
					items, err := got.List(context.Background(), kind, "")
					wantItems, wantErr := want.List(context.Background(), kind, "")
					if !reflect.DeepEqual(items, wantItems) {
						t.Fatalf("%s resources = %#v; serial = %#v", kind, items, wantItems)
					}
					assertConcurrentError(t, err, wantErr)
				}
				calls := readCalls(t, log)
				for i, bin := range []string{"docker", "podman"} {
					state := []runtimeState{docker, podman}[i]
					if cached, hit := gotInner.cachedProbe(bin); !hit || cached != state {
						t.Fatalf("%s warm cache = (%v, %v); want (%v, true)", bin, cached, hit, state)
					}
					if state != runtimeUp && strings.Contains(calls, bin+" ") {
						t.Fatalf("cached unavailable %s was invoked: %q", bin, calls)
					}
				}
				if strings.Contains(calls, " info\n") {
					t.Fatalf("warm cache caused an info probe: %q", calls)
				}
			})
		}
	}
}

func TestConcurrentBatchRequeriesExpiredSkippedProbe(t *testing.T) {
	for _, state := range []runtimeState{runtimeDown, runtimeAbsent} {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			log := concurrentFakeRuntimes(t, "up", "missing", "")
			runtime := &ExecRuntime{Binaries: []string{"docker"}, usable: map[string]probeResult{
				"docker": {state: state, at: time.Now().Add(-probeTTL - time.Second)},
			}}
			// Empty slots model workers skipped while a negative probe was still fresh.
			results := make([][]listingResult, len(Kinds))
			for i := range results {
				results[i] = []listingResult{{}}
			}
			snapshots := runtime.reduceKinds(context.Background(), runtime.binaries(), results)
			for _, kind := range Kinds {
				snapshot := snapshots[kind]
				if snapshot.err != nil || len(snapshot.items) != 2 || !strings.HasPrefix(snapshot.items[0].ID, "docker:") {
					t.Fatalf("%s expired skipped slot = %#v, %v", kind, snapshot.items, snapshot.err)
				}
			}
			if state, hit := runtime.cachedProbe("docker"); !hit || state != runtimeUp {
				t.Fatalf("expired negative cache was not refreshed: (%v, %v)", state, hit)
			}
			if calls := strings.Fields(readCalls(t, log)); len(calls) != 6 {
				t.Fatalf("expected three real listings after cache expiry: %v", calls)
			}
		})
	}
}

// lifecycleRuntimes starts one failing listing and five delayed listings.
// Inputs: whether delays should wait for cancellation. Outputs: start and completion logs.
// Constraints: cancellation uses exec sleep so there are no surviving shell descendants.
func lifecycleRuntimes(t *testing.T, untilCancel bool) (string, string) {
	t.Helper()
	sleep, err := osexec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	sleep, err = filepath.Abs(sleep)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	starts, done := filepath.Join(dir, "starts.log"), filepath.Join(dir, "done.log")
	for _, bin := range []string{"docker", "podman"} {
		script := "#!/bin/sh\nif [ \"$1\" = info ]; then exit 0; fi\n"
		script += "printf '%s %s %s\\n' \"$$\" '" + bin + "' \"$1\" >> '" + starts + "'\n"
		script += "if [ '" + bin + "' = docker ] && [ \"$1\" = ps ]; then printf 'bad/name a1 running\\n'; exit 0; fi\n"
		if untilCancel {
			script += "exec '" + sleep + "' 10\n"
		} else {
			script += "'" + sleep + "' 0.2\nprintf '%s %s\\n' '" + bin + "' \"$1\" >> '" + done + "'\n"
		}
		if err := os.WriteFile(filepath.Join(dir, bin), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return starts, done
}

func TestConcurrentBatchJoinsListingsBeforeReturningError(t *testing.T) {
	starts, done := lifecycleRuntimes(t, false)
	_, err := ConcurrentBatchRuntime(&ExecRuntime{}).List(context.Background(), exec.KindContainer, "")
	if err == nil || !strings.Contains(err.Error(), "unsafe name or label") {
		t.Fatalf("first listing error = %v; want malformed docker resource", err)
	}
	if calls := strings.Fields(readCalls(t, starts)); len(calls) != 18 {
		t.Fatalf("expected six listing starts; got %v", calls)
	}
	if calls := strings.Fields(readCalls(t, done)); len(calls) != 10 {
		t.Fatalf("returned before all five delayed listings completed: %v", calls)
	}
}

func TestConcurrentBatchJoinsCancelledListings(t *testing.T) {
	starts, _ := lifecycleRuntimes(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := ConcurrentBatchRuntime(&ExecRuntime{}).List(ctx, exec.KindContainer, "")
		result <- err
	}()
	// Always cancel and join the caller, including when waiting for starts fails.
	defer func() {
		cancel()
		select {
		case <-result:
		case <-time.After(2 * time.Second):
			t.Error("listing caller did not exit after cancellation")
		}
	}()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	var calls []string
	for len(calls) < 18 {
		select {
		case <-deadline.C:
			t.Fatalf("expected six listing starts; got %v", calls)
		case <-tick.C:
			calls = strings.Fields(readCalls(t, starts))
		}
	}
	cancel()
	select {
	case err := <-result:
		// Leave a token for the deferred join after successfully observing completion.
		result <- err
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled batch error = %v; want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not promptly join listings")
	}
	for i := 0; i < len(calls); i += 3 {
		pid, err := strconv.Atoi(calls[i])
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("listing process %d remains after cancellation: %v", pid, err)
		}
	}
}
