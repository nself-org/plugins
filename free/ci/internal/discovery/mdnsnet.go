package discovery

// Purpose: the production Net: hashicorp/mdns v1.0.7 (MIT, vendored by P7-NODE-28). This is the ONLY file
//          that imports the library; TestOnlyMDNSNetImportsTheLibrary fails the build if another does.
// Inputs:  calls from the Guard, after Allowed() passed.
// Outputs: raw Entries from a browse; a running mDNS server for an advertisement.
// Constraints: the library keys records by instance name, so two responders that claim one name merge
//          inside it (debt: the Guard and candidates() keep them apart, the library cannot); it also does
//          not check that an answer matches the queried type (candidates() filters). The library logs
//          through a discarding logger; the service is built with an explicit host
//          name and explicit addresses, because the library otherwise calls os.Hostname() and resolves it
//          (leaking the real host name into SRV and A records); the browse channel is drained while the
//          query runs because the library drops sends to a full channel.

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

func (systemNet) Browse(ctx context.Context, serviceType string, timeout time.Duration) ([]Entry, error) {
	ch := make(chan *mdns.ServiceEntry, 64)
	done := make(chan []Entry, 1)
	go func() {
		var got []Entry
		for e := range ch {
			got = append(got, fromLibrary(e))
			if len(got) >= RawCap {
				break // a flood: stop collecting, keep draining below
			}
		}
		for range ch {
		}
		done <- got
	}()
	err := mdns.QueryContext(ctx, &mdns.QueryParam{Service: serviceType, Domain: "local", Timeout: timeout, Entries: ch, Logger: quiet})
	close(ch)
	got := <-done
	if err != nil && len(got) == 0 && ctx.Err() == nil {
		return nil, err
	}
	return got, nil
}

func fromLibrary(e *mdns.ServiceEntry) Entry {
	out := Entry{Name: e.Name, Host: e.Host, Port: e.Port, TXT: append([]string(nil), e.InfoFields...)}
	for _, ip := range []net.IP{e.AddrV4, e.AddrV6, e.Addr} {
		if a, ok := netip.AddrFromSlice(ip); ok {
			out.Addrs = append(out.Addrs, a.Unmap())
		}
	}
	return out
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

// LocalAddrs lists up to four private or global IPv4 addresses of this host's multicast-capable
// interfaces. IPv6 is left out on purpose: a SLAAC address can embed the MAC address and stays the same
// across windows, which would make the record stable and identifying. Virtual interfaces (containers,
// VPN tunnels, bridges) are skipped.
func (systemNet) LocalAddrs() []netip.Addr {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, ifc := range ifs {
		if !publishable(ifc.Name, ifc.Flags) {
			continue
		}
		as, err := ifc.Addrs()
		if err != nil {
			continue
		}
		out = append(out, pickLocal(as)...)
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

var virtualPrefixes = []string{"docker", "br-", "veth", "utun", "tun", "tap", "vmnet", "vboxnet", "virbr", "bridge", "awdl", "llw", "ap", "gif", "stf", "wg", "tailscale", "zt"}

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

// pickLocal keeps the private or global IPv4 addresses of as.
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
		if ip = ip.Unmap(); ip.Is4() && ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast() {
			out = append(out, ip)
		}
	}
	return out
}
