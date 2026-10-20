package netscope

import (
	"net/netip"
	"testing"
)

func TestMergeKeepsEveryListOnce(t *testing.T) {
	a := Facts{OnLink: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("0.0.0.0/0")}, HostAddrs: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
		ExclusionHosts: []string{"a.example"}}
	b := Facts{OnLink: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2a00::/64")}, HostAddrs: []netip.Addr{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("2a00::1")},
		Exclusion: []netip.Prefix{netip.MustParsePrefix("5.75.235.42/32")}, ExclusionHosts: []string{"a.example", "b.example"}}
	m := Merge(a, b)
	if len(m.OnLink) != 2 || len(m.HostAddrs) != 2 || len(m.Exclusion) != 1 || len(m.ExclusionHosts) != 2 {
		t.Fatalf("%+v", m)
	}
	for _, p := range m.OnLink {
		if p.Bits() == 0 {
			t.Errorf("a default route %s counts as on-link: every address would be refused", p)
		}
	}
}

func TestDiscoverFindsTheHostsOwnAddresses(t *testing.T) {
	f, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.HostAddrs) == 0 || len(f.OnLink) == 0 {
		t.Fatalf("no addresses discovered: %+v", f)
	}
	for _, a := range f.HostAddrs {
		if v := Allowed("internet", a, f); v.Allowed {
			t.Errorf("the host's own address %s is allowed", a)
		}
	}
}
