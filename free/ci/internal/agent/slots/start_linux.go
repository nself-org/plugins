package slots

import (
	"os"
	"strconv"
	"strings"
)

// startTime is the process start in clock ticks since boot (field 22 of
// /proc/<pid>/stat); "" when it cannot be read.
func startTime(pid int) string {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	s := string(raw)
	i := strings.LastIndexByte(s, ')') // the command name may hold spaces and parentheses
	if i < 0 {
		return ""
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
