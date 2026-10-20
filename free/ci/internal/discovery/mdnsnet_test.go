package discovery

import (
	"context"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

// TestZoneNamesAreExplicit: the library service carries our random names and addresses, never the
// machine's host name, and builds without a lookup.
func TestZoneNamesAreExplicit(t *testing.T) {
	id, err := newIdentity(nil)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zoneFor(Service{Instance: id.Instance, Type: ServiceType, Domain: Domain, HostName: id.Host, Port: 22, IPs: addr("192.168.1.20"), TXT: []string{id.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if z.HostName != id.Host || z.Instance != id.Instance || len(z.TXT) != 1 || z.TXT[0] != id.ID || len(z.IPs) != 1 {
		t.Fatalf("service not built from the window identity: %+v", z)
	}
	host, _ := os.Hostname()
	if h := strings.ToLower(host); len(h) > 3 && strings.Contains(strings.ToLower(z.HostName+z.Instance), h) {
		t.Fatal("the library service carries the host name")
	}
	if _, err = zoneFor(Service{Instance: id.Instance, Type: ServiceType, Domain: Domain, HostName: "no-trailing-dot", Port: 22, IPs: addr("192.168.1.20")}); err == nil {
		t.Fatal("a non-FQDN host name must be refused, not defaulted to os.Hostname")
	}
}

func TestPickLocalKeepsOnlyUsableAddresses(t *testing.T) {
	mk := func(s string) net.Addr {
		_, n, _ := net.ParseCIDR(s)
		ip, _, _ := net.ParseCIDR(s)
		n.IP = ip
		return n
	}
	// IPv6 is never published (a SLAAC address can embed the MAC); loopback and link-local are not either.
	got := pickLocal([]net.Addr{mk("127.0.0.1/8"), mk("::1/128"), mk("fe80::1/64"), mk("169.254.1.1/16"), mk("192.168.1.5/24"), mk("fd00::5/64"), mk("2001:db8::211:22ff:fe33:4455/64"), mk("8.8.4.4/24")})
	if len(got) != 1 || got[0] != netip.MustParseAddr("192.168.1.5") {
		t.Fatalf("got %v", got)
	}
}

func TestPublishableSkipsVirtualInterfaces(t *testing.T) {
	up := net.FlagUp | net.FlagMulticast
	for name, want := range map[string]bool{"en0": true, "eth0": true, "wlan0": true, "docker0": false, "br-1a2b": false, "veth12": false, "utun3": false, "tailscale0": false, "awdl0": false, "cni0": false, "podman0": false, "lxdbr0": false} {
		if publishable(name, up) != want {
			t.Errorf("%s: want %v", name, want)
		}
	}
	for _, f := range []net.Flags{0, net.FlagUp, up | net.FlagLoopback, up | net.FlagPointToPoint} {
		if publishable("en0", f) {
			t.Errorf("flags %v must not be published", f)
		}
	}
}

// TestRealLoopbackRoundTrip runs the real library: one advertisement, one browse. It needs a multicast
// capable interface, so it runs only when NSELF_CI_TEST_MDNS=1.
func TestRealLoopbackRoundTrip(t *testing.T) {
	if os.Getenv("NSELF_CI_TEST_MDNS") != "1" {
		t.Skip("set NSELF_CI_TEST_MDNS=1 to use real multicast sockets")
	}
	g := &Guard{Home: t.TempDir(), Getenv: func(string) string { return "" }}
	a, err := g.Advertise(context.Background(), time.Minute, SSHPort, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	var cs Result
	for range 3 { // a multicast query can be lost; the answer is what is under test
		var err error
		if cs, err = g.Browse(context.Background(), 3*time.Second); err != nil {
			t.Fatal(err)
		}
		for _, c := range cs.Candidates {
			if c.WindowID == a.WindowID && c.Instance == a.Instance && c.State == StateDiscovered && !c.Authorized {
				return
			}
		}
	}
	t.Fatalf("our own advertisement %s was not listed: %+v", a.WindowID, cs)
}
