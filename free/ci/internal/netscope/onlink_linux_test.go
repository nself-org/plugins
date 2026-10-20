//go:build linux

package netscope

import (
	"net/netip"
	"testing"
)

func TestParseRoute4(t *testing.T) {
	for line, want := range map[string]string{
		"eth0\t0000A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0":  "192.168.0.0/24",
		"docker0\t000011AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0": "172.17.0.0/16",
		"eth0\t00000000\t0100A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0":  "", // default route
	} {
		p, ok, err := parseRoute4(line)
		if err != nil || (want == "") == ok || (ok && p != netip.MustParsePrefix(want)) {
			t.Errorf("%q: %v %v %v, want %q", line, p, ok, err, want)
		}
	}
	if _, _, err := parseRoute4("eth0 1 2"); err == nil {
		t.Error("short line accepted")
	}
}

func TestParseRoute6(t *testing.T) {
	p, ok, err := parseRoute6("2a001450400100000000000000000000 40 00000000000000000000000000000000 00 00000000000000000000000000000000 00000100 00000001 00000000 00000001 eth0")
	if err != nil || !ok || p != netip.MustParsePrefix("2a00:1450:4001::/64") {
		t.Errorf("%v %v %v", p, ok, err)
	}
	if _, ok, _ := parseRoute6("00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000400 00000001 00000000 00000003 eth0"); ok {
		t.Error("the default route was kept")
	}
	if _, _, err := parseRoute6("xyz 40"); err == nil {
		t.Error("bad line accepted")
	}
}
