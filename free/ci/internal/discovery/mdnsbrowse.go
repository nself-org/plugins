package discovery

// Purpose: the mDNS browse, written on miekg/dns so that every record stays tied to the source that sent it.
//          hashicorp/mdns keys in-progress records by instance name alone and keeps the last SRV, A and TXT,
//          so two responders that claim one name merged into one row (debt D-1267) and a foreign A record
//          could swap the address shown for a real host.
// Inputs:  a destination (the mDNS group in production, a loopback socket in tests), a service type and a
//          timeout.
// Outputs: raw Entries, one per (source, instance name): the SRV, TXT and A/AAAA records of one claim all
//          come from one source address and port.
// Constraints: untrusted input. An address record counts only if the same source sent the SRV that names its
//          host. Only responses for our service become entries, the entry count is capped at RawCap while
//          records are read, and at most maxPackets datagrams are read. This file opens a socket only when
//          queryService is called, which only the production Net (mdnsnet.go) does, behind the Guard.
//          Follow-up queries are not sent: our own advertisement (hashicorp server) puts SRV, TXT and A in
//          the additional section of its PTR answer. The query asks for a unicast reply (QU bit), so the
//          answers arrive at this socket and not on the group.

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"
)

const (
	maxPackets = 2048
	maxMessage = 9000 // the mDNS maximum message size (RFC 6762 §17)
	quBit      = 1 << 15
)

// mdnsGroup is the IPv4 mDNS multicast group.
var mdnsGroup = netip.MustParseAddrPort("224.0.0.251:5353")

// reply is one datagram: who sent it and what it said.
type reply struct {
	from netip.AddrPort
	msg  *dns.Msg
}

// claim is what one source says about one instance name.
type claim struct {
	from  netip.AddrPort
	name  string // as sent
	host  string
	port  int
	txt   []string
	addrs map[string][]netip.Addr // lower-case host name -> addresses this source sent for it
}

// entriesFrom turns replies into entries, one per (source, instance name). Address records are matched to
// the SRV target of the claim from the same source only.
func entriesFrom(replies []reply) []Entry {
	type key struct {
		from netip.AddrPort
		name string
	}
	claims := map[key]*claim{}
	var order []*claim
	addrsBySource := map[netip.AddrPort]map[string][]netip.Addr{}
	get := func(from netip.AddrPort, name string) *claim {
		k := key{from, strings.ToLower(name)}
		if c := claims[k]; c != nil {
			return c
		}
		if len(order) >= RawCap || !ofService(name) {
			return nil
		}
		c := &claim{from: from, name: name}
		claims[k] = c
		order = append(order, c)
		return c
	}
	for _, r := range replies {
		if r.msg == nil || !r.msg.Response {
			continue
		}
		rrs := append(append(append([]dns.RR(nil), r.msg.Answer...), r.msg.Ns...), r.msg.Extra...)
		for _, rr := range rrs {
			switch v := rr.(type) {
			case *dns.PTR:
				if strings.EqualFold(v.Hdr.Name, ServiceType+"."+Domain) {
					get(r.from, v.Ptr)
				}
			case *dns.SRV:
				if c := get(r.from, v.Hdr.Name); c != nil {
					c.host, c.port = v.Target, int(v.Port)
				}
			case *dns.TXT:
				if c := get(r.from, v.Hdr.Name); c != nil {
					c.txt = append([]string(nil), v.Txt...)
				}
			case *dns.A:
				addHost(addrsBySource, r.from, v.Hdr.Name, v.A)
			case *dns.AAAA:
				addHost(addrsBySource, r.from, v.Hdr.Name, v.AAAA)
			}
		}
	}
	out := make([]Entry, 0, len(order))
	for _, c := range order {
		e := Entry{Name: c.name, Host: c.host, Port: c.port, TXT: c.txt}
		if c.host != "" {
			e.Addrs = addrsBySource[c.from][strings.ToLower(c.host)]
		}
		if len(e.Addrs) == 0 { // the responder sent no address: the source it answered from is the hint
			e.Addrs = []netip.Addr{c.from.Addr().Unmap()}
		}
		out = append(out, e)
	}
	return out
}

func addHost(m map[netip.AddrPort]map[string][]netip.Addr, from netip.AddrPort, name string, ip net.IP) {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return
	}
	if m[from] == nil {
		m[from] = map[string][]netip.Addr{}
	}
	k := strings.ToLower(name)
	m[from][k] = append(m[from][k], a.Unmap())
}

// queryService sends one PTR query for serviceType to dest and reads replies until timeout or ctx ends.
// When dest is the multicast group the query goes out on every publishable interface.
func queryService(ctx context.Context, dest netip.AddrPort, serviceType string, timeout time.Duration) ([]Entry, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	q := new(dns.Msg)
	q.SetQuestion(serviceType+"."+Domain, dns.TypePTR)
	q.RecursionDesired = false
	q.Question[0].Qclass |= quBit
	wire, err := q.Pack()
	if err != nil {
		return nil, err
	}
	if err := sendQuery(conn, dest, wire); err != nil {
		return nil, err
	}
	end := time.Now().Add(timeout)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetReadDeadline(time.Unix(1, 0)) // unblocks the read at once
		case <-stop:
		}
	}()
	_ = conn.SetReadDeadline(end)
	var replies []reply
	buf := make([]byte, maxMessage)
	for range maxPackets {
		n, from, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				break
			}
			return nil, err
		}
		msg := new(dns.Msg)
		if msg.Unpack(buf[:n]) != nil {
			continue
		}
		replies = append(replies, reply{from: netip.AddrPortFrom(from.Addr().Unmap(), from.Port()), msg: msg})
	}
	return entriesFrom(replies), nil
}

// sendQuery writes wire to dest; for the mDNS group it writes once per publishable interface that has a private IPv4 address.
func sendQuery(conn *net.UDPConn, dest netip.AddrPort, wire []byte) error {
	if !dest.Addr().IsMulticast() {
		_, err := conn.WriteToUDPAddrPort(wire, dest)
		return err
	}
	ifs, _ := net.Interfaces()
	pc := ipv4.NewPacketConn(conn)
	sent := false
	for _, ifc := range ifs {
		if !publishable(ifc.Name, ifc.Flags) || !hasPrivateV4(&ifc) {
			continue
		}
		if pc.SetMulticastInterface(&ifc) != nil {
			continue
		}
		if _, err := conn.WriteToUDPAddrPort(wire, dest); err == nil {
			sent = true
		}
	}
	if sent {
		return nil
	}
	_, err := conn.WriteToUDPAddrPort(wire, dest) // no usable interface: let the system pick one
	return err
}

// hasPrivateV4 reports whether ifc has a private IPv4 address, the only kind a LAN query can leave from.
func hasPrivateV4(ifc *net.Interface) bool {
	as, err := ifc.Addrs()
	return err == nil && len(pickLocal(as)) > 0
}
