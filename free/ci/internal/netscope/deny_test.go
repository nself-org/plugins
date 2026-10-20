package netscope

import (
	"net/netip"
	"os"
	"regexp"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func mustAddr(t testing.TB, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAllowedPublicAndSpecialPurpose(t *testing.T) {
	for _, s := range []string{"93.184.216.34", "1.1.1.1", "2606:4700:4700::1111", "8.8.8.8"} {
		for _, scope := range []model.NetworkScope{"restricted", "internet"} {
			if v := Allowed(scope, mustAddr(t, s), Facts{}); !v.Allowed {
				t.Errorf("%s under %s refused: %+v", s, scope, v)
			}
		}
	}
	for _, s := range []string{"127.0.0.1", "169.254.169.254", "10.1.2.3", "172.16.0.1", "192.168.1.1", "100.64.0.1", "0.0.0.0", "192.0.2.5",
		"255.255.255.255", "240.0.0.1", "224.0.0.1", "198.18.0.1", "::1", "::", "fd00::1", "fe80::1", "ff02::1", "2001:db8::1", "2001:0:4136:e378::1", "100::1"} {
		for _, scope := range []model.NetworkScope{"restricted", "internet"} {
			if v := Allowed(scope, mustAddr(t, s), Facts{}); v.Allowed || v.Reason != ReasonSpecial {
				t.Errorf("%s under %s: %+v, want special_purpose", s, scope, v)
			}
		}
	}
}

// The mapped, NAT64 and 6to4 spellings of a denied IPv4 address are denied with it.
func TestAllowedRefusesEmbeddedIPv4Forms(t *testing.T) {
	for _, s := range []string{"::ffff:10.0.0.1", "::ffff:a00:1", "64:ff9b::a00:1", "64:ff9b:1::a00:1", "2002:a00:1::1", "::ffff:169.254.169.254", "::ffff:127.0.0.1"} {
		for _, scope := range []model.NetworkScope{"restricted", "internet"} {
			if v := Allowed(scope, mustAddr(t, s), Facts{}); v.Allowed {
				t.Errorf("%s under %s was allowed", s, scope)
			}
		}
	}
	// a mapped spelling of a PUBLIC address is the public address, and an exclusion on it still applies
	f := Facts{Exclusion: []netip.Prefix{netip.MustParsePrefix("93.184.216.34/32")}}
	if v := Allowed("internet", mustAddr(t, "::ffff:93.184.216.34"), f); v.Allowed || v.Reason != ReasonExclusion {
		t.Errorf("mapped exclusion address: %+v", v)
	}
	if v := Allowed("internet", mustAddr(t, "::ffff:93.184.216.35"), f); !v.Allowed {
		t.Errorf("mapped neighbour refused: %+v", v)
	}
}

func TestAllowedHonoursTheWorldsFacts(t *testing.T) {
	f := Facts{
		OnLink:    []netip.Prefix{netip.MustParsePrefix("203.1.1.0/24"), netip.MustParsePrefix("2a00:1450:4001::/64")},
		HostAddrs: []netip.Addr{mustAddr(t, "198.41.0.4")},
		Exclusion: []netip.Prefix{netip.MustParsePrefix("5.75.235.42/32"), netip.MustParsePrefix("2a01:4f8::/32")},
	}
	cases := []struct {
		addr string
		want Reason
	}{
		{"203.1.1.77", ReasonOnLink}, {"2a00:1450:4001::1234", ReasonOnLink}, {"198.41.0.4", ReasonHostAddr},
		{"5.75.235.42", ReasonExclusion}, {"2a01:4f8:1::9", ReasonExclusion}, {"::ffff:5.75.235.42", ReasonExclusion},
	}
	for _, c := range cases {
		if v := Allowed("internet", mustAddr(t, c.addr), f); v.Allowed || v.Reason != c.want {
			t.Errorf("%s: %+v, want %s", c.addr, v, c.want)
		}
	}
	for _, ok := range []string{"203.1.2.1", "2a00:1450:4002::1", "5.75.235.43", "2a01:4f9::1"} {
		if v := Allowed("restricted", mustAddr(t, ok), f); !v.Allowed {
			t.Errorf("%s refused: %+v", ok, v)
		}
	}
}

// Only restricted and internet reach the proxy; every other scope answers no, whatever the address.
func TestAllowedOtherScopesNeverServed(t *testing.T) {
	for _, scope := range []model.NetworkScope{"none", "lan", "privileged", "", "bogus"} {
		if v := Allowed(scope, mustAddr(t, "93.184.216.34"), Facts{}); v.Allowed || v.Reason != ReasonScope {
			t.Errorf("scope %q: %+v", scope, v)
		}
	}
	if v := Allowed("internet", netip.Addr{}, Facts{}); v.Allowed || v.Reason != ReasonInvalid {
		t.Errorf("invalid address: %+v", v)
	}
}

// NSELF_ALLOW_PRIVATE_URLS (the SDK guard's bypass) has no counterpart here.
func TestNoEnvironmentVariableLoosensTheDenySet(t *testing.T) {
	t.Setenv("NSELF_ALLOW_PRIVATE_URLS", "1")
	t.Setenv("NSELF_CI_ALLOW_PRIVATE", "1")
	if v := Allowed("internet", mustAddr(t, "10.0.0.1"), Facts{}); v.Allowed {
		t.Fatalf("private address allowed under an env bypass: %+v", v)
	}
}

// Every CIDR in the SDK's SSRF guard is covered by the deny table (the ci module does not require the SDK).
func TestDenyTableCoversTheSDKList(t *testing.T) {
	raw, err := os.ReadFile("../../../../sdk/httpx/ssrf.go")
	if err != nil {
		t.Fatalf("sdk/httpx/ssrf.go must be readable: %v", err)
	}
	cidrs := regexp.MustCompile(`"([0-9a-f:.]+/[0-9]+)"`).FindAllStringSubmatch(string(raw), -1)
	if len(cidrs) < 10 {
		t.Fatalf("found only %d CIDRs in ssrf.go", len(cidrs))
	}
	have := map[string]bool{}
	for _, c := range DenyTable() {
		have[c] = true
	}
	for _, m := range cidrs {
		if !have[m[1]] {
			t.Errorf("deny table misses %s", m[1])
		}
	}
	for _, extra := range []string{"::ffff:0:0/96", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "192.0.2.0/24", "224.0.0.0/4", "2001:db8::/32"} {
		if !have[extra] {
			t.Errorf("deny table misses %s", extra)
		}
	}
}

func FuzzDenyDecision(f *testing.F) {
	for _, s := range []string{"::ffff:10.0.0.1", "64:ff9b::a00:1", "2002:a00:1::1", "::ffff:169.254.169.254", "93.184.216.34", "2606:4700::1111", "::", "fe80::1%eth0", "0.0.0.0", "100.64.0.1"} {
		f.Add(s, uint8(0))
		f.Add(s, uint8(1))
	}
	facts := Facts{
		OnLink:    []netip.Prefix{netip.MustParsePrefix("203.1.1.0/24"), netip.MustParsePrefix("2a00:1450:4001::/64")},
		HostAddrs: []netip.Addr{netip.MustParseAddr("198.41.0.4")},
		Exclusion: []netip.Prefix{netip.MustParsePrefix("5.75.235.42/32")},
	}
	f.Fuzz(func(t *testing.T, s string, which uint8) {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return
		}
		scope := model.NetworkScope([]string{"restricted", "internet"}[which%2])
		v := Allowed(scope, a, facts)
		if !v.Allowed {
			return
		}
		// an allowed address is public after unmapping and in none of the sets
		u := a.WithZone("").Unmap()
		for _, c := range DenyTable() {
			if netip.MustParsePrefix(c).Contains(u) {
				t.Fatalf("%s allowed but in %s", s, c)
			}
		}
		if u.IsLoopback() || u.IsUnspecified() || u.IsMulticast() || u.IsLinkLocalUnicast() {
			t.Fatalf("%s allowed but special", s)
		}
		for _, p := range append(append([]netip.Prefix(nil), facts.OnLink...), facts.Exclusion...) {
			if p.Contains(u) {
				t.Fatalf("%s allowed but in %s", s, p)
			}
		}
		if u == facts.HostAddrs[0] {
			t.Fatalf("%s allowed but is the host", s)
		}
	})
}
