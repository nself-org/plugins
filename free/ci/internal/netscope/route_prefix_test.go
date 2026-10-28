package netscope

import "testing"

func TestRouteDestination(t *testing.T) {
	for _, tc := range []struct {
		dst  []byte
		bits int
		want string
	}{
		{[]byte{192, 0, 2, 42}, 24, "192.0.2.0/24"},
		{[]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, 64, "2001:db8::/64"},
		{[]byte{192, 0, 2, 42}, 33, ""}, {[]byte{1}, 24, ""},
	} {
		p, err := routePrefix(tc.dst, tc.bits)
		if tc.want == "" {
			if err == nil {
				t.Fatal("accepted malformed route")
			}
			continue
		}
		if err != nil || p.String() != tc.want {
			t.Fatalf("%s %v, want %s", p, err, tc.want)
		}
	}
}
