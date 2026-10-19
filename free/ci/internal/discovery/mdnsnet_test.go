package discovery

import (
	"context"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/mdns"
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

func TestFromLibraryMapsEntries(t *testing.T) {
	e := fromLibrary(&mdns.ServiceEntry{Name: "n._nself-ci._tcp.local.", Host: "h.local.", AddrV4: net.ParseIP("192.168.1.9"), Port: 22, InfoFields: []string{"0a1b2c3d"}})
	if e.Name != "n._nself-ci._tcp.local." || len(e.Addrs) != 1 || e.Addrs[0] != netip.MustParseAddr("192.168.1.9") || e.TXT[0] != "0a1b2c3d" {
		t.Fatalf("%+v", e)
	}
}

func TestPickLocalKeepsOnlyUsableAddresses(t *testing.T) {
	mk := func(s string) net.Addr {
		_, n, _ := net.ParseCIDR(s)
		ip, _, _ := net.ParseCIDR(s)
		n.IP = ip
		return n
	}
	got := pickLocal([]net.Addr{mk("127.0.0.1/8"), mk("::1/128"), mk("fe80::1/64"), mk("169.254.1.1/16"), mk("192.168.1.5/24"), mk("fd00::5/64")})
	if len(got) != 2 || got[0] != netip.MustParseAddr("192.168.1.5") || got[1] != netip.MustParseAddr("fd00::5") {
		t.Fatalf("got %v", got)
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
	cs, err := g.Browse(context.Background(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.WindowID == a.WindowID && c.Instance == a.Instance && c.State == StateDiscovered && !c.Authorized {
			return
		}
	}
	t.Fatalf("our own advertisement %s was not listed: %+v", a.WindowID, cs)
}
