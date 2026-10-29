//go:build linux

package netscope

// Purpose: inventory non-default routes from every Linux routing table.
// Inputs: an unfiltered rtnetlink route dump. Outputs: destination prefixes.
// Constraints: no table filter (including policy-routing tables); interrupted or
// malformed dumps fail closed; default routes never deny the entire internet.

import (
	"fmt"
	"net/netip"
	"syscall"
)

func routePrefixes() ([]netip.Prefix, error) {
	raw, err := syscall.NetlinkRIB(syscall.RTM_GETROUTE, syscall.AF_UNSPEC)
	if err != nil {
		return nil, err
	}
	return parseRouteDump(raw)
}

func parseRouteDump(raw []byte) ([]netip.Prefix, error) {
	messages, err := syscall.ParseNetlinkMessage(raw)
	if err != nil {
		return nil, err
	}
	var out []netip.Prefix
	for _, msg := range messages {
		if msg.Header.Flags&0x10 != 0 {
			return nil, fmt.Errorf("route dump interrupted")
		} // NLM_F_DUMP_INTR
		if msg.Header.Type == syscall.NLMSG_ERROR {
			return nil, fmt.Errorf("route dump failed")
		}
		if msg.Header.Type != syscall.RTM_NEWROUTE {
			continue
		}
		if len(msg.Data) < syscall.SizeofRtMsg {
			return nil, fmt.Errorf("short route message")
		}
		family, bits := msg.Data[0], int(msg.Data[1])
		if family != syscall.AF_INET && family != syscall.AF_INET6 {
			continue
		}
		if bits == 0 {
			continue
		}
		attrs, err := syscall.ParseNetlinkRouteAttr(&msg)
		if err != nil {
			return nil, err
		}
		var dst []byte
		for _, attr := range attrs {
			if attr.Attr.Type == syscall.RTA_DST {
				dst = attr.Value
			}
		}
		expected := 4
		if family == syscall.AF_INET6 {
			expected = 16
		}
		if len(dst) != expected {
			return nil, fmt.Errorf("route destination has wrong length")
		}
		p, err := routePrefix(dst, bits)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
