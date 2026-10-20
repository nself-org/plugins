//go:build integration_tart

package local

// Apple-silicon evidence for P7-SCHED-09 (Constitution 10.3, Epic D20): real Tart clones, one per job, never
// more than Apple's two at once, none left after a run or after a coordinator is killed and the next one
// sweeps. Run on an Apple-silicon Mac with Tart installed and the image and key P7-TRUST-06 documents:
//
//	NSELF_TEST_TART_IMAGE=<ref>@sha256:<digest> NSELF_TEST_TART_SSH_KEY=<identity file the image trusts> \
//	go test -mod=vendor -count=1 -v -tags integration_tart -timeout 45m ./internal/providers/ephemeral/local/...
//
// Every missing prerequisite is t.Fatal, never t.Skip.

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/tart"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func tartNeed(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Fatalf("prerequisite missing: set %s", name)
	}
	return v
}

func tartProvider(t *testing.T, home string) (*Provider, *tart.Profile) {
	t.Helper()
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatalf("prerequisite missing: an Apple-silicon Mac, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if _, err := osexec.LookPath("tart"); err != nil {
		t.Fatal("prerequisite missing: tart is not installed")
	}
	image, key := tartNeed(t, "NSELF_TEST_TART_IMAGE"), tartNeed(t, "NSELF_TEST_TART_SSH_KEY")
	prof := tart.New("vm", tart.Options{BaseImage: func() string { return image }, SSHKey: func() string { return key }, Home: func() string { return home }})
	p, err := New(Options{Backend: NewTartBackend(prof), Host: bigHost})
	if err != nil {
		t.Fatal(err)
	}
	return p, prof
}

func tartClones(t *testing.T) []string {
	t.Helper()
	left, err := tartClonesErr()
	if err != nil {
		t.Fatal(err)
	}
	return left
}

// tartClonesErr is tartClones for goroutines, which must not call t.Fatal.
func tartClonesErr() ([]string, error) {
	out, err := osexec.Command("tart", "list", "--quiet").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tart list: %v %s", err, out)
	}
	var left []string
	for _, l := range strings.Fields(string(out)) {
		if strings.HasPrefix(l, tart.ClonePrefix) {
			left = append(left, l)
		}
	}
	return left, nil
}

type tartIsolator struct{ p *tart.Profile }

func (i tartIsolator) Prepare(ctx context.Context, s *exec.JobSpec) error {
	if err := i.p.Prepare(ctx, isolation.Attempt{ID: s.AttemptID, CoordinatorID: s.CoordinatorID, Slot: s.Slot}); err != nil {
		return err
	}
	w, err := i.p.Wrap(*s)
	*s = w
	return err
}

func tartIntent(coord, token string) burst.Intent {
	return burst.Intent{Token: token, Spec: burst.Spec{Platform: "darwin/arm64", CPU: 4, MemMB: 8192, TimeoutMs: 1_200_000}, Labels: burst.IntentLabels(coord, token)}
}

func runTartJob(t *testing.T, prof *tart.Profile, res burst.Resource, script string) (exec.AttemptResult, string) {
	t.Helper()
	var out strings.Builder
	var mu sync.Mutex
	e := exec.Executor{Isolation: tartIsolator{prof}}
	r := e.Run(context.Background(), exec.JobSpec{AttemptID: res.ID, CoordinatorID: res.Labels[coordKey], Home: t.TempDir(), Network: "lan", Timeout: 20 * time.Minute,
		Job: model.Job{Kind: "run", Isolation: "vm"}, Command: []string{"/bin/sh", "-c", script},
		Output: func(b []byte) { mu.Lock(); out.Write(b); mu.Unlock() }})
	mu.Lock()
	defer mu.Unlock()
	return r, out.String()
}

func TestTartOnePerJob(t *testing.T) {
	p, prof := tartProvider(t, t.TempDir())
	ctx := context.Background()
	co := "ELTART" + strings.ReplaceAll(time.Now().Format("150405.000"), ".", "")
	if left := tartClones(t); len(left) != 0 {
		t.Fatalf("prerequisite missing: clones from an earlier run are still there: %v", left)
	}
	a, err := p.Provision(ctx, tartIntent(co, "ELTARTA"+co))
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Provision(ctx, tartIntent(co, "ELTARTB"+co))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Provision(ctx, tartIntent(co, "ELTARTC"+co)); err == nil {
		t.Fatal("Apple allows two VMs: a third slot was granted")
	}
	// the two jobs run together: at no time may more than two nself-ci clones exist
	var peak atomic.Int32
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Second):
				left, err := tartClonesErr()
				if err != nil {
					t.Error(err)
					return
				}
				if n := int32(len(left)); n > peak.Load() {
					peak.Store(n)
				}
			}
		}
	}()
	var wg sync.WaitGroup
	outs := make([]string, 2)
	errs := make([]error, 2)
	for i, r := range []burst.Resource{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started := time.Now()
			res, out := runTartJob(t, prof, r, `test -e /tmp/it-marker && echo REUSED || echo FRESH; touch /tmp/it-marker; echo job-ran`)
			outs[i], errs[i] = out, res.Err
			p.Observe(r.ID, time.Since(started))
		}()
	}
	wg.Wait()
	close(stop)
	for i := range outs {
		if errs[i] != nil || !strings.Contains(outs[i], "FRESH") || !strings.Contains(outs[i], "job-ran") {
			t.Fatalf("job %d: %v\n%s", i, errs[i], outs[i])
		}
	}
	if peak.Load() > 2 {
		t.Fatalf("%d clones at once, Apple's limit is 2", peak.Load())
	}
	for _, r := range []burst.Resource{a, b} {
		if err := p.Teardown(ctx, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	if left := tartClones(t); len(left) != 0 {
		t.Fatalf("clones left after the run: %v", left)
	}
	if _, err := p.Provision(ctx, tartIntent(co, "ELTARTC"+co)); err != nil {
		t.Fatalf("the slot did not come back after teardown: %v", err)
	}
	if e, _ := p.Estimate(ctx, burst.Spec{Platform: "darwin/arm64"}); e.LatencyMs <= 0 || e.Class != burst.ClassLocal {
		t.Fatalf("%+v", e)
	}
}

// A coordinator killed with a job running leaves a clone; the next coordinator's first Provision sweeps it.
func TestTartSweep(t *testing.T) {
	if os.Getenv("TART_SWEEP_HELPER") == "1" {
		p, prof := tartProvider(t, os.Getenv("TART_SWEEP_HOME"))
		res, err := p.Provision(context.Background(), tartIntent("ELKILLED", "ELKILLEDJOB"))
		if err != nil {
			t.Fatal(err)
		}
		runTartJob(t, prof, res, "sleep 600")
		return
	}
	tartNeed(t, "NSELF_TEST_TART_IMAGE")
	if left := tartClones(t); len(left) != 0 {
		t.Fatalf("prerequisite missing: clones from an earlier run are still there: %v", left)
	}
	home := t.TempDir()
	cmd := osexec.Command(os.Args[0], "-test.run=^TestTartSweep$", "-test.timeout=30m")
	cmd.Env = append(os.Environ(), "TART_SWEEP_HELPER=1", "TART_SWEEP_HOME="+home)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Minute)
	for len(tartClones(t)) == 0 {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the helper never produced a clone")
		}
		time.Sleep(5 * time.Second)
	}
	time.Sleep(time.Minute) // let the VM boot so the kill lands mid-job
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if len(tartClones(t)) == 0 {
		t.Fatal("the kill left no clone: the test proves nothing")
	}
	p, _ := tartProvider(t, home)
	res, err := p.Provision(context.Background(), tartIntent("ELNEXT", "ELNEXTJOB")) // Prepare runs the profile's lock-proven sweep
	if err != nil {
		t.Fatal(err)
	}
	if left := tartClones(t); len(left) != 0 {
		t.Fatalf("the sweep left: %v", left)
	}
	if err := p.Teardown(context.Background(), res.ID); err != nil {
		t.Fatal(err)
	}
}
