package discovery

// Purpose: the `--advertise` flag of `nself-ci-agent join`, which cmd/agent/join.go does not declare
//          (join.go is outside this Ticket's write scope), so cmd/agent/advertise.go removes the flag from
//          argv before join parses it and starts the advertisement from a PreStart hook.
// Inputs:  argv after the word join.
// Outputs: argv without the flag, and whether it asked for an advertisement.
// Constraints: only --advertise, -advertise and their =<bool> forms (strconv.ParseBool) count; the
//          flag stops at a literal "--". Debt: when join.go can declare the flag itself, delete this file's
//          caller and use the flag value directly.

import (
	"strconv"
	"strings"
)

// StripAdvertiseFlag removes every advertise flag from args and reports whether the last one asked for
// advertising. Any other argument is returned untouched and in order.
func StripAdvertiseFlag(args []string) (rest []string, advertise bool) {
	rest = make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			return append(rest, args[i:]...), advertise
		}
		if a == "--advertise" || a == "-advertise" { // exact, like the flag package
			advertise = true
			continue
		}
		v, ok := strings.CutPrefix(a, "--advertise=")
		if !ok {
			v, ok = strings.CutPrefix(a, "-advertise=")
		}
		if ok {
			if b, err := strconv.ParseBool(v); err == nil { // an unparsable value stays, so join refuses it
				advertise = b
				continue
			}
		}
		rest = append(rest, a)
	}
	return rest, advertise
}
