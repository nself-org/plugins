package netscope

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"
)

func TestWorldFactsSharedContract(t *testing.T) {
	host := func() (Facts, error) { return Facts{HostAddrs: []netip.Addr{netip.MustParseAddr("192.0.2.1")}}, nil }
	excl := func(context.Context) (Exclusions, error) {
		return Exclusions{Hosts: []string{"::ffff:192.0.2.2"}, Prefixes: []string{"198.51.100.0/24"}}, nil
	}
	got, err := WorldFacts(context.Background(), host, excl, nil)
	want := Facts{HostAddrs: []netip.Addr{netip.MustParseAddr("192.0.2.1")}, Exclusion: []netip.Prefix{netip.MustParsePrefix("192.0.2.2/32"), netip.MustParsePrefix("198.51.100.0/24")}, ExclusionHosts: []string{"::ffff:192.0.2.2"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("facts=%+v err=%v", got, err)
	}
	sentinel := errors.New("host unavailable")
	_, err = WorldFacts(context.Background(), func() (Facts, error) { return Facts{}, sentinel }, nil, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("host error: %v", err)
	}
}
