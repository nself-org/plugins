package netscope

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func scopesOf(s []model.NetworkScope) string {
	var out []string
	for _, x := range s {
		out = append(out, string(x))
	}
	return strings.Join(out, ",")
}

func newProber(rt *fakeRuntime) (*Prober, *time.Time) {
	now := time.Unix(1_000_000, 0)
	p := testPlanner(rt)
	rt.log = ReadyLine()
	return &Prober{Planner: p, Now: func() time.Time { return now }}, &now
}

func TestProbeProvesProxiedScopesWhenNothingIsReached(t *testing.T) {
	rt := &fakeRuntime{probeOut: []byte("reached 172.30.0.2:3128\ndone\n")}
	pr, _ := newProber(rt)
	got := pr.EnforcedScopes(context.Background(), "docker")
	if scopesOf(got) != "none,restricted,internet" {
		t.Fatalf("scopes %v", got)
	}
	// the probe dials the host's own non-loopback address on the fixture port plus the raw targets
	want := map[string]bool{"169.254.169.254:80": true, "1.1.1.1:443": true, "172.30.0.2:3128": true}
	hostTargets := 0
	for _, tg := range rt.targets {
		if want[tg] {
			delete(want, tg)
		} else if strings.HasPrefix(tg, "198.41.0.4:") {
			hostTargets++
		} else {
			t.Errorf("unexpected target %s", tg)
		}
	}
	if len(want) != 0 || hostTargets != 1 {
		t.Errorf("targets %v", rt.targets)
	}
	if rt.steps[len(rt.steps)-1] != "remove" {
		t.Errorf("probe left resources: %v", rt.steps)
	}
}

func TestProbeWithholdsProxiedScopesWhenAnythingIsReached(t *testing.T) {
	for name, out := range map[string]string{
		"reached host": "reached 172.30.0.2:3128\nreached 198.41.0.4:4000\ndone\n", "reached raw": "reached 172.30.0.2:3128\nreached 1.1.1.1:443\ndone\n",
		"control not reached": "done\n", "only host reached": "reached 198.41.0.4:4000\ndone\n",
		"cut short": "reached 172.30.0.2:3128\n", "garbled": "boom\n",
	} {
		rt := &fakeRuntime{probeOut: []byte(out)}
		pr, _ := newProber(rt)
		if got := pr.EnforcedScopes(context.Background(), "docker"); scopesOf(got) != "none" {
			t.Errorf("%s: scopes %v, want none only", name, got)
		}
	}
	rt := &fakeRuntime{probeErr: errors.New("image pull failed")}
	pr, _ := newProber(rt)
	if got := pr.EnforcedScopes(context.Background(), "docker"); scopesOf(got) != "none" {
		t.Errorf("probe error: %v", got)
	}
}

// A connection the host fixture accepts proves the job network has a route to the host.
func TestProbeWithholdsWhenTheHostFixtureSawAConnection(t *testing.T) {
	rt := &fakeRuntime{probeOut: []byte("reached 172.30.0.2:3128\ndone\n")}
	pr, _ := newProber(rt)
	pr.Listen = func() (net.Listener, error) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			go func() {
				c, e := net.Dial("tcp", ln.Addr().String())
				if e == nil {
					_ = c.Close()
				}
			}()
		}
		return ln, err
	}
	if got := pr.EnforcedScopes(context.Background(), "docker"); scopesOf(got) != "none" {
		t.Fatalf("scopes %v", got)
	}
}

func TestProbeSidecarFailureAndNoRuntimeClaimNothingProxied(t *testing.T) {
	rt := &fakeRuntime{failAt: "sidecar", probeOut: []byte("done\n")}
	pr, _ := newProber(rt)
	if got := pr.EnforcedScopes(context.Background(), "docker"); scopesOf(got) != "none" {
		t.Errorf("sidecar failure: %v", got)
	}
	if got := (&Prober{}).EnforcedScopes(context.Background(), ""); got != nil {
		t.Errorf("no runtime: %v", got)
	}
}

func TestProbeResultIsCached(t *testing.T) {
	rt := &fakeRuntime{probeOut: []byte("reached 172.30.0.2:3128\ndone\n")}
	pr, now := newProber(rt)
	pr.EnforcedScopes(context.Background(), "docker")
	n := len(rt.steps)
	pr.EnforcedScopes(context.Background(), "docker")
	if len(rt.steps) != n {
		t.Error("second call probed again inside the TTL")
	}
	*now = now.Add(probeTTL + time.Second)
	pr.EnforcedScopes(context.Background(), "docker")
	if len(rt.steps) == n {
		t.Error("no re-probe after the TTL")
	}
}

func TestProbeModeOutput(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	closedAddr := closed.Addr().String()
	_ = closed.Close()
	var out bytes.Buffer
	RunProbeMode(context.Background(), []string{ln.Addr().String(), closedAddr}, &out)
	got := Reached(out.Bytes())
	if len(got) != 1 || got[0] != ln.Addr().String() || !probeCompleted(out.Bytes()) {
		t.Fatalf("output %q reached %v", out.String(), got)
	}
}
