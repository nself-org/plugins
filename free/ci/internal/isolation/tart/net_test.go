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

func goodOutput(c netChecks) string {
	var b strings.Builder
	b.WriteString("OS Darwin\nnoise line\n")
	for _, ap := range c.TCPOpen {
		b.WriteString("OPEN " + tcpKey(ap) + "\n")
	}
	for _, ap := range c.TCPShut {
		b.WriteString("SHUT " + tcpKey(ap) + "\n")
	}
	for _, a := range c.PingUp {
		b.WriteString("UP " + a.String() + "\n")
	}
	for _, a := range c.PingDown {
		b.WriteString("DOWN " + a.String() + "\n")
	}
	for _, a := range c.Ping6Up {
		b.WriteString("UP " + a.String() + "\n")
	}
	for _, a := range c.Ping6Down {
		b.WriteString("DOWN " + a.String() + "\n")
	}
	for _, a := range c.Route6Down {
		b.WriteString("DOWN route:" + a.String() + "\n")
	}
	for _, d := range c.DNSDown {
		b.WriteString("DOWN dns:" + d + "\n")
	}
	return b.String()
}

func TestPlanChecksAndJudge(t *testing.T) {
	px := netip.MustParseAddrPort("192.168.64.1:3128")
	c := planChecks(softnetPlan{Scope: "restricted"}, gw, &px, facts())
	script := string(c.script())
	for _, want := range []string{"tcp 192.168.64.1 3128", "tcp 1.1.1.1 443", "tcp 2606:4700:4700::1111 443", "tcp 192.168.64.1 22", "tcp 192.168.64.1 445",
		"png 1.1.1.1", "png6 ::1", "png6 2606:4700:4700::1111", "rt6 2606:4700:4700::1111", "png 192.168.1.20", "png 5.75.235.42", "dns example.com", "uname -s"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %q:\n%s", want, script)
		}
	}
	if strings.Count(script, "png 192.168.64.1 &\n") != 1 {
		t.Fatal("the gateway is a positive control, never a negative target")
	}
	if strings.Contains(script, "tcp 192.168.64.1 3128\ntcp 192.168.64.1 22\ntcp 192.168.64.1 3128") {
		t.Fatal("dup")
	}
	good := goodOutput(c)
	if err := c.judge([]byte(good)); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(string) string{
		"proxy shut":        func(s string) string { return strings.Replace(s, "OPEN", "SHUT", 1) },
		"public ping leaks": func(s string) string { return strings.Replace(s, "DOWN 1.1.1.1", "UP 1.1.1.1", 1) },
		"public tcp leaks":  func(s string) string { return strings.Replace(s, "SHUT 1.1.1.1:443", "OPEN 1.1.1.1:443", 1) },
		"public v6 leaks": func(s string) string {
			return strings.Replace(s, "SHUT 2606:4700:4700::1111:443", "OPEN 2606:4700:4700::1111:443", 1)
		},
		"public v6 ping leaks": func(s string) string {
			return strings.Replace(s, "DOWN 2606:4700:4700::1111", "UP 2606:4700:4700::1111", 1)
		},
		"v6 route exists": func(s string) string {
			return strings.Replace(s, "DOWN route:2606:4700:4700::1111", "UP route:2606:4700:4700::1111", 1)
		},
		"no v6 positive control": func(s string) string { return strings.Replace(s, "UP ::1", "DOWN ::1", 1) },
		"lan leaks":              func(s string) string { return strings.Replace(s, "DOWN 192.168.1.20", "UP 192.168.1.20", 1) },
		"host sshd at gateway":   func(s string) string { return strings.Replace(s, "SHUT 192.168.64.1:22", "OPEN 192.168.64.1:22", 1) },
		"dns leaks":              func(s string) string { return strings.Replace(s, "DOWN dns:example.com", "UP dns:example.com", 1) },
		"linux guest":            func(s string) string { return strings.Replace(s, "OS Darwin", "OS Linux", 1) },
		"no gateway answer":      func(s string) string { return strings.Replace(s, "UP 192.168.64.1", "DOWN 192.168.64.1", 1) },
		"silence":                func(string) string { return "" },
		"missing target":         func(s string) string { return strings.Replace(s, "DOWN 5.75.235.42\n", "", 1) },
		"missing tcp":            func(s string) string { return strings.Replace(s, "SHUT 192.168.64.1:445\n", "", 1) },
	}
	for name, mut := range mutations {
		if err := c.judge([]byte(mut(good))); !isScopeErr(err) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// internet reaches the public net through the proxy, resolver included: no DNS negative
	ci := planChecks(softnetPlan{Scope: "internet"}, gw, &px, facts())
	if len(ci.DNSDown) != 0 || len(c.DNSDown) != 1 {
		t.Fatal("dns negative applies below internet only")
	}
	// scope none has no proxy and no TCP positive control: the all-DOWN output must still fail without the gateway answer
	cn := planChecks(softnetPlan{Scope: "none"}, gw, nil, facts())
	if len(cn.TCPOpen) != 0 {
		t.Fatal("none has no proxy")
	}
	if err := cn.judge([]byte(strings.Replace(goodOutput(cn), "UP 192.168.64.1", "DOWN 192.168.64.1", 1))); !isScopeErr(err) {
		t.Fatalf("none without positive control: %v", err)
	}
}

func TestSourceFilterServesOnlyTheVM(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	f := &sourceFilter{Listener: ln, allow: netip.MustParseAddr("127.0.0.2")} // not the dialer's address
	go func() {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err == nil {
			defer c.Close()
			buf := make([]byte, 1)
			c.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, err := c.Read(buf); err == nil {
				t.Error("the filtered connection must be closed, not served")
			}
		}
		ln.Close() // end Accept
	}()
	if c, err := f.Accept(); err == nil {
		c.Close()
		t.Fatal("a connection from another address was accepted")
	}
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln2.Close()
	ok := &sourceFilter{Listener: ln2, allow: netip.MustParseAddr("127.0.0.1")}
	go func() {
		c, _ := net.Dial("tcp", ln2.Addr().String())
		if c != nil {
			defer c.Close()
			time.Sleep(200 * time.Millisecond)
		}
	}()
	c, err := ok.Accept()
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
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
	if len(started) != 1 || started[0].Scope != "restricted" || closed != 1 || started[0].VMAddr != netip.MustParseAddr("192.168.64.7") {
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
	h, err := start(context.Background(), ProxyRequest{Scope: "restricted", Gateway: loop, VMAddr: loop, Facts: netscope.Facts{}})
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
	if _, err := start(context.Background(), ProxyRequest{Scope: "restricted", Gateway: netip.MustParseAddr("192.168.64.1"), VMAddr: loop}); !isScopeErr(err) {
		t.Fatalf("a gateway that is not a host address must fail: %v", err)
	}
}
