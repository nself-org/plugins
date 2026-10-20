//go:build integration

package local

// Linux evidence for P7-SCHED-09 (Constitution 10.3, Epic D20): real jobs run through the executor in
// sandboxes the provider reserved, over the real container and gVisor profiles. EPHEMERAL_LOCAL_EXPECT
// (`sandboxed-container` with runsc installed, `container` without) pins the level the host must offer;
// ci-ephemeral-local.yml runs both legs. A missing prerequisite is t.Fatal, never t.Skip.

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
	"github.com/nself-org/plugins/free/ci/internal/isolation/gvisor"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type linuxHost struct {
	provider *Provider
	cp, gp   isolation.Profile
	image    string
}

// profileFor wraps a job at the level the reservation was granted at.
type profileFor struct {
	h *linuxHost
	p *Provider
}

func (i profileFor) Prepare(_ context.Context, s *exec.JobSpec) error {
	prof := i.h.cp
	if i.p.Level(s.AttemptID) == sandboxLevel {
		prof = i.h.gp
	}
	w, err := prof.Wrap(*s)
	*s = w
	return err
}

func linuxPrereqs(t *testing.T) *linuxHost {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Fatalf("prerequisite missing: the Linux leg needs Linux, got %s", runtime.GOOS)
	}
	h := &linuxHost{cp: container.New(container.Options{}), gp: gvisor.New(gvisor.Options{}), image: os.Getenv("TRUST04_TEST_IMAGE")}
	if h.image == "" {
		h.image = "alpine:3.19"
	}
	if av := h.cp.Probe(context.Background()); !av.Available() {
		t.Fatalf("prerequisite missing: a container runtime is required: %+v", av)
	}
	if _, err := exec.RuntimeQuery(context.Background(), "docker", "inspect", h.image); err != nil {
		t.Fatalf("prerequisite missing: image %s must be pulled first: %v", h.image, err)
	}
	p, err := New(Options{Backend: NewContainerBackend(h.cp, h.gp, nil), MaxSlots: 4, Host: bigHost})
	if err != nil {
		t.Fatal(err)
	}
	h.provider = p
	return h
}

func ecoord(t *testing.T) string { return "ELCOORD" + fmt.Sprint(time.Now().UnixNano()%1_000_000_000) }

// labelledContainers lists role=ci-ephemeral containers; with a coordinator it narrows to that coordinator's, so
// packages that run in parallel on one daemon (the isolation suites) cannot make the check flaky. The workflow
// then asserts the whole daemon is empty after the run.
func labelledContainers(t *testing.T, coordinator string) []string {
	t.Helper()
	args := []string{"ps", "-a", "--filter", "label=role=ci-ephemeral", "--format", "{{.Names}}"}
	if coordinator != "" {
		args = append(args, "--filter", "label=coordinator_id="+coordinator)
	}
	out, err := exec.RuntimeQuery(context.Background(), "docker", args...)
	if err != nil {
		t.Fatalf("docker ps: %v", err)
	}
	return strings.Fields(string(out))
}

func intentLinux(coord, token string) burst.Intent {
	return burst.Intent{Token: token, Spec: burst.Spec{Platform: "linux/" + runtime.GOARCH, CPU: 1, MemMB: 128, TimeoutMs: 120000}, Labels: burst.IntentLabels(coord, token)}
}

func (h *linuxHost) job(res burst.Resource, home, script string, out *strings.Builder, mu *sync.Mutex) exec.AttemptResult {
	level := h.provider.Level(res.ID)
	e := &exec.Executor{Isolation: profileFor{h, h.provider}, Grace: 2 * time.Second}
	return e.Run(context.Background(), exec.JobSpec{AttemptID: res.ID, CoordinatorID: res.Labels[coordKey], Home: home,
		Output: func(b []byte) { mu.Lock(); out.Write(b); mu.Unlock() }, Command: []string{"sh", "-c", script},
		Job: model.Job{Isolation: level, Container: &model.ContainerConfig{Image: h.image}}, CPU: 1, MemoryMB: 128, Timeout: 90 * time.Second})
}

func TestLinuxOneSandboxPerJob(t *testing.T) {
	h := linuxPrereqs(t)
	ctx := context.Background()
	co := ecoord(t)
	var ress []burst.Resource
	for i := 0; i < 4; i++ {
		r, err := h.provider.Provision(ctx, intentLinux(co, fmt.Sprintf("ELJOB%dX%d", i, time.Now().UnixNano()%100000)))
		if err != nil {
			t.Fatalf("provision %d: %v", i, err)
		}
		ress = append(ress, r)
	}
	t.Cleanup(func() {
		for _, r := range ress {
			_ = h.provider.Teardown(ctx, r.ID)
		}
	})
	if want := os.Getenv("EPHEMERAL_LOCAL_EXPECT"); want != "" && string(h.provider.Level(ress[0].ID)) != want {
		t.Fatalf("the host offers level %q, this leg expects %q", h.provider.Level(ress[0].ID), want)
	}
	if lvl := h.provider.Level(ress[0].ID); lvl != sandboxLevel && lvl != containerLevel {
		t.Fatalf("level %q", lvl)
	}
	if _, err := h.provider.Provision(ctx, intentLinux(co, "ELOVERFLOW")); err == nil {
		t.Fatal("a fifth slot was granted over MaxSlots 4")
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	outs := make([]strings.Builder, len(ress))
	results := make([]exec.AttemptResult, len(ress))
	took := make([]time.Duration, len(ress))
	for i := range ress {
		home := t.TempDir()
		wg.Add(1)
		go func() {
			defer wg.Done()
			// a fresh sandbox has an empty /tmp and its own hostname; the whole run of a trivial job is its cold-start figure
			start := time.Now()
			results[i] = h.job(ress[i], home, `test -e /tmp/marker && echo REUSED || echo FRESH; touch /tmp/marker; hostname`, &outs[i], &mu)
			took[i] = time.Since(start)
		}()
	}
	wg.Wait()
	hosts := map[string]bool{}
	for i, r := range results {
		out := outs[i].String()
		if r.Err != nil || r.ExitCode != 0 || !strings.Contains(out, "FRESH") || strings.Contains(out, "REUSED") {
			t.Fatalf("job %d: %+v\n%s", i, r, out)
		}
		hosts[strings.Fields(out)[len(strings.Fields(out))-1]] = true
		h.provider.Observe(ress[i].ID, took[i])
	}
	if len(hosts) != 4 {
		t.Fatalf("4 jobs shared sandboxes: hostnames %v", hosts)
	}
	if _, n := h.provider.lat.get(h.provider.Level(ress[0].ID)); n != 4 {
		t.Fatalf("latency samples %d, want 4", n)
	}
	for _, r := range ress {
		if err := h.provider.Teardown(ctx, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	if left := labelledContainers(t, co); len(left) != 0 {
		t.Fatalf("docker ps -a --filter label=role=ci-ephemeral is not empty for %s: %v", co, left)
	}
	if got, err := h.provider.List(ctx, burst.OwnLabels(co)); err != nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

// A coordinator that died with a job running leaves a labelled container; a new provider lists it with its
// age and intent, never lists an ordinary job container, adopts it on a retry and removes it on Teardown.
func TestLinuxLeakIsFoundAdoptedAndRemoved(t *testing.T) {
	h := linuxPrereqs(t)
	ctx := context.Background()
	co := ecoord(t)
	token := fmt.Sprintf("ELLEAK%d", time.Now().UnixNano()%1_000_000)
	id := idPrefix + token
	run := func(name, attempt string) {
		out, err := osexec.Command("docker", "run", "-d", "--name", name, "--label", "role=ci-ephemeral", "--label", "coordinator_id="+co,
			"--label", "nself.ci.attempt="+attempt, h.image, "sleep", "300").CombinedOutput()
		if err != nil {
			t.Fatalf("docker run %s: %v %s", name, err, out)
		}
	}
	plain := "nself-ci-plain-" + token
	run("nself-ci-"+id, id)
	run(plain, "plain-"+token)
	t.Cleanup(func() { _ = osexec.Command("docker", "rm", "-f", "nself-ci-"+id, plain).Run() })

	restarted, err := New(Options{Backend: NewContainerBackend(h.cp, h.gp, nil), MaxSlots: 4, Host: bigHost})
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.List(ctx, burst.OwnLabels(co))
	if err != nil || len(got) != 1 {
		t.Fatalf("want only the burst container, got %+v %v", got, err)
	}
	if got[0].ID != id || got[0].Labels[intentKey] != token || got[0].CreatedAt.IsZero() || time.Since(got[0].CreatedAt) > 5*time.Minute {
		t.Fatalf("%+v", got[0])
	}
	r, err := restarted.Provision(ctx, intentLinux(co, token))
	if err != nil || r.ID != id || !r.CreatedAt.Equal(got[0].CreatedAt) {
		t.Fatalf("a retry must adopt the running sandbox: %+v %v", r, err)
	}
	if err := restarted.Teardown(ctx, id); err != nil {
		t.Fatal(err)
	}
	left := labelledContainers(t, co)
	sort.Strings(left)
	for _, n := range left {
		if n == "nself-ci-"+id {
			t.Fatalf("the leak is still there: %v", left)
		}
	}
	if err := restarted.Teardown(ctx, id); err != nil {
		t.Fatalf("a second teardown must succeed: %v", err)
	}
	if err := restarted.Teardown(ctx, "plain-"+token); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range labelledContainers(t, co) {
		found = found || n == plain
	}
	if !found {
		t.Fatal("Teardown removed an ordinary job container that is not in the burst namespace")
	}
}
