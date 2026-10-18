package run

import (
	"context"
	"errors"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type wrapProfile struct {
	level model.Isolation
	av    isolation.Availability
	err   error
	calls []string
}

func (w *wrapProfile) Level() model.Isolation                            { return w.level }
func (w *wrapProfile) Probe(context.Context) isolation.Availability      { return w.av }
func (w *wrapProfile) Teardown(context.Context, isolation.Attempt) error { return nil }
func (w *wrapProfile) Prepare(context.Context, isolation.Attempt) error {
	w.calls = append(w.calls, "prepare")
	return w.err
}
func (w *wrapProfile) Wrap(s ciexec.JobSpec) (ciexec.JobSpec, error) {
	w.calls = append(w.calls, "wrap")
	s.Slot = 7
	return s, nil
}

func TestIsolatorSelectsAndWraps(t *testing.T) {
	reg := &isolation.Registry{}
	p := &wrapProfile{level: "container", av: isolation.Availability{Level: "container"}}
	reg.Register(p)
	iso := selectIsolator{reg: reg}
	spec := ciexec.JobSpec{AttemptID: "a", CoordinatorID: "c", Job: model.Job{Isolation: "container"}}
	if err := iso.Prepare(context.Background(), &spec); err != nil || spec.Slot != 7 || len(p.calls) != 2 {
		t.Fatalf("%v %+v %v", err, spec, p.calls)
	}
	for _, level := range []model.Isolation{"", "process"} {
		plain := ciexec.JobSpec{Job: model.Job{Isolation: level}}
		if err := iso.Prepare(context.Background(), &plain); err != nil || plain.Slot != 0 {
			t.Fatalf("%q: %v", level, err)
		}
	}
}

func TestIsolatorRefusesUnavailableLevelsWithE678(t *testing.T) {
	reg := &isolation.Registry{}
	reg.Register(&wrapProfile{level: "container", av: isolation.Availability{Reason: isolation.ReasonRuntimeMissing}})
	iso := selectIsolator{reg: reg}
	for _, level := range []model.Isolation{"container", "sandboxed-container", "vm"} {
		spec := ciexec.JobSpec{Job: model.Job{Isolation: level}}
		var e *isolation.Error
		if err := iso.Prepare(context.Background(), &spec); !errors.As(err, &e) || e.Code != "E678" {
			t.Fatalf("%q: %v", level, err)
		}
	}
}

func TestIsolatorPrepareFailureStopsWrap(t *testing.T) {
	reg := &isolation.Registry{}
	p := &wrapProfile{level: "container", av: isolation.Availability{Level: "container"}, err: errors.New("gone")}
	reg.Register(p)
	spec := ciexec.JobSpec{Job: model.Job{Isolation: "container"}}
	if err := (selectIsolator{reg: reg}).Prepare(context.Background(), &spec); err == nil || len(p.calls) != 1 {
		t.Fatalf("%v %v", err, p.calls)
	}
}

func TestRegisteredIsolatorIsTheRegistryOne(t *testing.T) {
	if Registered.Isolation == nil {
		t.Fatal("isolation hook not registered")
	}
	resolved := Hooks{}.Resolved()
	if resolved.Isolation == nil {
		t.Fatal("Resolved dropped the isolation hook")
	}
}
