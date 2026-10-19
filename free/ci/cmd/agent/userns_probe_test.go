package main

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

// TestUsernsProbePrintsTheProbeAsOneJSONLine is the D-1084 regression: the node lifecycle runs
// `nself-ci-agent userns-probe` and parses the last line as hygiene.Probe. Without the subcommand
// every user-mode node was stored uid_separation false.
func TestUsernsProbePrintsTheProbeAsOneJSONLine(t *testing.T) {
	old := usernsProbeFor
	t.Cleanup(func() { usernsProbeFor = old })
	usernsProbeFor = func(context.Context) (hygiene.Probe, error) {
		return hygiene.Probe{Available: true, Reason: hygiene.ProbeOK}, nil
	}
	code, out, errs := runCmd(t, "userns-probe")
	if code != 0 || errs != "" {
		t.Fatalf("code %d stderr %q", code, errs)
	}
	if out != "{\"available\":true,\"reason\":\"userns.ok\"}\n" {
		t.Fatalf("stdout %q", out)
	}

	usernsProbeFor = func(context.Context) (hygiene.Probe, error) {
		return hygiene.Probe{Reason: hygiene.ProbeNoSubuid}, nil
	}
	_, out, _ = runCmd(t, "userns-probe")
	var p hygiene.Probe
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &p); err != nil || p.Available || p.Reason != hygiene.ProbeNoSubuid {
		t.Fatalf("unavailable probe: %q %+v %v", out, p, err)
	}
}

func TestUsernsProbeUsageAndFailure(t *testing.T) {
	if code, _, _ := runCmd(t, "userns-probe", "extra"); code != 2 {
		t.Fatalf("extra arg: code %d, want 2", code)
	}
	old := usernsProbeFor
	t.Cleanup(func() { usernsProbeFor = old })
	usernsProbeFor = func(context.Context) (hygiene.Probe, error) { return hygiene.Probe{}, errors.New("no scratch") }
	code, out, errs := runCmd(t, "userns-probe")
	if code != 1 || out != "" || !strings.Contains(errs, "no scratch") {
		t.Fatalf("code %d stdout %q stderr %q", code, out, errs)
	}
}

// TestUsernsProbeRealRunFailsClosed runs the real probe: it must print a parsable Probe whatever the
// host allows, and never claim separation off Linux.
func TestUsernsProbeRealRunFailsClosed(t *testing.T) {
	code, out, errs := runCmd(t, "userns-probe")
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, errs)
	}
	var p hygiene.Probe
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &p); err != nil || p.Reason == "" {
		t.Fatalf("stdout %q: %+v %v", out, p, err)
	}
	if runtime.GOOS != "linux" && (p.Available || p.Reason != hygiene.ProbeNotLinux) {
		t.Fatalf("non-linux host reported %+v", p)
	}
}
