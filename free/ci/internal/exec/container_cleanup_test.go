//go:build !windows

package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helperPlanner runs its cleanup in its own image, as the container profile does (D-0963).
type helperPlanner struct {
	hardPlanner
	declared bool // the command carries Image; false hides it so the backstop cannot find the helper image
}

func (p helperPlanner) Cleanup(r ContainerRun) (*ContainerCommand, error) {
	if !r.JobImageCleanup {
		r.Image = "docker.io/library/busybox@sha256:helper"
	}
	cmd, err := p.hardPlanner.Cleanup(r)
	if err == nil && p.declared {
		cmd.Image = r.Image
	}
	return cmd, err
}

func TestPlannedCleanupRunsItsOwnImage(t *testing.T) {
	for name, tc := range map[string]struct {
		declared  bool
		wantNote  bool
		wantClean bool
	}{
		"helper image declared":     {declared: true, wantClean: true},
		"helper image not declared": {declared: false, wantNote: true},
	} {
		t.Run(name, func(t *testing.T) {
			calls := fakeRuntime(t, "ok\n", "0")
			var out strings.Builder
			spec := plannedSpec(t, "plan-helper", helperPlanner{declared: tc.declared})
			spec.Output = func(b []byte) { out.Write(b) }
			res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), spec)
			if res.Err != nil || res.Attempt.State != "passed" {
				t.Fatalf("result %+v", res)
			}
			got := calls()
			cleaned := len(got) == 3 && got[1] == "rm -f nself-ci-plan-helper" && strings.Contains(got[2], "docker.io/library/busybox@sha256:helper -c true") && !strings.Contains(got[2], "alpine:3.19")
			if cleaned != tc.wantClean {
				t.Errorf("cleanup ran in the helper image = %v, want %v (calls %q)", cleaned, tc.wantClean, got)
			}
			if note := strings.Contains(out.String(), "cleanup helper did not finish"); note != tc.wantNote {
				t.Errorf("failure note = %v, want %v (output %q)", note, tc.wantNote, out.String())
			}
		})
	}
}

// A node that cannot start the helper image (never pulled, no registry) still cleans the workspace in the job image,
// as it did before the helper existed; a node where both fail reports it in the job log.
func TestPlannedCleanupFallsBackToTheJobImage(t *testing.T) {
	for name, tc := range map[string]struct {
		helperFails, jobFails bool
		wantCalls             int
		wantNote              bool
	}{
		"helper fine":                   {wantCalls: 3},
		"helper missing then job image": {helperFails: true, wantCalls: 4},
		"both fail":                     {helperFails: true, jobFails: true, wantCalls: 4, wantNote: true},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "calls")
			body := "echo \"$*\" >> " + log + "\ncase \"$*\" in\n"
			if tc.helperFails {
				body += "*sha256:helper*) echo 'Unable to find image' >&2; exit 125;;\n"
			}
			if tc.jobFails {
				body += "*--entrypoint*) exit 1;;\n"
			}
			body += "esac\nexit 0\n"
			fakeDocker(t, body)
			var out strings.Builder
			spec := plannedSpec(t, "plan-fallback", helperPlanner{declared: true})
			spec.Output = func(b []byte) { out.Write(b) }
			res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), spec)
			if res.Err != nil || res.Attempt.State != "passed" {
				t.Fatalf("result %+v", res)
			}
			data, _ := os.ReadFile(log)
			calls := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(calls) != tc.wantCalls {
				t.Fatalf("calls %q, want %d", calls, tc.wantCalls)
			}
			if tc.helperFails && (!strings.Contains(calls[2], "sha256:helper") || !strings.Contains(calls[3], "alpine:3.19") || strings.Contains(calls[3], "sha256:helper")) {
				t.Errorf("second try must run the job image: %q", calls[1:])
			}
			if note := strings.Contains(out.String(), "cleanup helper did not finish"); note != tc.wantNote {
				t.Errorf("note = %v, want %v (output %q)", note, tc.wantNote, out.String())
			}
		})
	}
}
