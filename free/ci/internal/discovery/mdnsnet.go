package discovery

// Purpose: the production Net: Advertise runs the hashicorp/mdns v1.0.7 server (MIT, vendored by P7-NODE-28);
//          Browse is mdnsbrowse.go (miekg/dns). This is the ONLY file that imports hashicorp/mdns;
//          TestOnlyMDNSNetImportsTheLibrary fails the build if another does.
// Inputs:  calls from the Guard, after Allowed() passed.
// Outputs: raw Entries from a browse; a running mDNS server for an advertisement.
// Constraints: the library's browse keyed records by instance name, so two responders that claimed one name
//          merged (D-1267); Browse no longer uses it. The library logs through a discarding logger; the
//          service is built with an explicit host name and explicit addresses, because the library
//          otherwise calls os.Hostname() and resolves it (leaking the real host name into SRV and A records).

import (
	"context"
	"io"
	"log"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
)

type systemNet struct{}

var quiet = log.New(io.Discard, "", 0)

// browseDest is where Browse sends its query; a var so tests can aim it at a loopback socket.
var browseDest = mdnsGroup

func (systemNet) Browse(ctx context.Context, serviceType string, timeout time.Duration) ([]Entry, error) {
	// mdnsbrowse.go reads the answers with their source, so two responders under one name stay two entries.
	raw, err := queryService(ctx, browseDest, serviceType, timeout)
	if err != nil && len(raw) == 0 && ctx.Err() == nil {
		return nil, err
	}
	return raw, nil
}

// zoneFor builds the library service from svc with every name explicit.
func zoneFor(svc Service) (*mdns.MDNSService, error) {
	ips := make([]net.IP, 0, len(svc.IPs))
	for _, a := range svc.IPs {
		ips = append(ips, net.IP(a.AsSlice()))
	}
	return mdns.NewMDNSService(svc.Instance, svc.Type, svc.Domain, svc.HostName, svc.Port, ips, svc.TXT)
}

func (systemNet) Advertise(svc Service) (func(), error) {
	zone, err := zoneFor(svc)
	if err != nil {
		return nil, err
	}
	srv, err := mdns.NewServer(&mdns.Config{Zone: zone, Logger: quiet})
	if err != nil {
		return nil, err
	}
	return func() { _ = srv.Shutdown() }, nil
}

// LocalAddrs returns the one private IPv4 address to publish: the first on a multicast-capable real
// interface. One, because the library keeps only the last A record of a name, so more would be sent to
// the LAN for nothing. Private only, because the server socket is not limited to the local link and a
// public address must not be handed to whoever queries it. IPv6 is left out on purpose: a SLAAC
// address can embed the MAC address and stays the same across windows. Virtual interfaces are skipped.
func (systemNet) LocalAddrs() []netip.Addr {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifs {
		if !publishable(ifc.Name, ifc.Flags) {
			continue
		}
		as, err := ifc.Addrs()
		if err != nil {
			continue
		}
		if got := pickLocal(as); len(got) > 0 {
			return got[:1]
		}
	}
	return nil
}

var virtualPrefixes = []string{"docker", "br-", "veth", "utun", "tun", "tap", "vmnet", "vboxnet", "virbr", "bridge", "awdl", "llw", "ap", "gif", "stf", "wg", "tailscale", "zt", "cni", "podman", "flannel", "cali", "lxc", "lxd", "incus", "nordlynx", "wt"}

// publishable keeps up, multicast-capable, real interfaces.
func publishable(name string, flags net.Flags) bool {
	if flags&net.FlagUp == 0 || flags&net.FlagMulticast == 0 || flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
		return false
	}
	n := strings.ToLower(name)
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(n, p) {
			return false
		}
	}
	return true
}

// pickLocal keeps the private IPv4 addresses of as.
func pickLocal(as []net.Addr) []netip.Addr {
	var out []netip.Addr
	for _, a := range as {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(ipn.IP)
		if !ok {
			continue
		}
		if ip = ip.Unmap(); ip.Is4() && ip.IsPrivate() {
			out = append(out, ip)
		}
	}
	return out
}
