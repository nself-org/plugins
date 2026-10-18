package container

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// fakeRunner answers `<binary> info` and `ps` from canned output.
type fakeRunner struct {
	info  map[string]string
	ps    string
	calls int
	down  string // binary whose daemon is installed but not answering
}

func (f *fakeRunner) Query(_ context.Context, binary string, args ...string) ([]byte, error) {
	f.calls++
	if args[0] == "ps" {
		return []byte(f.ps), nil
	}
	if out, ok := f.info[binary]; ok {
		return []byte(out), nil
	}
	if binary == f.down {
		return nil, errors.New("Cannot connect to the Docker daemon")
	}
	return nil, fs.ErrNotExist
}

const (
	dockerRootful   = `{"ServerVersion":"27.0.1","MemoryLimit":true,"PidsLimit":true,"CPUCfsQuota":true,"SecurityOptions":["name=seccomp,profile=builtin"],"Runtimes":{"runc":{},"runsc":{}}}`
	dockerRootless  = `{"ServerVersion":"27.0.1","MemoryLimit":true,"PidsLimit":true,"CPUCfsQuota":true,"SecurityOptions":["name=seccomp,profile=builtin","name=rootless"],"Runtimes":{"runc":{}}}`
	dockerUserns    = `{"ServerVersion":"27.0.1","MemoryLimit":true,"PidsLimit":true,"CPUCfsQuota":true,"SecurityOptions":["name=userns"],"Runtimes":{"runc":{}}}`
	podmanRootless  = `{"host":{"security":{"rootless":true},"ociRuntime":{"name":"crun"},"cgroupVersion":"v2","cgroupControllers":["cpu","memory","pids"]},"version":{"Version":"5.2.0"}}`
	dockerNoLimits  = `{"ServerVersion":"27.0.1","SecurityOptions":["name=rootless"],"MemoryLimit":true,"PidsLimit":false,"CPUCfsQuota":true,"Runtimes":{"runc":{}}}`
	podmanNoCgroups = `{"host":{"security":{"rootless":true},"ociRuntime":{"name":"crun"},"cgroupVersion":"v2","cgroupControllers":["memory"]},"version":{"Version":"5.2.0"}}`
	podmanRootfulFx = `{"host":{"security":{"rootless":false},"ociRuntime":{"name":"crun"},"cgroupVersion":"v2","cgroupControllers":["cpu","memory","pids"]},"version":{"Version":"5.2.0"}}`
)

func TestProbeTable(t *testing.T) {
	for name, tc := range map[string]struct {
		info      map[string]string
		separated bool
		level     model.Isolation
		reason    string
		runtime   string
	}{
		"no runtime":                                     {map[string]string{}, false, "", isolation.ReasonRuntimeMissing, ""},
		"rootful docker, own machine":                    {map[string]string{"docker": dockerRootful}, false, "container", "", "docker"},
		"rootful docker, separated node":                 {map[string]string{"docker": dockerRootful}, true, "", isolation.ReasonRootful, ""},
		"rootless docker, separated node":                {map[string]string{"docker": dockerRootless}, true, "container", "", "docker"},
		"userns docker, separated node":                  {map[string]string{"docker": dockerUserns}, true, "", isolation.ReasonRootful, ""},
		"rootless podman, separated node":                {map[string]string{"podman": podmanRootless}, true, "container", "", "podman"},
		"rootful podman, separated node":                 {map[string]string{"podman": podmanRootfulFx}, true, "", isolation.ReasonRootful, ""},
		"rootful docker then rootless podman, separated": {map[string]string{"docker": dockerRootful, "podman": podmanRootless}, true, "container", "", "podman"},
		"docker without a pids controller":               {map[string]string{"docker": dockerNoLimits}, false, "", isolation.ReasonLimits, ""},
		"podman without delegated cgroup controllers":    {map[string]string{"podman": podmanNoCgroups}, true, "", isolation.ReasonLimits, ""},
		"unparseable":                                    {map[string]string{"docker": "not json"}, false, "", isolation.ReasonProbeFailed, ""},
	} {
		t.Run(name, func(t *testing.T) {
			facts, av := probeRuntime(context.Background(), &fakeRunner{info: tc.info}, tc.separated)
			if av.Level != tc.level || av.Reason != tc.reason || facts.Binary != tc.runtime {
				t.Fatalf("level %q reason %q runtime %q", av.Level, av.Reason, facts.Binary)
			}
		})
	}
}

func TestProbeDistinguishesDaemonDownFromMissing(t *testing.T) {
	_, av := probeRuntime(context.Background(), &fakeRunner{down: "docker"}, false)
	if av.Reason != isolation.ReasonProbeFailed {
		t.Fatalf("daemon down reported as %q", av.Reason)
	}
	_, av = probeRuntime(context.Background(), &fakeRunner{down: "docker", info: map[string]string{"podman": podmanRootfulFx}}, true)
	if av.Reason != isolation.ReasonRootful {
		t.Fatalf("rootful outranks probe failure: %q", av.Reason)
	}
}

func TestListAndTeardownReportUnavailableRuntime(t *testing.T) {
	p := newProfile(&fakeRunner{down: "docker"}, false)
	var e *isolation.Error
	if _, err := p.List(context.Background(), "role=ci-ephemeral"); !errors.As(err, &e) || e.Reason != isolation.ReasonProbeFailed {
		t.Fatalf("List hid an unreachable runtime: %v", err)
	}
	if err := p.Teardown(context.Background(), isolation.Attempt{ID: "x"}); !errors.As(err, &e) {
		t.Fatalf("Teardown hid an unreachable runtime: %v", err)
	}
}

func TestWrapRefusesUnmappedSlotOnRootless(t *testing.T) {
	p := newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootless}}, true)
	if _, err := p.Wrap(exec.JobSpec{Slot: 3}); err != nil {
		t.Fatalf("slot 3: %v", err)
	}
	var e *isolation.Error
	if _, err := p.Wrap(exec.JobSpec{Slot: 4}); !errors.As(err, &e) || e.Reason != isolation.ReasonSlotUID {
		t.Fatalf("slot 4: %v", err)
	}
	if _, err := p.Wrap(exec.JobSpec{Slot: -1}); err == nil {
		t.Fatal("negative slot accepted")
	}
}

func TestProbeFacts(t *testing.T) {
	facts, av := probeRuntime(context.Background(), &fakeRunner{info: map[string]string{"docker": dockerRootful}}, false)
	if av.Facts["runtimes"] != "runc,runsc" || av.Facts["rootless"] != "false" || av.Facts["server_version"] != "27.0.1" || facts.Binary != "docker" {
		t.Fatalf("%v", av.Facts)
	}
}

func newProfile(r *fakeRunner, separated bool) *Profile {
	return New(Options{Runner: r, Separated: func() bool { return separated }, UIDs: func() []int { return []int{501} }})
}

func TestWrapAttachesPlannerAndBuildsHardenedArgv(t *testing.T) {
	p := newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootful}}, false)
	spec, err := p.Wrap(exec.JobSpec{Network: "none", Slot: 1})
	if err != nil || spec.Plan == nil {
		t.Fatalf("%v %v", spec.Plan, err)
	}
	run := exec.ContainerRun{Name: "nself-ci-x", Image: "alpine:3.19", Workspace: "/w/x", Workdir: "/repo", Command: []string{"id"}, AttemptID: "x", CoordinatorID: "c", Slot: 1, CPU: 1, MemoryMB: 64}
	cmd, err := spec.Plan.Plan(run)
	if err != nil || !has(cmd.Args, "--user", "65533:65533") || !has(cmd.Args, "--network=none") {
		t.Fatalf("%v %v", cmd.Args, err)
	}
	clean, err := spec.Plan.Cleanup(run)
	if err != nil || clean == nil || !has(clean.Args, "--entrypoint", "sh") || !has(clean.Args, "--network=none") || has(clean.Args, "--env-file") || !has(clean.Args, "--name", "nself-ci-x-clean") {
		t.Fatalf("%v %v", clean, err)
	}
}

func TestWrapWithoutRuntimeIsE678NotAFallback(t *testing.T) {
	p := newProfile(&fakeRunner{}, false)
	spec, err := p.Wrap(exec.JobSpec{})
	var e *isolation.Error
	if !errors.As(err, &e) || e.Code != "E678" || e.Reason != isolation.ReasonRuntimeMissing || spec.Plan != nil {
		t.Fatalf("%v %v", err, spec.Plan)
	}
	if err := p.Prepare(context.Background(), isolation.Attempt{ID: "x"}); err == nil {
		t.Fatal("Prepare passed without a runtime")
	}
}

func TestWrapRefusesRootfulOnSeparatedNode(t *testing.T) {
	p := newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootful}}, true)
	_, err := p.Wrap(exec.JobSpec{})
	var e *isolation.Error
	if !errors.As(err, &e) || e.Reason != isolation.ReasonRootful {
		t.Fatalf("%v", err)
	}
}

func TestWrapNetworkScopeE679(t *testing.T) {
	p := newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootful}}, false)
	_, err := p.Wrap(exec.JobSpec{Network: "internet"})
	var e *isolation.Error
	if !errors.As(err, &e) || e.Code != "E679" {
		t.Fatalf("%v", err)
	}
}

func TestProbeIsCachedThenRefreshed(t *testing.T) {
	r := &fakeRunner{info: map[string]string{"docker": dockerRootful}}
	now := time.Unix(1000, 0)
	p := New(Options{Runner: r, TTL: time.Minute, Now: func() time.Time { return now }})
	p.Probe(context.Background())
	p.Probe(context.Background())
	if r.calls != 1 {
		t.Fatalf("%d queries inside the TTL", r.calls)
	}
	now = now.Add(2 * time.Minute)
	p.Probe(context.Background())
	if r.calls != 2 {
		t.Fatalf("%d queries after the TTL", r.calls)
	}
}

func TestListParsesNames(t *testing.T) {
	r := &fakeRunner{info: map[string]string{"docker": dockerRootful}, ps: "nself-ci-a\n\n nself-ci-b \n"}
	names, err := newProfile(r, false).List(context.Background(), "role=ci-ephemeral")
	if err != nil || strings.Join(names, ",") != "nself-ci-a,nself-ci-b" {
		t.Fatalf("%v %v", names, err)
	}
}

func TestProfileLevelAndRegistry(t *testing.T) {
	reg := &isolation.Registry{}
	reg.Register(newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootful}}, false))
	p, av, err := reg.Select(context.Background(), "container")
	if err != nil || p.Level() != "container" || av.Level != "container" {
		t.Fatalf("%v %v %v", p, av, err)
	}
	if _, _, err := reg.Select(context.Background(), "sandboxed-container"); err == nil {
		t.Fatal("container profile served a sandboxed request")
	}
	names, err := reg.List(context.Background(), "role=ci-ephemeral")
	if err != nil || len(names) != 0 {
		t.Fatalf("%v %v", names, err)
	}
}

// D-0963: cleanup must not run in the job image (it may have no sh, or a hostile sh).
func TestCleanupRunsInThePinnedHelperImageNotTheJobImage(t *testing.T) {
	p := newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootful}}, false)
	spec, err := p.Wrap(exec.JobSpec{Network: "none"})
	if err != nil {
		t.Fatal(err)
	}
	run := exec.ContainerRun{Name: "nself-ci-x", Image: "gcr.io/distroless/static:nonroot", Workspace: "/w/x", Workdir: "/repo", Command: []string{"id"}, AttemptID: "x", CoordinatorID: "c", CPU: 1, MemoryMB: 64}
	clean, err := spec.Plan.Cleanup(run)
	if err != nil || clean == nil {
		t.Fatalf("%v %v", clean, err)
	}
	var image string
	for i, a := range clean.Args {
		if a == "-c" && i > 0 { // the image is the argument before the sh script flag
			image = clean.Args[i-1]
		}
	}
	if image != CleanupImage {
		t.Fatalf("cleanup image %q, want the pinned helper %q (job image %q must never run cleanup)", image, CleanupImage, run.Image)
	}
}

func TestCleanupHelperImageIsDigestPinnedAndMatchesImagesD(t *testing.T) {
	if !regexp.MustCompile(`^[a-z0-9][^@\s]*:[^@:/\s]+@sha256:[0-9a-f]{64}$`).MatchString(CleanupImage) {
		t.Fatalf("CleanupImage %q is not tag@sha256 pinned", CleanupImage)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "images.d", "ci-cleanup-helper.json"))
	if err != nil {
		t.Fatalf("the helper image needs an images.d fragment: %v", err)
	}
	var frag struct{ Image string }
	if err := json.Unmarshal(data, &frag); err != nil || frag.Image != CleanupImage {
		t.Fatalf("images.d fragment image %q != CleanupImage %q (%v)", frag.Image, CleanupImage, err)
	}
}
