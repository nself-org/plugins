//go:build linux

package netscope

import (
	"encoding/binary"
	"syscall"
	"testing"
)

func routeMessage(table, bits byte, dst []byte) []byte {
	n := syscall.NLMSG_HDRLEN + syscall.SizeofRtMsg + 4 + len(dst)
	b := make([]byte, n)
	binary.NativeEndian.PutUint32(b, uint32(n))
	binary.NativeEndian.PutUint16(b[4:], syscall.RTM_NEWROUTE)
	b[syscall.NLMSG_HDRLEN] = syscall.AF_INET
	b[syscall.NLMSG_HDRLEN+1] = bits
	b[syscall.NLMSG_HDRLEN+4] = table
	at := syscall.NLMSG_HDRLEN + syscall.SizeofRtMsg
	binary.NativeEndian.PutUint16(b[at:], uint16(4+len(dst)))
	binary.NativeEndian.PutUint16(b[at+2:], syscall.RTA_DST)
	copy(b[at+4:], dst)
	return b
}

func TestRouteDumpIncludesPolicyTables(t *testing.T) {
	raw := append(routeMessage(254, 24, []byte{192, 0, 2, 0}), routeMessage(100, 24, []byte{198, 51, 100, 0})...)
	raw = append(raw, routeMessage(254, 0, []byte{0, 0, 0, 0})...)
	got, err := parseRouteDump(raw)
	if err != nil || len(got) != 2 || got[0].String() != "192.0.2.0/24" || got[1].String() != "198.51.100.0/24" {
		t.Fatalf("%v %v", got, err)
	}
	raw = routeMessage(100, 24, []byte{198, 51, 100, 0})
	binary.NativeEndian.PutUint16(raw[6:], 0x10)
	if _, err := parseRouteDump(raw); err == nil {
		t.Fatal("accepted interrupted dump")
	}
}
