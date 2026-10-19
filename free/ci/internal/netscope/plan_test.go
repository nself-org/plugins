package netscope

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// fakeRuntime records the runtime calls of a plan and can fail at a chosen step.
type fakeRuntime struct {
	mu        sync.Mutex
	steps     []string
	failAt    string
	log       []byte
	sidecar   exec.SidecarSpec
	probeOut  []byte
	probeErr  error
	targets   []string
	proxy     string
	probeRuns int
}

func (f *fakeRuntime) step(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, name)
	if f.failAt == name {
		return errors.New(name + " failed")
	}
	return nil
}
func (f *fakeRuntime) NetworkCreate(_ context.Context, _, _, _ string) (netip.Prefix, error) {
	return netip.MustParsePrefix("172.30.0.0/24"), f.step("network")
}
func (f *fakeRuntime) SidecarStart(_ context.Context, _ string, s exec.SidecarSpec, _ netip.Prefix) (netip.Addr, error) {
	f.sidecar = s
	return netip.MustParseAddr("172.30.0.2"), f.step("sidecar")
}
func (f *fakeRuntime) SidecarLog(context.Context, string, string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.log, nil
}
func (f *fakeRuntime) Remove(context.Context, string, string) error { return f.step("remove") }
func (f *fakeRuntime) ProbeRun(_ context.Context, _, _, _, _, proxy string, targets []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.targets = append(f.targets, targets...)
	f.proxy = proxy
	f.probeRuns++
	return f.probeOut, f.probeErr
}

// fakeResolver for the excluded names: answers from a map, "missing" is NXDOMAIN, "broken" times out.
type nameResolver map[string][]string

func (r nameResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	switch host {
	case "missing.example.com":
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	case "broken.example.com":
		return nil, &net.DNSError{Err: "i/o timeout", Name: host, IsTimeout: true}
	}
	var out []netip.Addr
	for _, s := range r[host] {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

func testPlanner(rt *fakeRuntime) *Planner {
	return &Planner{
		Runtime: rt,
		Binary:  func() (string, error) { return "/opt/nself/nself-ci", nil },
		Host: func() (Facts, error) {
			return Facts{HostAddrs: []netip.Addr{netip.MustParseAddr("198.41.0.4"), netip.MustParseAddr("127.0.0.1")},
				OnLink: []netip.Prefix{netip.MustParsePrefix("198.41.0.0/24")}}, nil
		},
		Ready:    time.Second,
		Resolver: nameResolver{"prod.example.com": {"93.184.216.50", "2606:2800::5"}},
	}
}

var att = isolation.Attempt{ID: "a1", CoordinatorID: "c1"}

func TestPlanNoneAndLanCreateNothing(t *testing.T) {
	rt := &fakeRuntime{}
	p := testPlanner(rt)
	for scope, want := range map[model.NetworkScope]string{"": "none", "none": "none", "lan": "bridge"} {
		plan, err := p.PlanAttempt(context.Background(), "docker", att, scope)
		if err != nil || plan.Network != want || plan.Release != nil || len(plan.Env) != 0 {
			t.Errorf("%q: %+v %v", scope, plan, err)
		}
	}
	if _, err := p.PlanAttempt(context.Background(), "docker", att, "privileged"); err == nil {
		t.Error("privileged planned")
	}
	if len(rt.steps) != 0 {
		t.Errorf("runtime calls %v", rt.steps)
	}
	// the stateless Plan never serves a proxied scope: no attempt, no proxy, so E679
	if _, err := p.Plan("restricted"); err == nil || !strings.Contains(err.Error(), "E679") {
		t.Errorf("Plan(restricted) = %v, want E679", err)
	}
}

func TestPlanRestrictedBuildsNetworkSidecarAndProxyEnv(t *testing.T) {
	rt := &fakeRuntime{log: ReadyLine()}
	p := testPlanner(rt)
	plan, err := p.PlanAttempt(context.Background(), "docker", att, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Network != "nself-ci-a1-net" || len(plan.DNS) != 1 || plan.DNS[0] != "127.0.0.1" || plan.Release == nil {
		t.Fatalf("plan %+v", plan)
	}
	joined := strings.Join(plan.Env, "\n")
	for _, want := range []string{"HTTP_PROXY=http://172.30.0.2:3128", "HTTPS_PROXY=http://172.30.0.2:3128", "https_proxy=http://172.30.0.2:3128", "NO_PROXY=\n"} {
		if !strings.Contains(joined+"\n", want) {
			t.Errorf("env misses %q:\n%s", want, joined)
		}
	}
	if strings.Join(rt.steps, ",") != "network,sidecar" {
		t.Errorf("steps %v", rt.steps)
	}
	if rt.sidecar.Image != ProxyImage || rt.sidecar.ProxyBinary != "/opt/nself/nself-ci" || rt.sidecar.Attempt != "a1" {
		t.Errorf("sidecar spec %+v", rt.sidecar)
	}
	c, f, err := DecodeConfig(rt.sidecar.Config)
	if err != nil || c.Scope != "restricted" || c.JobPrefix != "172.30.0.0/24" || len(f.HostAddrs) != 2 {
		t.Errorf("config %+v %+v %v", c, f, err)
	}
	plan.Release(context.Background())
	plan.Release(context.Background()) // idempotent
	if strings.Join(rt.steps, ",") != "network,sidecar,remove" {
		t.Errorf("steps after release %v", rt.steps)
	}
}

func TestPlanReleaseEmitsEachDenialOnce(t *testing.T) {
	denial := Event{Scope: "restricted", Host: "10.0.0.1", Port: "443", Reason: ReasonSpecial, Detail: "10.0.0.0/8"}.Encode()
	rt := &fakeRuntime{log: append(append([]byte(nil), ReadyLine()...), denial...)}
	p := testPlanner(rt)
	var got []Event
	p.Events = func(attempt string, e Event) {
		if attempt != "a1" {
			t.Errorf("attempt %q", attempt)
		}
		got = append(got, e)
	}
	plan, err := p.PlanAttempt(context.Background(), "docker", att, "internet")
	if err != nil {
		t.Fatal(err)
	}
	plan.Release(context.Background())
	plan.Release(context.Background())
	if len(got) != 1 || got[0].Host != "10.0.0.1" || got[0].Attempt != "a1" {
		t.Fatalf("events %+v", got)
	}
}

// A failure at any step removes what was created and reports E679: the scope is unavailable, never approximated.
func TestPlanFailureCleansUpAndIsE679(t *testing.T) {
	for _, step := range []string{"network", "sidecar"} {
		rt := &fakeRuntime{log: ReadyLine(), failAt: step}
		plan, err := testPlanner(rt).PlanAttempt(context.Background(), "docker", att, "restricted")
		if err == nil || !strings.Contains(err.Error(), "E679") {
			t.Fatalf("%s: err %v, want E679", step, err)
		}
		if plan.Release != nil {
			t.Errorf("%s: a failed plan still holds a Release", step)
		}
		// a failed create removes nothing by label (the id may belong to another attempt); a later step does
		if removed := rt.steps[len(rt.steps)-1] == "remove"; removed != (step != "network") {
			t.Errorf("%s: steps %v", step, rt.steps)
		}
	}
}

func TestPlanSidecarNeverReadyIsE679(t *testing.T) {
	rt := &fakeRuntime{log: []byte("exec format error\n")}
	p := testPlanner(rt)
	p.Ready = 150 * time.Millisecond
	_, err := p.PlanAttempt(context.Background(), "docker", att, "restricted")
	if err == nil || !strings.Contains(err.Error(), "E679") || !strings.Contains(err.Error(), "exec format error") {
		t.Fatalf("err %v", err)
	}
	if rt.steps[len(rt.steps)-1] != "remove" {
		t.Errorf("steps %v", rt.steps)
	}
}

func TestPlanExclusionSource(t *testing.T) {
	rt := &fakeRuntime{log: ReadyLine()}
	p := testPlanner(rt)
	p.Exclusion = func(context.Context) (Exclusions, error) {
		return Exclusions{Hosts: []string{"Prod.Example.com."}, Addresses: []string{"5.75.235.42", "2a01:4f8::1"}, Prefixes: []string{"167.235.195.0/24"}}, nil
	}
	p.ExtraHosts = func(isolation.Attempt) []string { return []string{"ci.example.net"} }
	if _, err := p.PlanAttempt(context.Background(), "docker", att, "restricted"); err != nil {
		t.Fatal(err)
	}
	c, f, err := DecodeConfig(rt.sidecar.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Exclusion) != 5 || len(f.ExclusionHosts) != 1 || f.ExclusionHosts[0] != "prod.example.com" || len(c.Hosts) != 1 {
		t.Errorf("config exclusion=%v hosts=%v extra=%v", f.Exclusion, f.ExclusionHosts, c.Hosts)
	}
	// internet takes no policy hosts
	rt2 := &fakeRuntime{log: ReadyLine()}
	p2 := testPlanner(rt2)
	p2.ExtraHosts = p.ExtraHosts
	if _, err := p2.PlanAttempt(context.Background(), "docker", att, "internet"); err != nil {
		t.Fatal(err)
	}
	if c2, _, _ := DecodeConfig(rt2.sidecar.Config); len(c2.Hosts) != 0 {
		t.Errorf("internet config carries hosts %v", c2.Hosts)
	}
	// a registered source that cannot be read makes the scope unavailable
	rt3 := &fakeRuntime{log: ReadyLine()}
	p3 := testPlanner(rt3)
	p3.Exclusion = func(context.Context) (Exclusions, error) { return Exclusions{}, errors.New("store down") }
	if _, err := p3.PlanAttempt(context.Background(), "docker", att, "restricted"); err == nil || !strings.Contains(err.Error(), "E679") {
		t.Errorf("unreadable exclusion set: %v", err)
	}
	if len(rt3.steps) != 0 {
		t.Errorf("resources created before the exclusion set was read: %v", rt3.steps)
	}
}

func TestProxyEnvIsPlainAndHasNoSecretShape(t *testing.T) {
	for _, kv := range ProxyEnv(netip.MustParseAddr("172.30.0.2")) {
		if strings.ContainsAny(kv, "\r\n@") {
			t.Errorf("env entry %q", kv)
		}
	}
}

// The excluded NAMES are resolved at setup and their addresses join the exclusion, so dialling the address (or
// another name for it) is refused too.
func TestPlanResolvesExcludedNamesAndFailsClosed(t *testing.T) {
	rt := &fakeRuntime{log: ReadyLine()}
	p := testPlanner(rt)
	p.Exclusion = func(context.Context) (Exclusions, error) {
		return Exclusions{Hosts: []string{"prod.example.com", "missing.example.com"}}, nil
	}
	if _, err := p.PlanAttempt(context.Background(), "docker", att, "internet"); err != nil {
		t.Fatal(err)
	}
	_, f, _ := DecodeConfig(rt.sidecar.Config)
	if v := Allowed("internet", netip.MustParseAddr("93.184.216.50"), f); v.Allowed || v.Reason != ReasonExclusion {
		t.Errorf("the address of an excluded name is allowed: %+v", v)
	}
	if v := Allowed("internet", netip.MustParseAddr("2606:2800::5"), f); v.Allowed {
		t.Errorf("the v6 address of an excluded name is allowed: %+v", v)
	}
	for name, ex := range map[string]Exclusions{
		"lookup failure": {Hosts: []string{"broken.example.com"}}, "bad address": {Addresses: []string{"nope"}},
		"bad prefix": {Prefixes: []string{"1.2.3.4"}}, "bad host": {Hosts: []string{"a b"}},
	} {
		rt2 := &fakeRuntime{log: ReadyLine()}
		p2 := testPlanner(rt2)
		p2.Exclusion = func(context.Context) (Exclusions, error) { return ex, nil }
		if _, err := p2.PlanAttempt(context.Background(), "docker", att, "restricted"); err == nil || !strings.Contains(err.Error(), "E679") {
			t.Errorf("%s: %v", name, err)
		}
		if len(rt2.steps) != 0 {
			t.Errorf("%s: resources created: %v", name, rt2.steps)
		}
	}
}

// A create that fails (a duplicate attempt id) must not remove what another attempt with that label owns.
func TestPlanCreateFailureReleasesNothing(t *testing.T) {
	rt := &fakeRuntime{failAt: "network"}
	plan, err := testPlanner(rt).PlanAttempt(context.Background(), "docker", att, "restricted")
	if err == nil || plan.Release != nil {
		t.Fatalf("%v %v", plan, err)
	}
	for _, s := range rt.steps {
		if s == "remove" {
			t.Errorf("steps %v: the label-wide removal ran after a failed create", rt.steps)
		}
	}
}

func TestPlanReportsUnreadableLogAndFailedCleanup(t *testing.T) {
	rt := &fakeRuntime{log: ReadyLine()}
	p := testPlanner(rt)
	var got []Event
	p.Events = func(_ string, e Event) { got = append(got, e) }
	plan, err := p.PlanAttempt(context.Background(), "docker", att, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	rt.failAt = "remove"
	plan.Release(context.Background())
	if len(got) != 1 || got[0].Reason != ReasonCleanupFailed {
		t.Fatalf("%+v", got)
	}
}
