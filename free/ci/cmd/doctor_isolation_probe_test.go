package main

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/tart"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type probeStub struct {
	level model.Isolation
	av    isolation.Availability
}

func (p probeStub) Level() model.Isolation                            { return p.level }
func (p probeStub) Probe(context.Context) isolation.Availability      { return p.av }
func (p probeStub) Prepare(context.Context, isolation.Attempt) error  { return nil }
func (p probeStub) Wrap(s exec.JobSpec) (exec.JobSpec, error)         { return s, nil }
func (p probeStub) Teardown(context.Context, isolation.Attempt) error { return nil }

// availabilityChecks runs the availability section over the given profiles.
func availabilityChecks(t *testing.T, profiles ...isolation.Profile) map[string]doctor.Check {
	t.Helper()
	old := isolationProfiles
	isolationProfiles = func() []isolation.Profile { return profiles }
	t.Cleanup(func() { isolationProfiles = old })
	out := map[string]doctor.Check{}
	for _, c := range isolationAvailability(context.Background(), doctor.Env{Getenv: func(string) string { return "" }}) {
		out[c.ID] = c
	}
	return out
}

func TestDoctorIsolationAvailabilitySectionIsRegistered(t *testing.T) {
	for _, s := range doctor.Registered() {
		if s.Name == "isolation-availability" && s.Selector == doctor.SelectAlways {
			return
		}
	}
	t.Fatal("the isolation-availability section is not in the doctor registry")
}

// D-1354, D-1063: an unavailable level shows its reason, its install hint and the Tart licence note.
func TestDoctorIsolationAvailabilityShowsReasonHintAndLicence(t *testing.T) {
	vm := tart.New("vm", tart.Options{GOOS: "linux", GOARCH: "amd64"})
	gv := probeStub{"sandboxed-container", isolation.Availability{
		Reason: isolation.ReasonRuntimeMissing, Facts: map[string]string{"hint": "install gVisor and run runsc install"}}}
	up := probeStub{"container", isolation.Availability{Level: "container", Facts: map[string]string{"engine": "docker"}}}
	got := availabilityChecks(t, up, gv, vm)

	c, ok := got["isolation.vm.availability"]
	if !ok {
		t.Fatalf("no vm availability check in %v", got)
	}
	if c.Status != doctor.Pass {
		t.Errorf("an unavailable level is the honest state of some hosts, not a fault: status %v", c.Status)
	}
	for _, want := range []string{isolation.ReasonRuntimeMissing, "unavailable"} {
		if !strings.Contains(c.Message, want) {
			t.Errorf("vm message %q lacks %q", c.Message, want)
		}
	}
	if !strings.Contains(c.Remediation, "Apple-silicon") || !strings.Contains(c.Detail, "Apple-silicon") {
		t.Errorf("vm hint missing: remediation %q detail %q", c.Remediation, c.Detail)
	}
	if !strings.Contains(c.Detail, tart.LicenceNote) {
		t.Errorf("vm detail lacks the Tart licence note: %q", c.Detail)
	}
	if strings.Contains(c.Detail, "hint=") {
		t.Errorf("the hint is printed once as the fix line, not as a fact: %q", c.Detail)
	}

	g := got["isolation.sandboxed-container.availability"]
	if !strings.Contains(g.Detail, "install gVisor") || g.Remediation != "install gVisor and run runsc install" {
		t.Errorf("gvisor install hint missing: %+v", g)
	}

	u := got["isolation.container.availability"]
	if u.Status != doctor.Pass || !strings.Contains(u.Message, "available") || u.Remediation != "" || !strings.Contains(u.Detail, "engine=docker") {
		t.Errorf("available container check wrong: %+v", u)
	}
}
