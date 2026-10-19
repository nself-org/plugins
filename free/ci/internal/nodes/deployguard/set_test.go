package deployguard

import (
	"net/netip"
	"testing"
)

func fixtureSet() Set {
	return NewSet([]Entry{
		{KindHost, FixtureHostV4, SourceInventory}, {KindHost, FixtureHostV6, SourceInventory},
		{KindHost, FixtureHostName, SourceInventory}, {KindFingerprint, FixtureFingerprint, SourceInventory},
		{KindPrefix, "198.51.100.0/24", SourceNeverList}, {KindMachineID, FixtureMachineID, SourceNeverList},
	})
}

func TestEverySpellingOfAListedHostMatches(t *testing.T) {
	s := fixtureSet()
	for _, sp := range FixtureSpellings() {
		if r := s.MatchHost(sp); !r.Matched || r.Source != SourceInventory {
			t.Errorf("%q: %+v", sp, r)
		}
	}
}

func TestUnlistedHostsDoNotMatch(t *testing.T) {
	s := fixtureSet()
	for _, h := range []string{FixtureOther, "ci@" + FixtureOther + ":22", "2001:db8::11", "other.example.test", "", "   ", "@", "[", "::", "203.0.113", "0x", "1.2.3.4.5"} {
		if r := s.MatchHost(h); r.Matched {
			t.Errorf("%q matched: %+v", h, r)
		}
	}
}

func TestMatchChecksEveryIdentityAndNamesTheSource(t *testing.T) {
	s := fixtureSet()
	cases := []struct {
		name string
		f    Facts
		kind string
		src  string
	}{
		{"prefix", Facts{Addrs: []netip.Addr{netip.MustParseAddr("198.51.100.77")}}, KindPrefix, SourceNeverList},
		{"mapped prefix", Facts{Addrs: []netip.Addr{netip.MustParseAddr("::ffff:198.51.100.77")}}, KindPrefix, SourceNeverList},
		{"address", Facts{Addrs: []netip.Addr{netip.MustParseAddr(FixtureHostV4)}}, KindAddress, SourceInventory},
		{"fingerprint", Facts{Fingerprints: []string{"sha256:" + FixtureFingerprint[7:] + "="}}, KindFingerprint, SourceInventory},
		{"machine id", Facts{MachineIDs: []string{"0123456789ABCDEF0123456789abcdef\n"}}, KindMachineID, SourceNeverList},
		{"name", Facts{Names: []string{"DEPLOY.example.test."}}, KindHost, SourceInventory},
	}
	for _, c := range cases {
		r := s.Match(c.f)
		if !r.Matched || r.Kind != c.kind || r.Source != c.src {
			t.Errorf("%s: %+v", c.name, r)
		}
	}
	if r := s.Match(Facts{Fingerprints: []string{"SHA256:" + "B" + FixtureFingerprint[8:]}, MachineIDs: []string{"ff"}}); r.Matched {
		t.Errorf("different fingerprint matched: %+v", r)
	}
	if r := s.Match(Facts{Fingerprints: []string{"", "junk"}, MachineIDs: []string{"", " "}}); r.Matched {
		t.Errorf("empty identities matched: %+v", r)
	}
	if r := (Set{}).Match(Facts{Names: []string{FixtureHostName}}); r.Matched || !(Set{}).Empty() {
		t.Error("empty set matched")
	}
}

func TestAddDropsGarbageAndFirstSourceWins(t *testing.T) {
	var s Set
	s.Add(Entry{KindPrefix, "not-a-cidr", SourceInventory})
	s.Add(Entry{KindFingerprint, "junk", SourceInventory})
	s.Add(Entry{KindMachineID, "  ", SourceInventory})
	s.Add(Entry{"nonsense", "x", SourceInventory})
	s.Add(Entry{KindHost, "", SourceInventory})
	if !s.Empty() {
		t.Fatalf("garbage kept: %+v", s.View())
	}
	s.Add(Entry{KindHost, FixtureHostV4, SourceNeverList})
	s.Add(Entry{KindHost, FixtureHostV4, SourceInventory})
	s.Add(Entry{KindAddress, "10.0.0.1", ""})
	if r := s.MatchHost(FixtureHostV4); r.Source != SourceNeverList {
		t.Errorf("source = %q", r.Source)
	}
	if r := s.MatchHost("10.0.0.1"); r.Source != SourceInventory {
		t.Errorf("default source = %q", r.Source)
	}
}

func TestViewIsSortedAndComplete(t *testing.T) {
	v := fixtureSet().View()
	if len(v.Addresses) != 2 || v.Addresses[0] != FixtureHostV6 || v.Addresses[1] != FixtureHostV4 {
		t.Errorf("addresses %v", v.Addresses)
	}
	if len(v.Hosts) != 1 || v.Hosts[0] != FixtureHostName || len(v.Prefixes) != 1 || v.Prefixes[0] != "198.51.100.0/24" ||
		len(v.Fingerprints) != 1 || len(v.MachineIDs) != 1 || v.MachineIDs[0] != FixtureMachineID {
		t.Errorf("view %+v", v)
	}
}

func TestCanonTable(t *testing.T) {
	for in, want := range map[string]string{
		"Example.TEST.": "example.test", "ci@example.test:22": "example.test", "[2001:DB8::1]:22": "2001:db8::1", "2001:db8::1": "2001:db8::1",
		"::ffff:1.2.3.4": "1.2.3.4", "fe80::1%eth0": "fe80::1", "1.2.3.4:22": "1.2.3.4", "16909060": "1.2.3.4", "1.2.3.4.": "1.2.3.4",
		"": "", "  ": "", "host..": "host",
	} {
		if got := Canon(in); got != want {
			t.Errorf("Canon(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"256.1.1.1", "1.2.3.4.5", "0x1ffffffff", "1..2"} {
		if _, ok := inetAton(bad); ok {
			t.Errorf("inetAton(%q) accepted", bad)
		}
	}
}

func TestNormalizeFingerprintAndMachineID(t *testing.T) {
	if NormalizeFingerprint(FixtureFingerprint) != FixtureFingerprint || NormalizeFingerprint(FixtureFingerprint[7:]) != FixtureFingerprint {
		t.Error("fingerprint forms")
	}
	for _, bad := range []string{"", "SHA256:short", "MD5:aa:bb", "SHA256:" + FixtureFingerprint[7:] + "x"} {
		if NormalizeFingerprint(bad) != "" {
			t.Errorf("%q accepted", bad)
		}
	}
	if NormalizeMachineID(" 01234567-89AB-CDEF-0123-456789ABCDEF\n") != "0123456789abcdef0123456789abcdef" || NormalizeMachineID(" ") != "" {
		t.Error("machine id")
	}
}
