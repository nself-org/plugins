package gvisor

import (
	"context"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	dockerInfoRootful  = `{"ServerVersion":"27.0.1","MemoryLimit":true,"PidsLimit":true,"CPUCfsQuota":true,"SecurityOptions":["name=seccomp,profile=builtin"]}`
	dockerInfoRootless = `{"ServerVersion":"27.0.1","MemoryLimit":true,"PidsLimit":true,"CPUCfsQuota":true,"SecurityOptions":["name=seccomp,profile=builtin","name=rootless"]}`
	podmanInfo         = `{"host":{"security":{"rootless":true},"ociRuntime":{"name":"crun"},"cgroupVersion":"v2","cgroupControllers":["cpu","memory","pids"]},"version":{"Version":"5.2.0"}}`
	banner             = "[    0.000000] Starting gVisor...\n[    0.4] Reticulating splines..."
)

// fakeRunner answers `info` (whole and the runtime table) from canned JSON.
type fakeRunner struct {
	ctxAware   bool // fail a query whose context ended, as exec.RuntimeQuery does
	mu         sync.Mutex
	info       map[string]string
	runtimes   string
	runtimeErr error
}

func (f *fakeRunner) setRuntimes(table string) { f.mu.Lock(); f.runtimes = table; f.mu.Unlock() }

func (f *fakeRunner) Query(ctx context.Context, binary string, args ...string) ([]byte, error) {
	if f.ctxAware && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(strings.Join(args, " "), ".Runtimes") {
		if f.runtimeErr != nil {
			return nil, f.runtimeErr
		}
		return []byte(f.runtimes), nil
	}
	if out, ok := f.info[binary]; ok {
		return []byte(out), nil
	}
	return nil, fs.ErrNotExist
}

// fakeStarter records what the probe started.
type fakeStarter struct {
	mu                                  sync.Mutex
	delay                               time.Duration
	out                                 string
	err                                 error
	version                             string
	versionErr                          error
	starts                              int
	binary, runtime, image, versionPath string
}

func (f *fakeStarter) Banner(ctx context.Context, binary, runtime, image string) ([]byte, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	f.binary, f.runtime, f.image = binary, runtime, image
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return []byte(f.out), f.err
}

func (f *fakeStarter) Version(_ context.Context, path string) (string, error) {
	f.versionPath = path
	return f.version, f.versionErr
}

func newProfile(r *fakeRunner, s *fakeStarter, mut func(*Options)) *Profile {
	o := Options{Runner: r, Starter: s, HasKVM: func() bool { return false }, UIDs: func() []int { return []int{501} }}
	if mut != nil {
		mut(&o)
	}
	return New(o)
}

func okRunner(runtimes string) *fakeRunner {
	return &fakeRunner{info: map[string]string{"docker": dockerInfoRootful}, runtimes: runtimes}
}

func okStarter() *fakeStarter {
	return &fakeStarter{out: banner, version: "runsc version release-20261012.0"}
}

func TestProbeAvailableStartsTheProbeImageUnderRunsc(t *testing.T) {
	st := okStarter()
	p := newProfile(okRunner(`{"runc":{"path":"runc"},"runsc":{"path":"/usr/local/bin/runsc"}}`), st, nil)
	av := p.Probe(context.Background())
	if av.Level != "sandboxed-container" || av.Reason != "" {
		t.Fatalf("not available: %+v", av)
	}
	if st.binary != "docker" || st.runtime != "runsc" || st.image != ProbeImage || st.versionPath != "/usr/local/bin/runsc" {
		t.Fatalf("probe started %+v", st)
	}
	for k, want := range map[string]string{"platform": "systrap", "runsc_version": "runsc version release-20261012.0", "sandbox_runtime": "runsc", "runtime": "docker", "banner": "gvisor"} {
		if av.Facts[k] != want {
			t.Errorf("fact %s = %q, want %q", k, av.Facts[k], want)
		}
	}
}

func TestProbeImageIsPinnedByDigest(t *testing.T) {
	if !regexp.MustCompile(`^docker\.io/library/busybox@sha256:[0-9a-f]{64}$`).MatchString(ProbeImage) {
		t.Fatalf("probe image must be a tagless ref@sha256 digest: %s", ProbeImage)
	}
}

func TestProbeUnavailableReasons(t *testing.T) {
	denyCases := map[string][]string{
		"network host":          {"--network=host"},
		"network host spaced":   {"--network", "host"},
		"single dash":           {"-network=host"},
		"exclusive file access": {"--file-access=exclusive"},
		"host uds":              {"--host-uds=open"},
		"host uds bare":         {"--host-uds"},
		"host fifo":             {"--host-fifo=all"},
		"net raw":               {"--net-raw"},
		"allow flag override":   {"--allow-flag-override"},
		"denied after a good":   {"--platform=ptrace", "--debug", "--net-raw"},
	}
	for name, args := range denyCases {
		t.Run("denied "+name, func(t *testing.T) {
			st := okStarter()
			table := `{"runsc":{"path":"runsc","runtimeArgs":[` + quoteAll(args) + `]}}`
			av := newProfile(okRunner(table), st, nil).Probe(context.Background())
			if av.Available() || av.Reason != ReasonRuntimeFlagDenied || st.starts != 0 {
				t.Fatalf("%+v starts=%d", av, st.starts)
			}
		})
	}
	for name, tc := range map[string]struct {
		runner  *fakeRunner
		starter *fakeStarter
		mut     func(*Options)
		reason  string
		starts  int
	}{
		"runsc not registered":           {okRunner(`{"runc":{"path":"runc"}}`), okStarter(), nil, isolation.ReasonRuntimeMissing, 0},
		"runtime table broken":           {okRunner(`nope`), okStarter(), nil, isolation.ReasonProbeFailed, 0},
		"runtime table query fails":      {&fakeRunner{info: map[string]string{"docker": dockerInfoRootful}, runtimeErr: errors.New("daemon gone")}, okStarter(), nil, isolation.ReasonProbeFailed, 0},
		"registered but will not start":  {okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), &fakeStarter{err: errors.New("exit 125")}, nil, isolation.ReasonProbeFailed, 1},
		"started without the banner":     {okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), &fakeStarter{out: "[    0.000000] Linux version 6.8.0"}, nil, isolation.ReasonProbeFailed, 1},
		"kvm without /dev/kvm":           {okRunner(`{"runsc":{"path":"runsc","runtimeArgs":["--platform=kvm"]}}`), okStarter(), nil, ReasonPlatformUnsupported, 0},
		"unknown platform":               {okRunner(`{"runsc":{"path":"runsc","runtimeArgs":["--platform","bogus"]}}`), okStarter(), nil, ReasonPlatformUnsupported, 0},
		"no container runtime":           {&fakeRunner{info: map[string]string{}}, okStarter(), nil, isolation.ReasonRuntimeMissing, 0},
		"podman only":                    {&fakeRunner{info: map[string]string{"podman": podmanInfo}, runtimes: `{"runsc":{"path":"/usr/local/bin/runsc"}}`}, okStarter(), nil, isolation.ReasonRuntimeMissing, 0},
		"separated node, rootful docker": {okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), okStarter(), func(o *Options) { o.Separated = func() bool { return true } }, isolation.ReasonRootful, 0},
	} {
		t.Run(name, func(t *testing.T) {
			av := newProfile(tc.runner, tc.starter, tc.mut).Probe(context.Background())
			if av.Available() || av.Level != "" || av.Reason != tc.reason || tc.starter.starts != tc.starts {
				t.Fatalf("level %q reason %q starts %d: %+v", av.Level, av.Reason, tc.starter.starts, av)
			}
		})
	}
}

func TestProbeRuntimeMissingCarriesTheInstallHint(t *testing.T) {
	av := newProfile(okRunner(`{"runc":{}}`), okStarter(), nil).Probe(context.Background())
	if !strings.Contains(av.Facts["hint"], "runsc install") {
		t.Fatalf("no install hint: %+v", av)
	}
}

func TestProbePlatforms(t *testing.T) {
	for name, tc := range map[string]struct {
		args string
		kvm  bool
		want string
	}{
		"default is systrap":    {``, false, "systrap"},
		"ptrace":                {`"--platform=ptrace"`, false, "ptrace"},
		"spaced form":           {`"--platform","ptrace"`, false, "ptrace"},
		"last one wins":         {`"--platform=kvm","--platform=systrap"`, false, "systrap"},
		"kvm where /dev/kvm is": {`"--platform=kvm"`, true, "kvm"},
		"safe flags accepted":   {`"--debug-log=/tmp/x","--strace"`, false, "systrap"},
	} {
		t.Run(name, func(t *testing.T) {
			table := `{"runsc":{"path":"runsc","runtimeArgs":[` + tc.args + `]}}`
			p := newProfile(okRunner(table), okStarter(), func(o *Options) { o.HasKVM = func() bool { return tc.kvm } })
			av := p.Probe(context.Background())
			if !av.Available() || av.Facts["platform"] != tc.want {
				t.Fatalf("%+v", av)
			}
		})
	}
}

func TestProbeVersionFailureIsNotAvailabilityFailure(t *testing.T) {
	st := okStarter()
	st.versionErr = errors.New("not on PATH")
	av := newProfile(okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), st, nil).Probe(context.Background())
	if !av.Available() || av.Facts["runsc_version"] != "unknown" {
		t.Fatalf("%+v", av)
	}
}

func TestProbeIsCachedForTheTTL(t *testing.T) {
	now := time.Now()
	st := okStarter()
	p := newProfile(okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), st, func(o *Options) { o.Now = func() time.Time { return now } })
	p.Probe(context.Background())
	p.Probe(context.Background())
	if st.starts != 1 {
		t.Fatalf("probe started %d times inside the TTL", st.starts)
	}
	now = now.Add(2 * time.Minute)
	p.Probe(context.Background())
	if st.starts != 2 {
		t.Fatalf("probe did not refresh after the TTL: %d", st.starts)
	}
}

func TestPrepareAndWrapRefuseWithE678WhenUnavailable(t *testing.T) {
	p := newProfile(okRunner(`{"runc":{}}`), okStarter(), nil)
	err := p.Prepare(context.Background(), isolation.Attempt{ID: "a1"})
	var ie *isolation.Error
	if !errors.As(err, &ie) || ie.Code != "E678" || ie.Reason != isolation.ReasonRuntimeMissing {
		t.Fatalf("prepare: %v", err)
	}
	spec, err := p.Wrap(exec.JobSpec{AttemptID: "a1"})
	if !errors.As(err, &ie) || ie.Code != "E678" || spec.Plan != nil {
		t.Fatalf("wrap must fail without a plan, never fall back to container: %v %+v", err, spec.Plan)
	}
}

func TestRegistrySelectsSandboxedOnlyWhenRunscStarts(t *testing.T) {
	reg := &isolation.Registry{}
	reg.Register(newProfile(okRunner(`{"runc":{}}`), okStarter(), nil))
	_, av, err := reg.Select(context.Background(), "sandboxed-container")
	var ie *isolation.Error
	if !errors.As(err, &ie) || ie.Code != "E678" || av.Available() {
		t.Fatalf("a host without runsc must be unavailable: %+v %v", av, err)
	}
	reg = &isolation.Registry{}
	reg.Register(newProfile(okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), okStarter(), nil))
	p, av, err := reg.Select(context.Background(), "sandboxed-container")
	if err != nil || p == nil || p.Level() != model.Isolation("sandboxed-container") || av.Level != "sandboxed-container" {
		t.Fatalf("%v %+v %v", p, av, err)
	}
}

func quoteAll(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = `"` + a + `"`
	}
	return strings.Join(q, ",")
}

// A daemon reload can add a denied argument while a start is cached: the table is read on every call.
func TestRuntimeTableIsReadOnEveryProbe(t *testing.T) {
	r, st := okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), okStarter()
	p := newProfile(r, st, nil)
	if !p.Probe(context.Background()).Available() {
		t.Fatal("first probe must be available")
	}
	r.setRuntimes(`{"runsc":{"path":"/usr/local/bin/runsc","runtimeArgs":["--network=host"]}}`)
	if av := p.Probe(context.Background()); av.Available() || av.Reason != ReasonRuntimeFlagDenied {
		t.Fatalf("a cached start hid a newly denied argument: %+v", av)
	}
	if st.starts != 1 {
		t.Fatalf("the table check must not start a container: %d starts", st.starts)
	}
	r.setRuntimes(`{"runsc":{"path":"/usr/local/bin/runsc","runtimeArgs":["--platform=ptrace"],"status":{"x":"y"}}}`)
	if av := p.Probe(context.Background()); !av.Available() || av.Facts["platform"] != "ptrace" || st.starts != 2 {
		t.Fatalf("a changed runtime entry must be started again: %+v starts=%d", av, st.starts)
	}
}

// A start cut short by the caller's context says nothing about the host and must not be remembered.
func TestCancelledProbeIsNotCached(t *testing.T) {
	st := okStarter()
	p := newProfile(okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), st, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if av := p.Probe(ctx); av.Available() {
		t.Fatalf("a cancelled probe reported available: %+v", av)
	}
	if av := p.Probe(context.Background()); !av.Available() {
		t.Fatalf("the cancelled result was cached: %+v", av)
	}
}

func TestConcurrentProbesShareOneStart(t *testing.T) {
	st := okStarter()
	st.delay = 50 * time.Millisecond
	p := newProfile(okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), st, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			av := p.Probe(context.Background())
			if !av.Available() {
				t.Errorf("%+v", av)
			}
			av.Facts["banner"] = "edited by the caller" // must not reach the cache
		}()
	}
	wg.Wait()
	if st.starts != 1 {
		t.Fatalf("%d containers started for 20 concurrent probes", st.starts)
	}
	if got := p.Probe(context.Background()).Facts["banner"]; got != "gvisor" {
		t.Fatalf("a caller edited the cached facts: %q", got)
	}
}

func TestBannerMustBeAWholeLine(t *testing.T) {
	for out, want := range map[string]bool{
		"[    0.000000] Starting gVisor...\n[    0.4] Reticulating splines...": true,
		"Starting gVisor...\n":                                             true,
		"[    0.000000] Starting gVisor...   \n":                           true,
		"[ 1234.5] Starting gVisor[4242]: segfault at 0 ip 0 sp 0 error 4": false, // a host log line naming a process so
		"starting gvisor":                                                  false,
		"[    0.000000] Linux version 6.8.0":                               false,
		"":                                                                 false,
	} {
		if got := bannerSeen([]byte(out)); got != want {
			t.Errorf("bannerSeen(%q) = %v, want %v", out, got, want)
		}
	}
}

func TestProbeGivesUpWaitingWhenItsContextEnds(t *testing.T) {
	st := okStarter()
	st.delay = 300 * time.Millisecond
	p := newProfile(okRunner(`{"runsc":{"path":"/usr/local/bin/runsc"}}`), st, nil)
	go p.Probe(context.Background()) // holds the start
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if av := p.Probe(ctx); av.Available() || time.Since(start) > 250*time.Millisecond {
		t.Fatalf("waiter did not give up with its context: %+v after %s", av, time.Since(start))
	}
}
