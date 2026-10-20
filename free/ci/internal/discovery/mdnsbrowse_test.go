package discovery

import (
	"context"
	"net"
	"net/netip"
	"sort"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// rrs parses zone-file lines into records.
func rrs(t *testing.T, lines ...string) []dns.RR {
	t.Helper()
	var out []dns.RR
	for _, l := range lines {
		rr, err := dns.NewRR(l)
		if err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		out = append(out, rr)
	}
	return out
}

// response is a reply as one responder sends it: PTR in the answer, SRV, TXT and A in the extra section.
func response(t *testing.T, inst, host, ip, txt string) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.Response = true
	m.Answer = rrs(t, "_nself-ci._tcp.local. 120 IN PTR "+inst+"._nself-ci._tcp.local.")
	m.Extra = rrs(t,
		inst+"._nself-ci._tcp.local. 120 IN SRV 0 0 22 "+host,
		inst+"._nself-ci._tcp.local. 120 IN TXT \""+txt+"\"",
		host+" 120 IN A "+ip)
	return m
}

func ap(s string) netip.AddrPort { return netip.MustParseAddrPort(s) }

// TestSameNameRespondersStayApart is D-1267: two hosts that claim one instance name must come out as two
// entries, each with its own host, address and window id. The old library browse kept the last SRV, A and
// TXT it saw per name, so one of them vanished.
func TestSameNameRespondersStayApart(t *testing.T) {
	got := entriesFrom([]reply{
		{ap("192.168.1.10:5353"), response(t, "0123456789abcdef", "aaaaaaaaaaaaaaaa.local.", "192.168.1.10", "11111111")},
		{ap("192.168.1.66:5353"), response(t, "0123456789abcdef", "bbbbbbbbbbbbbbbb.local.", "192.168.1.66", "22222222")},
	})
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %+v", got)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Host < got[j].Host })
	if got[0].Host != "aaaaaaaaaaaaaaaa.local." || got[0].Addrs[0] != netip.MustParseAddr("192.168.1.10") || got[0].TXT[0] != "11111111" {
		t.Fatalf("first responder lost or mixed: %+v", got[0])
	}
	if got[1].Host != "bbbbbbbbbbbbbbbb.local." || got[1].Addrs[0] != netip.MustParseAddr("192.168.1.66") || got[1].TXT[0] != "22222222" {
		t.Fatalf("second responder lost or mixed: %+v", got[1])
	}
	if res := candidates(got); len(res.Candidates) != 2 {
		t.Fatalf("candidates must list both: %+v", res)
	}
}

// TestForeignAddressRecordCannotSwapTheListedAddress: an A record for a sniffed host name that arrives from
// another source is not attributed to the real responder.
func TestForeignAddressRecordCannotSwapTheListedAddress(t *testing.T) {
	spoof := new(dns.Msg)
	spoof.Response = true
	spoof.Extra = rrs(t, "aaaaaaaaaaaaaaaa.local. 120 IN A 192.168.1.66")
	got := entriesFrom([]reply{
		{ap("192.168.1.10:5353"), response(t, "0123456789abcdef", "aaaaaaaaaaaaaaaa.local.", "192.168.1.10", "11111111")},
		{ap("192.168.1.66:5353"), spoof},
	})
	if len(got) != 1 || len(got[0].Addrs) != 1 || got[0].Addrs[0] != netip.MustParseAddr("192.168.1.10") {
		t.Fatalf("the spoofed A record changed the listing: %+v", got)
	}
}

// TestEntriesIgnoreQueriesAndOtherServices: only responses for our service become entries.
func TestEntriesIgnoreQueriesAndOtherServices(t *testing.T) {
	q := response(t, "0123456789abcdef", "aaaaaaaaaaaaaaaa.local.", "192.168.1.10", "11111111")
	q.Response = false
	other := new(dns.Msg)
	other.Response = true
	other.Answer = rrs(t, "_googlecast._tcp.local. 120 IN PTR tv._googlecast._tcp.local.")
	other.Extra = rrs(t, "tv._googlecast._tcp.local. 120 IN SRV 0 0 8009 tv.local.")
	if got := entriesFrom([]reply{{ap("192.168.1.10:5353"), q}, {ap("192.168.1.11:5353"), other}}); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

// TestEntriesAreCappedBeforeTheyGrow: a flood of distinct instances stops at RawCap.
func TestEntriesAreCappedBeforeTheyGrow(t *testing.T) {
	var in []reply
	for i := range RawCap + 40 {
		inst := "i" + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676))
		in = append(in, reply{ap("192.168.1.10:5353"), response(t, inst, "h.local.", "192.168.1.10", "11111111")})
	}
	if got := entriesFrom(in); len(got) != RawCap {
		t.Fatalf("want %d, got %d", RawCap, len(got))
	}
}

// TestQueryServiceKeepsTwoRespondersOnTheWire runs the real UDP path against a loopback "LAN": one query
// is answered from two sockets (two responders, one name).
func TestQueryServiceKeepsTwoRespondersOnTheWire(t *testing.T) {
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skipf("no loopback udp: %v", err)
	}
	defer srv.Close()
	var senders []*net.UDPConn
	for range 2 {
		c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		senders = append(senders, c)
	}
	go func() {
		buf := make([]byte, 1500)
		_, from, err := srv.ReadFromUDP(buf)
		if err != nil {
			return
		}
		for i, c := range senders {
			ip := []string{"192.168.1.10", "192.168.1.66"}[i]
			b, _ := response(t, "0123456789abcdef", []string{"aaaaaaaaaaaaaaaa.local.", "bbbbbbbbbbbbbbbb.local."}[i], ip, []string{"11111111", "22222222"}[i]).Pack()
			_, _ = c.WriteToUDP(b, from)
		}
	}()
	got, err := queryService(context.Background(), srv.LocalAddr().(*net.UDPAddr).AddrPort(), ServiceType, 600*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 responders, got %+v", got)
	}
}

// TestQueryServiceStopsWhenTheContextEnds: a cancelled context ends the read at once, with what it has.
func TestQueryServiceStopsWhenTheContextEnds(t *testing.T) {
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skipf("no loopback udp: %v", err)
	}
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := queryService(ctx, srv.LocalAddr().(*net.UDPAddr).AddrPort(), ServiceType, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("the query ignored its context")
	}
}
