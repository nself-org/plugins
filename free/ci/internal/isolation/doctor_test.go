package isolation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

func TestDoctorSectionReportsEveryLevel(t *testing.T) {
	r := &Registry{}
	r.Register(fakeProfile{level: "container", av: Availability{Level: "container"}})
	r.Register(fakeProfile{level: "sandboxed-container", av: Availability{
		Reason: ReasonRuntimeMissing,
		Facts:  map[string]string{"hint": "install gVisor, run `sudo runsc install`"},
	}})
	checks := doctorChecks(context.Background(), r)
	if len(checks) != 2 {
		t.Fatalf("want one check per level, got %+v", checks)
	}
	ok, bad := checks[0], checks[1]
	if ok.Subject != "container" || ok.Status != doctor.Pass || ok.ID != "CI-DT001" {
		t.Errorf("available level: %+v", ok)
	}
	if bad.Subject != "sandboxed-container" || bad.Status != doctor.Warn || bad.ID != "CI-DT001" {
		t.Errorf("unavailable level: %+v", bad)
	}
	if !strings.Contains(bad.Message, ReasonRuntimeMissing) {
		t.Errorf("message must carry the reason: %q", bad.Message)
	}
	if !strings.Contains(bad.Remediation, "sudo runsc install") {
		t.Errorf("remediation must carry the hint fact: %q", bad.Remediation)
	}
}

func TestDoctorSectionUnavailableWithoutHint(t *testing.T) {
	r := &Registry{}
	r.Register(fakeProfile{level: "container", av: Availability{Reason: ReasonRuntimeMissing}})
	c := doctorChecks(context.Background(), r)[0]
	if c.Status != doctor.Warn || c.Remediation == "" {
		t.Errorf("an unavailable level always gets a remediation: %+v", c)
	}
}

func TestDoctorSectionNoProfiles(t *testing.T) {
	c := doctorChecks(context.Background(), &Registry{})
	if len(c) != 1 || c[0].Status != doctor.Pass || c[0].ID != "CI-DT001" {
		t.Errorf("no profile registered is one pass: %+v", c)
	}
}

// The section is registered for every doctor run, under its own name.
func TestDoctorSectionRegistered(t *testing.T) {
	for _, s := range doctor.Registered() {
		if s.Name == SectionName {
			if s.Selector != doctor.SelectAlways {
				t.Errorf("selector %q", s.Selector)
			}
			return
		}
	}
	t.Fatalf("section %q not registered", SectionName)
}

// A probe cut short by the section's budget must not read as an unavailable level.
func TestDoctorSectionCancelledProbeIsNotAVerdict(t *testing.T) {
	r := &Registry{}
	r.Register(fakeProfile{level: "container", av: Availability{Reason: ReasonProbeFailed}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	c := doctorChecks(ctx, r)[0]
	if c.Status != doctor.Warn || !strings.Contains(c.Message, "did not finish") {
		t.Errorf("cancelled probe: %+v", c)
	}
}
