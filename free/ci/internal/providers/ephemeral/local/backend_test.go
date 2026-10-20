package local

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func up(level model.Isolation) isolation.Availability {
	return isolation.Availability{Level: level, Facts: map[string]string{"runtime": "docker"}}
}

func down(reason string) isolation.Availability {
	return isolation.Availability{Reason: reason, Facts: map[string]string{"hint": "not here"}}
}

func inspectLine(name, created, labels string) string {
	return name + "|" + created + "|" + labels + "\n"
}

func TestContainerBackendLevels(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		container *fakeProfile
		sandbox   *fakeProfile
		level     model.Isolation
		reason    string
	}{
		{"gvisor where runsc starts", &fakeProfile{level: "container", av: up("container")}, &fakeProfile{level: "sandboxed-container", av: up("sandboxed-container")}, "sandboxed-container", ""},
		{"container where runsc is absent", &fakeProfile{level: "container", av: up("container")}, &fakeProfile{level: "sandboxed-container", av: down("isolation.runtime_missing")}, "container", ""},
		{"container with no gvisor profile at all", &fakeProfile{level: "container", av: up("container")}, nil, "container", ""},
		{"a profile that reports no level is E698, never an empty level", &fakeProfile{level: "", av: up("")}, nil, "", ReasonNoRuntime},
		{"no container runtime is E698 and never a weaker level", &fakeProfile{level: "container", av: down("isolation.runtime_missing")}, &fakeProfile{level: "sandboxed-container", av: up("sandboxed-container")}, "", ReasonNoRuntime},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sb isolation.Profile
			if c.sandbox != nil {
				sb = c.sandbox
			}
			off, err := NewContainerBackend(c.container, sb, nil).Probe(ctx)
			if c.reason != "" {
				if CodeOf(err) != "E698" || ReasonOf(err) != c.reason {
					t.Fatalf("want E698 %s, got %v", c.reason, err)
				}
				return
			}
			if err != nil || off.Level != c.level || off.Total != -1 {
				t.Fatalf("%+v %v", off, err)
			}
		})
	}
}

func TestContainerBackendWithoutAProfileIsE698(t *testing.T) {
	if _, err := NewContainerBackend(nil, nil, nil).Probe(context.Background()); ReasonOf(err) != ReasonNoAdapter {
		t.Fatal(err)
	}
}

func TestContainerLeftoversAreLimitedToTheBurstNamespace(t *testing.T) {
	prof := &fakeProfile{level: "container", av: up("container"), names: []string{"nself-ci-burst-A1", "nself-ci-run-77-job", "nself-ci-burst-A1-clean", "nself-ci-burst-B2"}}
	labels := func(attempt string) string {
		return `{"role":"ci-ephemeral","coordinator_id":"C1","nself.ci.attempt":"` + attempt + `"}`
	}
	var asked []string
	q := func(_ context.Context, bin string, args ...string) ([]byte, error) {
		asked = append([]string{bin}, args...)
		return []byte(inspectLine("/nself-ci-burst-A1", "2026-10-19T12:00:00.5Z", labels("burst-A1")) +
			inspectLine("/nself-ci-run-77-job", "2026-10-19T12:00:00Z", labels("run-77-job")) +
			inspectLine("/nself-ci-burst-A1-clean", "2026-10-19T12:00:01Z", labels("burst-A1")) +
			inspectLine("/nself-ci-burst-B2", "not a time", labels("burst-B2")) +
			inspectLine("/nself-ci-burst-P3", `"2026-10-19T08:00:00.123456789-04:00"`, labels("burst-P3")) +
			inspectLine("/nself-ci-burst-G4", "2026-10-19 12:00:00.5 +0000 UTC", labels("burst-G4")) +
			"garbage\n"), nil
	}
	got, err := NewContainerBackend(prof, nil, q).Leftovers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if asked[0] != "docker" || asked[1] != "inspect" {
		t.Fatalf("asked %v", asked)
	}
	if len(got) != 5 {
		t.Fatalf("an ordinary job container and garbage must be skipped, an unreadable time must not hide a leak: %+v", got)
	}
	byID := map[string]Found{}
	for _, f := range got {
		byID[f.ID] = f
	}
	if !byID["burst-B2"].CreatedAt.IsZero() || !byID["burst-P3"].CreatedAt.Equal(time.Date(2026, 10, 19, 12, 0, 0, 123456789, time.UTC)) || !byID["burst-G4"].CreatedAt.Equal(time.Date(2026, 10, 19, 12, 0, 0, 500000000, time.UTC)) {
		t.Fatalf("created times: %+v", byID)
	}
	main, helper := got[0], got[1]
	if main.ID != "burst-A1" || main.Labels[intentKey] != "A1" || main.Labels[coordKey] != "C1" || main.CreatedAt.IsZero() {
		t.Fatalf("%+v", main)
	}
	if helper.ID != "burst-A1-clean" || helper.Labels[intentKey] != "" {
		t.Fatalf("the cleanup helper must never stand for the intent: %+v", helper)
	}
}

func TestContainerLeftoversNeverReportAnUnreadableRuntimeAsEmpty(t *testing.T) {
	boom := errors.New("inspect failed")
	prof := &fakeProfile{level: "container", av: up("container"), names: []string{"nself-ci-burst-A1"}}
	if _, err := NewContainerBackend(prof, nil, func(context.Context, string, ...string) ([]byte, error) { return nil, boom }).Leftovers(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("inspect failure: %v", err)
	}
	// a container that vanished between ps and inspect: partial output is still an answer
	partial := func(context.Context, string, ...string) ([]byte, error) {
		return []byte(inspectLine("/nself-ci-burst-A1", "2026-10-19T12:00:00Z", `{"role":"ci-ephemeral","coordinator_id":"C1","nself.ci.attempt":"burst-A1"}`)), boom
	}
	if got, err := NewContainerBackend(prof, nil, partial).Leftovers(context.Background()); err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	prof.listErr = boom
	if _, err := NewContainerBackend(prof, nil, nil).Leftovers(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("list failure: %v", err)
	}
	prof.listErr, prof.names = nil, nil
	if got, err := NewContainerBackend(prof, nil, nil).Leftovers(context.Background()); err != nil || got != nil {
		t.Fatalf("no containers: %+v %v", got, err)
	}
}

func TestContainerTeardownGoesThroughTheProfile(t *testing.T) {
	prof := &fakeProfile{level: "container", av: up("container")}
	b := NewContainerBackend(prof, nil, nil)
	if err := b.Teardown(context.Background(), isolation.Attempt{ID: "burst-A1"}); err != nil || len(prof.tornDown) != 1 || prof.tornDown[0] != "burst-A1" {
		t.Fatalf("%v %v", err, prof.tornDown)
	}
}

func TestTartBackendReportsAppleSlots(t *testing.T) {
	ctx := context.Background()
	facts := func(foreign, own string) map[string]string {
		return map[string]string{"slots_total": "2", "foreign_running": foreign, "own_running": own}
	}
	vm := &fakeProfile{level: "vm", av: isolation.Availability{Level: "vm", Facts: facts("0", "1")}, names: []string{"nself-ci-burst-A1"}}
	off, err := NewTartBackend(vm).Probe(ctx)
	if err != nil || off.Level != "vm" || off.Total != 2 || len(off.Running) != 1 || off.Running[0] != "burst-A1" {
		t.Fatalf("%+v %v", off, err)
	}
	// a foreign VM leaves one slot for us
	vm.av = isolation.Availability{Level: "vm", Facts: facts("1", "0")}
	if off, _ = NewTartBackend(vm).Probe(ctx); off.Total != 1 {
		t.Fatalf("%+v", off)
	}
	// both slots busy is capacity zero, not unavailable
	vm.av = isolation.Availability{Reason: "isolation.tart_no_free_slot", Facts: facts("2", "0")}
	if off, err = NewTartBackend(vm).Probe(ctx); err != nil || off.Total != 0 {
		t.Fatalf("%+v %v", off, err)
	}
	// no Tart, or no base image digest, is E698 and never a lower level
	vm.av = down("isolation.runtime_missing")
	if _, err = NewTartBackend(vm).Probe(ctx); CodeOf(err) != "E698" || ReasonOf(err) != ReasonNoTart {
		t.Fatalf("%v", err)
	}
	if _, err = NewTartBackend(nil).Probe(ctx); ReasonOf(err) != ReasonNoAdapter {
		t.Fatalf("%v", err)
	}
	vm.av = isolation.Availability{Level: "vm", Facts: facts("0", "0")}
	vm.listErr = errors.New("tart list failed")
	if _, err = NewTartBackend(vm).Probe(ctx); err == nil {
		t.Fatal("an unreadable VM list must not read as no VMs")
	}
}

func TestTartLimitReachesTheProvider(t *testing.T) {
	vm := &fakeProfile{level: "vm", av: isolation.Availability{Level: "vm", Facts: map[string]string{"slots_total": "2", "foreign_running": "0", "own_running": "0"}}}
	b := NewTartBackend(vm).(*tartBackend)
	b.platform = "darwin/arm64"
	p := newProvider(t, b, nil)
	granted := 0
	for _, tok := range []string{"T1", "T2", "T3"} {
		in := intentFor(tok)
		in.Spec.Platform = "darwin/arm64"
		if _, err := p.Provision(context.Background(), in); err == nil {
			granted++
		} else if !errors.Is(err, burst.ErrQuota) {
			t.Fatal(err)
		}
	}
	if granted != 2 {
		t.Fatalf("Apple allows two concurrent VMs, provider granted %d", granted)
	}
}

func TestNewHostPicksTheProfilesForTheOS(t *testing.T) {
	reg := &isolation.Registry{}
	if _, err := newFor("linux", reg, Options{}); ReasonOf(err) != ReasonNoAdapter {
		t.Fatalf("linux with no container profile: %v", err)
	}
	if _, err := newFor("darwin", reg, Options{}); ReasonOf(err) != ReasonNoTart {
		t.Fatalf("darwin with no vm profile: %v", err)
	}
	if _, err := newFor("windows", reg, Options{}); ReasonOf(err) != ReasonUnsupport {
		t.Fatalf("windows: %v", err)
	}
	reg.Register(&fakeProfile{level: "container", av: up("container")})
	p, err := newFor("linux", reg, Options{Now: func() time.Time { return epoch }, Host: bigHost})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.opts.Backend.(*containerBackend); !ok {
		t.Fatalf("%T", p.opts.Backend)
	}
	reg.Register(&fakeProfile{level: "vm", av: up("vm")})
	if p, err = newFor("darwin", reg, Options{}); err != nil {
		t.Fatal(err)
	} else if _, ok := p.opts.Backend.(*tartBackend); !ok {
		t.Fatalf("%T", p.opts.Backend)
	}
}

// The provider reaches containers and VMs only through the adapters and the exec funnel: no file of the
// package may import os/exec (the architecture test checks the layers; this names the rule for this package).
func TestNoDirectOSExec(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, im := range af.Imports {
			if im.Path.Value == `"os/exec"` {
				t.Errorf("%s imports os/exec: use the isolation adapters and internal/exec", f)
			}
		}
	}
	if checked < 6 {
		t.Fatalf("only %d source files read", checked)
	}
	if _, err := os.Stat("provider.go"); err != nil {
		t.Fatal(err)
	}
}
