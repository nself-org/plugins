package discovery

// Purpose: the production Net: hashicorp/mdns v1.0.7 (MIT, vendored by P7-NODE-28). This is the ONLY file
//          that imports the library; TestOnlyMDNSNetImportsTheLibrary fails the build if another does.
// Inputs:  calls from the Guard, after Allowed() passed.
// Outputs: raw Entries from a browse; a running mDNS server for an advertisement.
// Constraints: the library logs through a discarding logger; the service is built with an explicit host
//          name and explicit addresses, because the library otherwise calls os.Hostname() and resolves it
//          (leaking the real host name into SRV and A records); the browse channel is drained while the
//          query runs because the library drops sends to a full channel.

import (
	"context"
	"io"
	"log"
	"net"
	"net/netip"

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
			if len(got) >= 4*MaxCandidates {
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

// LocalAddrs lists up to four global-unicast (including private) addresses of this host.
func (systemNet) LocalAddrs() []netip.Addr {
	as, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	return pickLocal(as)
}

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
		if ip = ip.Unmap(); ip.IsGlobalUnicast() && !ip.IsLoopback() {
			out = append(out, ip)
		}
		if len(out) == 4 {
			break
		}
	}
	return out
}
