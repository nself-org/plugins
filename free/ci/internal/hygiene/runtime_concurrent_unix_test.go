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
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// serialReferenceRuntime preserves the pre-concurrency reducer as an independent oracle.
// Inputs: a context, kind and attempt. Outputs: resources or the first binary error.
// Constraints: use a separate ExecRuntime so the reference cannot prime the subject's cache.
type serialReferenceRuntime struct{ *ExecRuntime }

func (r serialReferenceRuntime) List(ctx context.Context, kind exec.ResourceKind, attempt string) ([]exec.Labelled, error) {
	var out []exec.Labelled
	var down []string
	present := 0
	for _, bin := range r.binaries() {
		found, state, err := r.listBinary(ctx, bin, kind, attempt)
		if err != nil {
			return nil, err
		}
		if state == runtimeDown {
			down = append(down, bin)
			continue
		}
		if state == runtimeAbsent {
			continue
		}
		present++
		for _, item := range found {
			item.ID = bin + ":" + item.ID
			out = append(out, item)
		}
	}
	if len(down) > 0 {
		return out, &RuntimeDownError{Binaries: down}
	}
	if present == 0 {
		return nil, ErrNoRuntime
	}
	return out, nil
}

// concurrentFakeRuntimes installs isolated fake CLIs, recording starts before any delay.
// Inputs: per-binary modes and an optional absolute sleep path. Outputs: invocation log.
// Constraints: no host runtime or external PATH command may be invoked.
func concurrentFakeRuntimes(t *testing.T, docker, podman, sleep string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	for i, mode := range []string{docker, podman} {
		bin := []string{"docker", "podman"}[i]
		if mode == "missing" {
			continue
		}
		script := "#!/bin/sh\nprintf '%s %s\\n' '" + bin + "' \"$1\" >> '" + log + "'\n"
		script += "if [ \"$1\" = info ]; then\n"
		if mode == "down" || mode == "network-down" {
			script += "exit 1\n"
		} else {
			script += "exit 0\n"
		}
		script += "fi\n"
		if sleep != "" {
			script += "'" + sleep + "' 0.2\n"
		}
		switch mode {
		case "down", "error":
			script += "exit 1\n"
		case "malformed":
			script += "printf 'unsafe/resource a1\\n'\nexit 0\n"
		case "network-down":
			script += "if [ \"$1\" = network ]; then exit 1; fi\n"
		}
		script += "case \"$1\" in\nps) printf 'c2 a2 exited\\nc1 a1 running\\n';;\n"
		script += "network) printf 'n2 a2\\nn1 a1\\n';;\nvolume) printf 'v2 a2\\nv1 a1\\n';;\nesac\n"
		if err := os.WriteFile(filepath.Join(dir, bin), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return log
}

func assertConcurrentError(t *testing.T, got, want error) {
	t.Helper()
	if fmt.Sprintf("%T:%v", got, got) != fmt.Sprintf("%T:%v", want, want) {
		t.Fatalf("error = %T:%v; serial = %T:%v", got, got, want, want)
	}
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded, ErrNoRuntime} {
		if errors.Is(got, sentinel) != errors.Is(want, sentinel) {
			t.Fatalf("errors.Is(%v) differs: got %v; serial %v", sentinel, got, want)
		}
	}
	var gotDown, wantDown *RuntimeDownError
	if errors.As(got, &gotDown) != errors.As(want, &wantDown) || !reflect.DeepEqual(gotDown, wantDown) {
		t.Fatalf("RuntimeDownError = %#v; serial = %#v", gotDown, wantDown)
	}
}

func TestConcurrentBatchMatchesSerialRuntime(t *testing.T) {
	cases := []struct {
		name, docker, podman string
		cancelled            bool
	}{
		{"both up", "up", "up", false},
		{"docker down", "down", "up", false},
		{"podman down", "up", "down", false},
		{"both down", "down", "down", false},
		{"docker missing", "missing", "up", false},
		{"podman missing", "up", "missing", false},
		{"both missing", "missing", "missing", false},
		{"cancelled", "up", "up", true},
		{"first binary error", "malformed", "error", false},
		{"error beats down", "down", "malformed", false},
		{"down after earlier success", "network-down", "up", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			concurrentFakeRuntimes(t, tc.docker, tc.podman, "")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			gotInner, wantInner := &ExecRuntime{}, &ExecRuntime{}
			got := ConcurrentBatchRuntime(gotInner)
			want := BatchRuntime(wantInner)
			reference := BatchRuntime(serialReferenceRuntime{&ExecRuntime{}})
			for _, kind := range Kinds {
				for _, attempt := range []string{"", "a1", "a2", "absent"} {
					gotItems, gotErr := got.List(ctx, kind, attempt)
					wantItems, wantErr := want.List(ctx, kind, attempt)
					refItems, refErr := reference.List(ctx, kind, attempt)
					if !reflect.DeepEqual(wantItems, refItems) {
						t.Fatalf("ordinary batch resources differ from serial reference: %#v; %#v", wantItems, refItems)
					}
					assertConcurrentError(t, wantErr, refErr)
					if !reflect.DeepEqual(gotItems, wantItems) {
						t.Fatalf("%s/%q resources = %#v; serial = %#v", kind, attempt, gotItems, wantItems)
					}
					assertConcurrentError(t, gotErr, wantErr)
				}
			}
			for _, bin := range gotInner.binaries() {
				gotState, gotHit := gotInner.cachedProbe(bin)
				wantState, wantHit := wantInner.cachedProbe(bin)
				if gotState != wantState || gotHit != wantHit {
					t.Fatalf("%s cache = (%v, %v); serial = (%v, %v)", bin, gotState, gotHit, wantState, wantHit)
				}
			}
		})
	}
}

func TestConcurrentBatchListingsOverlap(t *testing.T) {
	sleep, err := osexec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	sleep, err = filepath.Abs(sleep)
	if err != nil {
		t.Fatal(err)
	}
	log := concurrentFakeRuntimes(t, "up", "up", sleep)
	runtime := ConcurrentBatchRuntime(&ExecRuntime{})
	start := time.Now()
	for _, kind := range Kinds {
		if _, err := runtime.List(context.Background(), kind, ""); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed >= 400*time.Millisecond {
		t.Fatalf("six 200 ms listings took %s; want under 400 ms", elapsed)
	}
	calls := readCalls(t, log)
	for _, bin := range []string{"docker", "podman"} {
		for _, command := range []string{"ps", "network", "volume"} {
			if count := strings.Count(calls, bin+" "+command+"\n"); count != 1 {
				t.Fatalf("%s %s ran %d times; calls: %q", bin, command, count, calls)
			}
		}
	}
	for _, kind := range Kinds {
		if _, err := runtime.List(context.Background(), kind, "a1"); err != nil {
			t.Fatal(err)
		}
	}
	if got := readCalls(t, log); got != calls {
		t.Fatalf("cached snapshot ran more commands: before %q, after %q", calls, got)
	}
}
