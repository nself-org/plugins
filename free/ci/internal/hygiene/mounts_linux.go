//go:build linux

package hygiene

import (
	"bufio"
	"os"
	"strings"
)

// mountsUnder returns the mount points at or below dir that this process's mount namespace shows
// (a bind mount on the same device passes the device check but not this one); ok is false when the
// mount table cannot be read, which the caller treats as a refusal.
func mountsUnder(dir string) (found []string, ok bool) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		mp := unescapeMount(fields[4])
		if mp == dir || strings.HasPrefix(mp, dir+"/") {
			found = append(found, mp)
		}
	}
	return found, sc.Err() == nil
}

// unescapeMount decodes the octal escapes (\040 for a space) mountinfo uses.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			n := 0
			ok := true
			for _, c := range s[i+1 : i+4] {
				if c < '0' || c > '7' {
					ok = false
					break
				}
				n = n*8 + int(c-'0')
			}
			if ok {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
