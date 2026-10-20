//go:build security

package redteam

// Purpose: red team group C, deploy-host exclusion (TM-41). The attacker registers a CI node whose
// address or name is a deploy host written in a spelling the never-list reader might not
// canonicalise: URL form, integer, hex, octal and short IPv4, IPv4-mapped IPv6, zone ids, brackets,
// ports, user@, case, trailing dots, whitespace, path suffixes. Every spelling must still match.
// Inputs:  a deployguard.Set built from inventory and never-list entries.
// Outputs: pass (every variant matched) or fail (a variant slipped past the exclusion).
// Constraints: pure; no DNS, no ssh, no files.

import (
	"net/netip"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
)

func cDeploySet() deployguard.Set {
	return deployguard.NewSet([]deployguard.Entry{
		{Kind: deployguard.KindHost, Value: "prod.example.com", Source: deployguard.SourceInventory},
		{Kind: deployguard.KindAddress, Value: "192.168.1.10", Source: deployguard.SourceInventory},
		{Kind: deployguard.KindPrefix, Value: "10.0.0.0/8", Source: deployguard.SourceNeverList},
		{Kind: deployguard.KindAddress, Value: "2001:db8::5", Source: deployguard.SourceNeverList},
		{Kind: deployguard.KindHost, Value: "Deploy-Box.Local.", Source: deployguard.SourceNeverList},
	})
}

// TestC41SpellingsOfADeployHost: each way of writing a listed host or address is one match.
func TestC41SpellingsOfADeployHost(t *testing.T) {
	s := cDeploySet()
	if !s.MatchHost("prod.example.com").Matched {
		t.Fatal("control: the listed host did not match")
	}
	variants := []string{
		// host name
		"PROD.EXAMPLE.COM", "prod.example.com.", "prod.example.com..", " prod.example.com ", "prod.example.com:22", "deploy@prod.example.com", "deploy@prod.example.com:2222",
		"[prod.example.com]:22", "root@PROD.Example.com.", "\tprod.example.com\n", "deploy-box.local", "DEPLOY-BOX.LOCAL:22", "user:pw@prod.example.com",
		// 192.168.1.10
		"192.168.1.10", "192.168.1.10.", "192.168.1.10:22", "[192.168.1.10]:22", "u@192.168.1.10", "3232235786", "0xC0A8010A", "0xc0.0xa8.0x1.0xa", "0300.0250.01.012", "192.168.266", "192.11010314",
		"::ffff:192.168.1.10", "::ffff:c0a8:010a", "[::ffff:192.168.1.10]:22", "0:0:0:0:0:ffff:c0a8:10a", "::FFFF:192.168.1.10", "::ffff:192.168.1.10%eth0", "192.168.1.10%eth0",
		// inside 10.0.0.0/8
		"10.1.2.3", "10.1.2.3:22", "167838211", "0x0a010203", "012.1.2.3", "10.66051", "::ffff:10.1.2.3", "[::ffff:a01:203]:22", "10.1.2.3.",
		// 2001:db8::5
		"2001:db8::5", "2001:DB8::5", "[2001:db8::5]:22", "2001:db8:0:0:0:0:0:5", "2001:0db8:0000:0000:0000:0000:0000:0005", "2001:db8::5%en0", "u@2001:db8::5",
	}
	for _, v := range variants {
		if !s.MatchHost(v).Matched {
			t.Errorf("deploy host spelling %q was not excluded", v)
		}
		if !s.Match(deployguard.Facts{Names: []string{"harmless.example.org", v}}).Matched {
			t.Errorf("deploy host spelling %q among other names was not excluded", v)
		}
	}
	for _, a := range []string{"192.168.1.10", "::ffff:192.168.1.10", "10.200.200.200", "::ffff:10.0.0.1", "2001:db8::5", "fe80::1"} {
		addr := netip.MustParseAddr(a)
		want := a != "fe80::1"
		if got := s.Match(deployguard.Facts{Addrs: []netip.Addr{addr}}).Matched; got != want {
			t.Errorf("address %s matched=%v, want %v", a, got, want)
		}
		if a == "192.168.1.10" || a == "2001:db8::5" {
			z := addr.WithZone("en0")
			if !s.Match(deployguard.Facts{Addrs: []netip.Addr{z}}).Matched {
				t.Errorf("zoned address %s was not excluded", z)
			}
		}
	}
}

// TestC41NoFalseExemption: a CI node with a partial lookup never clears a match, and neighbours of the
// listed addresses are not excluded (the exclusion does not widen into noise that would be turned off).
func TestC41NoFalseExemption(t *testing.T) {
	s := cDeploySet()
	for _, ok := range []string{"192.168.1.11", "192.168.1.1", "11.0.0.1", "prod.example.org", "xprod.example.com", "prod.example.com.evil.net", "2001:db8::6"} {
		if s.MatchHost(ok).Matched {
			t.Errorf("%q was excluded", ok)
		}
	}
	partial := s.Match(deployguard.Facts{Names: []string{"unknown-alias"}, Partial: true})
	if partial.Matched || !partial.Partial {
		t.Errorf("a partial lookup must stay unmatched and flagged partial: %+v", partial)
	}
}
