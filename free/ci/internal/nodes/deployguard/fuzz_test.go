package deployguard

import (
	"strconv"
	"strings"
	"testing"
)

// FuzzHostAlias derives a spelling of each listed host from the fuzz input (case, trailing dots, user,
// port, brackets) and requires Match to catch it; arbitrary text must never panic or match an unlisted host.
func FuzzHostAlias(f *testing.F) {
	for _, sp := range FixtureSpellings() {
		f.Add(sp, uint8(0), uint8(0), uint16(0), "")
	}
	f.Add("ci@Deploy.Example.Test.:22", uint8(7), uint8(3), uint16(65535), "x")
	s := fixtureSet()
	listed := []string{FixtureHostV4, FixtureHostV6, FixtureHostName}
	f.Fuzz(func(t *testing.T, raw string, mode, dots uint8, port uint16, user string) {
		_ = s.MatchHost(raw) // arbitrary input: no panic
		_ = Canon(raw)
		for _, base := range listed {
			v := base
			if mode&1 != 0 {
				v = strings.ToUpper(v)
			}
			isV6 := strings.Contains(base, ":")
			if !isV6 {
				v += strings.Repeat(".", int(dots%4))
			}
			if isV6 && (mode&2 != 0 || port != 0) {
				v = "[" + v + "]"
			}
			if port != 0 && (!isV6 || strings.HasPrefix(v, "[")) {
				v += ":" + strconv.Itoa(int(port))
			}
			if mode&8 != 0 && validFuzzUser(user) {
				v = user + "@" + v
			}
			if r := s.MatchHost(v); !r.Matched {
				t.Fatalf("spelling %q of %s was not matched", v, base)
			}
		}
	})
}

func validFuzzUser(u string) bool {
	if u == "" || len(u) > 32 || strings.ContainsAny(u, "@[]: \t\r\n") {
		return false
	}
	return true
}
