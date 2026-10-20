package tart

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

var gw = netip.MustParseAddr("192.168.64.1")

func facts() netscope.Facts {
	return netscope.Facts{
		OnLink:    []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24"), netip.MustParsePrefix("192.168.64.0/24"), netip.MustParsePrefix("fd00::/64")},
		HostAddrs: []netip.Addr{netip.MustParseAddr("192.168.1.20"), gw},
		Exclusion: []netip.Prefix{netip.MustParsePrefix("5.75.235.42/32"), netip.MustParsePrefix("10.9.0.0/16")},
	}
}

func covered(rules []netip.Prefix, a netip.Addr) bool {
	for _, p := range rules {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func TestSubtractAddrCoversEverythingButTheAddress(t *testing.T) {
	for _, base := range []string{"0.0.0.0/0", "192.168.64.0/24", "192.168.64.1/32", "10.0.0.0/8"} {
		rules := subtractAddr(netip.MustParsePrefix(base), gw)
		var total uint64
		for _, p := range rules {
			total += 1 << (32 - p.Bits())
		}
		want := uint64(1) << (32 - netip.MustParsePrefix(base).Bits())
		if netip.MustParsePrefix(base).Contains(gw) {
			want--
		}
		if total != want || covered(rules, gw) {
			t.Fatalf("%s: covers %d want %d, gateway covered=%v", base, total, want, covered(rules, gw))
		}
	}
}

func TestPlanSoftnetBelowLan(t *testing.T) {
	for _, scope := range []string{"", "none", "restricted", "internet"} {
		pl, err := planSoftnet(modelScope(scope), facts(), gw, true)
		if err != nil {
			t.Fatal(err)
		}
		if covered(pl.Block, gw) || len(pl.Allow) != 1 || pl.Allow[0] != netip.PrefixFrom(gw, 32) {
			t.Fatalf("%s: the gateway must be allowed exactly: block=%v allow=%v", scope, pl.Block, pl.Allow)
		}
		for _, must := range []string{"5.75.235.42", "10.9.3.4", "192.168.1.20", "192.168.1.77", "8.8.8.8", "1.1.1.1"} {
			if !covered(pl.Block, netip.MustParseAddr(must)) {
				t.Fatalf("%s: %s is not blocked", scope, must)
			}
		}
		argv := strings.Join(pl.Args, " ")
		if !strings.Contains(argv, "--net-softnet-block=") || !strings.Contains(argv, "--net-softnet-allow=192.168.64.1/32") || strings.Contains(argv, "fd00") {
			t.Fatalf("argv %s", argv)
		}
		if pl.NeedProxy != (scope == "restricted" || scope == "internet") {
			t.Fatalf("%s: NeedProxy=%v", scope, pl.NeedProxy)
		}
	}
}

func TestPlanSoftnetLanIsDefaultNAT(t *testing.T) {
	for _, scope := range []string{"lan", "privileged"} {
		pl, err := planSoftnet(modelScope(scope), netscope.Facts{}, gw, false)
		if err != nil || pl.Args != nil || pl.NeedProxy {
			t.Fatalf("%s: %+v %v", scope, pl, err)
		}
	}
}

func TestPlanSoftnetRefusals(t *testing.T) {
	if _, err := planSoftnet("restricted", facts(), gw, false); !isScopeErr(err) {
		t.Fatalf("no block option: %v", err)
	}
	if _, err := planSoftnet("bogus", facts(), gw, true); !isScopeErr(err) {
		t.Fatalf("unknown scope: %v", err)
	}
	if _, err := planSoftnet("none", facts(), netip.MustParseAddr("fd00::1"), true); !isScopeErr(err) {
		t.Fatalf("v6 gateway: %v", err)
	}
	big := netscope.Facts{}
	for i := 0; i < 600; i++ {
		big.Exclusion = append(big.Exclusion, netip.PrefixFrom(netip.AddrFrom4([4]byte{5, byte(i >> 8), byte(i), 1}), 32))
	}
	if _, err := planSoftnet("none", big, gw, true); !isScopeErr(err) {
		t.Fatalf("too many rules: %v", err)
	}
}

func isScopeErr(err error) bool {
	e, ok := err.(*isolation.Error)
	return ok && e.Code == "E679"
}

func TestPlanChecksAndJudge(t *testing.T) {
	px := netip.MustParseAddrPort("192.168.64.1:3128")
	c := planChecks(softnetPlan{Scope: "restricted"}, gw, &px, facts())
	script := string(c.script())
	for _, want := range []string{"tcp 192.168.64.1 3128", "png 1.1.1.1", "png 192.168.1.20", "png 5.75.235.42"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "png 192.168.64.1\n") && strings.Count(script, "png 192.168.64.1\n") > 1 {
		t.Fatal("the gateway is a positive control, never a negative target")
	}
	good := "OPEN 192.168.64.1:3128\nUP 192.168.64.1\nDOWN 1.1.1.1\nDOWN 192.168.1.20\nDOWN 5.75.235.42\nDOWN 10.9.0.1\nnoise\n"
	if err := c.judge([]byte(good)); err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{
		"proxy shut":     strings.Replace(good, "OPEN", "SHUT", 1),
		"public leaks":   strings.Replace(good, "DOWN 1.1.1.1", "UP 1.1.1.1", 1),
		"lan leaks":      strings.Replace(good, "DOWN 192.168.1.20", "UP 192.168.1.20", 1),
		"silence":        "",
		"missing target": strings.Replace(good, "DOWN 5.75.235.42\n", "", 1),
	} {
		if err := c.judge([]byte(out)); !isScopeErr(err) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// scope none has no proxy: the gateway ping is its only positive control
	cn := planChecks(softnetPlan{Scope: "none"}, gw, nil, facts())
	if err := cn.judge([]byte(strings.Replace(good, "UP 192.168.64.1", "DOWN 192.168.64.1", 1))); !isScopeErr(err) {
		t.Fatalf("none without positive control: %v", err)
	}
}

func TestRestrictedEndToEndWithProxyAndSelfCheck(t *testing.T) {
	var started []ProxyRequest
	closed := 0
	r := newRig(t, func(o *Options) {
		o.Net = NetSource{
			Facts: func(context.Context) (netscope.Facts, error) { return facts(), nil },
			Proxy: func(_ context.Context, req ProxyRequest) (ProxyHandle, error) {
				started = append(started, req)
				return ProxyHandle{Addr: netip.MustParseAddrPort("192.168.64.1:3128"), Close: func() { closed++ }}, nil
			},
		}
	})
	t.Setenv("FAKE_SSH_NETCHECK", "1")
	t.Setenv("FAKE_SSH_UP", "192.168.64.1")
	t.Setenv("FAKE_SSH_OPEN", "192.168.64.1:3128")
	s := r.spec("p1", "sh", "-c", `echo "proxy=$HTTPS_PROXY"`)
	s.Network = "restricted"
	res, out := r.run(context.Background(), s)
	if res.Err != nil {
		t.Fatalf("%v\n%s", res.Err, out)
	}
	if !strings.Contains(out, "proxy=http://192.168.64.1:3128") {
		t.Fatalf("the session must set the proxy variables: %q", out)
	}
	run := r.read("run.argv")
	if !strings.Contains(run, "--net-softnet-block=") || !strings.Contains(run, "--net-softnet-allow=192.168.64.1/32") || !strings.Contains(run, "--net-softnet ") {
		t.Fatalf("tart run argv: %s", run)
	}
	if len(started) != 1 || started[0].Scope != "restricted" || closed != 1 {
		t.Fatalf("proxy started %d closed %d", len(started), closed)
	}
	r.noClones()
}

func TestRestrictedFailsClosedWhenTheScopeLeaks(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.Net = NetSource{
			Facts: func(context.Context) (netscope.Facts, error) { return facts(), nil },
			Proxy: func(context.Context, ProxyRequest) (ProxyHandle, error) {
				return ProxyHandle{Addr: netip.MustParseAddrPort("192.168.64.1:3128"), Close: func() {}}, nil
			},
		}
	})
	t.Setenv("FAKE_SSH_NETCHECK", "1")
	t.Setenv("FAKE_SSH_UP", "192.168.64.1 1.1.1.1") // the canary answers: softnet did not block it
	t.Setenv("FAKE_SSH_OPEN", "192.168.64.1:3128")
	s := r.spec("p2", "echo", "never")
	s.Network = "restricted"
	res, out := r.run(context.Background(), s)
	if !isScopeErr(unwrapErr(res.Err)) || strings.Contains(out, "never") {
		t.Fatalf("a leaking scope must stop the job: %+v %q", res, out)
	}
	r.noClones()
}

func TestRestrictedWithoutProxyIsUnavailable(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.Net = NetSource{Facts: func(context.Context) (netscope.Facts, error) { return facts(), nil }}
	})
	s := r.spec("p3", "true")
	s.Network = "restricted"
	res, _ := r.run(context.Background(), s)
	if !res.Refused || !strings.Contains(res.Err.Error(), "E679") {
		t.Fatalf("%+v", res)
	}
	s.AttemptID, s.Network = "p4", "none" // none needs no proxy
	t.Setenv("FAKE_SSH_NETCHECK", "1")
	t.Setenv("FAKE_SSH_UP", "192.168.64.1")
	if res, _ := r.run(context.Background(), s); res.Err != nil {
		t.Fatalf("none: %v", res.Err)
	}
	r.noClones()
}

func unwrapErr(err error) error { return err }

func TestHostProxyRefusesDestinationsAndStopsTheListener(t *testing.T) {
	loop := netip.MustParseAddr("127.0.0.1")
	start := HostProxy(nil, func() (netscope.Facts, error) {
		return netscope.Facts{HostAddrs: []netip.Addr{loop}}, nil
	}, nil)
	h, err := start(context.Background(), ProxyRequest{Scope: "restricted", Gateway: loop, Facts: netscope.Facts{}})
	if err != nil {
		t.Fatal(err)
	}
	cl := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(mustURL("http://" + h.Addr.String()))}}
	resp, err := cl.Get("http://127.0.0.1:1/") // a loopback literal is never an allowed destination
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status %d", resp.StatusCode)
		}
	}
	h.Close()
	h.Close()
	if c, err := net.DialTimeout("tcp", h.Addr.String(), time.Second); err == nil {
		c.Close()
		t.Fatal("the listener must be closed")
	}
	if _, err := start(context.Background(), ProxyRequest{Scope: "restricted", Gateway: netip.MustParseAddr("192.168.64.1")}); !isScopeErr(err) {
		t.Fatalf("a gateway that is not a host address must fail: %v", err)
	}
}
