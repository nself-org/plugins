package provision

// Purpose: describe the flags of `nodes provision` and `nodes verify` as a standard flag.FlagSet, so the
//   machine surface test (cmd/surface_test.go) compares commands.yaml with the flags the parser really accepts.
// Inputs:  the command name, "provision" or "verify".
// Outputs: a flag set whose flags carry the parser's names and value types; it parses nothing here.
// Constraints: built from the same definitions RunProvision and RunVerify use (provisionFlags, verifyFlags).

import (
	"flag"
	"strings"
)

// listFlag is a repeatable, comma-separated string flag; its type name is how the surface test spots a stringSlice.
type listFlag struct{ vals []string }

func (l *listFlag) String() string { return strings.Join(l.vals, ",") }

func (l *listFlag) Set(v string) error {
	l.vals = append(l.vals, strings.Split(v, ",")...)
	return nil
}

// StdFlagSet returns the flags of `nodes <name>` (name is "provision" or "verify") as a standard flag set;
// an unknown name gives an empty set.
func StdFlagSet(name string) *flag.FlagSet {
	fs := newFlagSet()
	switch name {
	case "provision":
		provisionFlags(fs)
	case "verify":
		verifyFlags(fs)
	}
	out := flag.NewFlagSet("nodes "+name, flag.ContinueOnError)
	for _, d := range fs.defs {
		switch d.kind {
		case kindInt:
			out.Int(d.name, 0, "")
		case kindBool:
			out.Bool(d.name, false, "")
		case kindSlice:
			out.Var(&listFlag{}, d.name, "")
		default:
			out.String(d.name, "", "")
		}
	}
	return out
}
