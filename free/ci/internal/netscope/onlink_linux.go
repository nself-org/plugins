//go:build linux

package netscope

// Purpose: read the IPv4 and IPv6 route tables from /proc (no netlink, no exec, no root).
// Inputs:  /proc/net/route and /proc/net/ipv6_route. Outputs: the destination prefix of every route that is
// not a default route. Constraints: a missing file is fine (IPv6 may be off); an unreadable line is an error.

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

func routePrefixes() ([]netip.Prefix, error) {
	v4, err := readRoutes("/proc/net/route", parseRoute4)
	if err != nil {
		return nil, err
	}
	v6, err := readRoutes("/proc/net/ipv6_route", parseRoute6)
	if err != nil {
		return nil, err
	}
	return append(v4, v6...), nil
}

func readRoutes(path string, parse func(string) (netip.Prefix, bool, error)) ([]netip.Prefix, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []netip.Prefix
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first && strings.HasPrefix(line, "Iface") {
			first = false
			continue
		}
		first = false
		p, ok, err := parse(line)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if ok {
			out = append(out, p)
		}
	}
	return out, sc.Err()
}

// parseRoute4: Iface Destination Gateway Flags RefCnt Use Metric Mask ... (hex, little-endian words).
func parseRoute4(line string) (netip.Prefix, bool, error) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return netip.Prefix{}, false, nil
	}
	if len(f) < 8 {
		return netip.Prefix{}, false, fmt.Errorf("short route line %q", line)
	}
	dst, err1 := strconv.ParseUint(f[1], 16, 32)
	mask, err2 := strconv.ParseUint(f[7], 16, 32)
	if err1 != nil || err2 != nil {
		return netip.Prefix{}, false, fmt.Errorf("bad route line %q", line)
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(dst))
	bits := 0
	for m := uint32(mask); m != 0; m &= m - 1 {
		bits++
	}
	p := netip.PrefixFrom(netip.AddrFrom4(b), bits)
	return p.Masked(), bits > 0, nil
}

// parseRoute6: dest(32 hex) destlen(hex) src srclen nexthop metric refcnt use flags iface.
func parseRoute6(line string) (netip.Prefix, bool, error) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return netip.Prefix{}, false, nil
	}
	if len(f) < 2 || len(f[0]) != 32 {
		return netip.Prefix{}, false, fmt.Errorf("bad route line %q", line)
	}
	raw, err := hex.DecodeString(f[0])
	bits, err2 := strconv.ParseUint(f[1], 16, 8)
	if err != nil || err2 != nil || bits > 128 {
		return netip.Prefix{}, false, fmt.Errorf("bad route line %q", line)
	}
	var b [16]byte
	copy(b[:], raw)
	p := netip.PrefixFrom(netip.AddrFrom16(b), int(bits))
	return p.Masked(), bits > 0, nil
}
