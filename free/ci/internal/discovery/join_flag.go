package discovery

// Purpose: the `--advertise` flag of `nself-ci-agent join`, which cmd/agent/join.go does not declare
//          (join.go is outside this Ticket's write scope), so cmd/agent/advertise.go removes the flag from
//          argv before join parses it and starts the advertisement from a PreStart hook.
// Inputs:  argv after the word join.
// Outputs: argv without the flag, and whether it asked for an advertisement.
// Constraints: only the exact spellings --advertise, -advertise and their =true/=false forms count; the
//          flag stops at a literal "--". Debt: when join.go can declare the flag itself, delete this file's
//          caller and use the flag value directly.

import "strings"

// StripAdvertiseFlag removes every advertise flag from args and reports whether the last one asked for
// advertising. Any other argument is returned untouched and in order.
func StripAdvertiseFlag(args []string) (rest []string, advertise bool) {
	rest = make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			return append(rest, args[i:]...), advertise
		}
		switch strings.ToLower(a) {
		case "--advertise", "-advertise", "--advertise=true", "-advertise=true":
			advertise = true
		case "--advertise=false", "-advertise=false":
			advertise = false
		default:
			rest = append(rest, a)
		}
	}
	return rest, advertise
}
