package rest

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"syscall"
)

// denyCIDRs covers plugin-sdk's deny set and mapped, translated, and tunneled IPs.
var denyCIDRs = []string{
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16",
	"127.0.0.0/8", "::1/128", "fc00::/7", "fe80::/10", "0.0.0.0/8",
	"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32",
	"192.0.2.0/24", "224.0.0.0/4", "::/128", "ff00::/8", "2001:db8::/32",
	"::ffff:0:0/96", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16",
}

var denied = func() []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(denyCIDRs))
	for _, cidr := range denyCIDRs {
		prefixes = append(prefixes, netip.MustParsePrefix(cidr))
	}
	return prefixes
}()

type privateBaseKey struct{}

func privateAddress(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	// Mapped IPv4 must be evaluated as IPv4, including translated forms.
	if addr.Is4In6() {
		return true
	}
	for _, prefix := range denied {
		if prefix.Contains(addr) {
			return true
		}
	}
	return !addr.IsGlobalUnicast()
}

func (c *Client) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errPrivate
	}
	allow := c.AllowPrivateBase && ctx.Value(privateBaseKey{}) == true && strings.EqualFold(host, c.baseHost())
	dialer := &net.Dialer{Timeout: connectTimeout, Resolver: c.Resolver}
	dialer.Control = func(_, ipAddress string, _ syscall.RawConn) error {
		ipHost, _, splitErr := net.SplitHostPort(ipAddress)
		if splitErr != nil {
			return errPrivate
		}
		ip, parseErr := netip.ParseAddr(ipHost)
		if parseErr != nil || (privateAddress(ip) && !allow) {
			return errPrivate
		}
		return nil
	}
	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		if errors.Is(err, errPrivate) {
			return nil, errPrivate
		}
		return nil, err
	}
	return conn, nil
}
