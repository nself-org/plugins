package readiness

// Purpose: readiness.yield_to. When a process the operator named is running (a game, a
// video call, a build of their own), this host steps aside.
// Inputs: the configured names and the running process names.
// Outputs: the configured names that matched, sorted and de-duplicated.
// Constraints: exact, case-sensitive match on the process name. Linux truncates a
// process name to 15 characters and macOS `ps -c` to 16, so a longer configured name
// matches either prefix. Readiness gates admission only: nothing here pauses or kills a running job.

import (
	"sort"
	"strings"
)

// The longest process names the kernels report: Linux /proc/<pid>/comm, macOS MAXCOMLEN.
const (
	linuxCommLen  = 15
	darwinCommLen = 16
)

// MatchYield returns the names in want that appear in running.
func MatchYield(want, running []string) []string {
	have := make(map[string]bool, len(running))
	for _, name := range running {
		have[name] = true
	}
	seen := map[string]bool{}
	var hit []string
	for _, name := range want {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if have[name] || len(name) > linuxCommLen && have[name[:linuxCommLen]] || len(name) > darwinCommLen && have[name[:darwinCommLen]] {
			seen[name] = true
			hit = append(hit, name)
		}
	}
	sort.Strings(hit)
	return hit
}
