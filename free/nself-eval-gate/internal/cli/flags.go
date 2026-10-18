package cli

// Purpose: a small long-flag parser that reproduces the argv behaviour of the
//   cobra/pflag command it was ported from (nself ci eval, nself ci eval gate),
//   so the plugin binary needs no cobra dependency.
// Inputs:  a flag table and argv (flags and positionals may be mixed).
// Outputs: parsed values, positionals, whether -h/--help was given, and pflag-
//   worded errors ("unknown flag: --x", "flag needs an argument: --x", ...).
// Constraints: `--name value`, `--name=value`, bool `--name` / `--name=false`,
//   `--` ends flags, `-h` is the only shorthand. Parse errors are returned in
//   argv order, before help or required-flag checks (cobra order).
// SPORT: PLUGINS-NSELF-EVAL-GATE-CLI-001

import (
	"fmt"
	"strconv"
	"strings"
)

type flagKind int

const (
	kindString flagKind = iota
	kindInt
	kindBool
)

// flagDef declares one long flag.
type flagDef struct {
	name string
	kind flagKind
	def  string
}

// globalFlags are the persistent flags the nself host owns. The host applies
// them before the plugin runs; the plugin accepts and ignores them.
var globalFlags = []flagDef{
	{"json", kindBool, "false"},
	{"no-deprecation-warnings", kindBool, "false"},
	{"no-monorepo", kindBool, "false"},
}

// parsed is the result of parseArgs.
type parsed struct {
	vals map[string]string
	set  map[string]bool
	pos  []string
	help bool
	defs map[string]flagDef
}

func (p *parsed) str(name string) string { return p.vals[name] }

func (p *parsed) boolean(name string) bool { return p.vals[name] == "true" }

func (p *parsed) integer(name string) int {
	n, _ := strconv.ParseInt(p.vals[name], 0, 64)
	return int(n)
}

func newDefs(own []flagDef) map[string]flagDef {
	m := map[string]flagDef{"help": {"help", kindBool, "false"}}
	for _, d := range append(append([]flagDef{}, globalFlags...), own...) {
		m[d.name] = d
	}
	return m
}

// parseArgs parses argv against own plus the global flags and -h/--help.
func parseArgs(own []flagDef, args []string) (*parsed, error) {
	defs := newDefs(own)
	p := &parsed{vals: map[string]string{}, set: map[string]bool{}, defs: defs}
	for n, d := range defs {
		p.vals[n] = d.def
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			p.pos = append(p.pos, args[i+1:]...)
			p.help = p.boolean("help")
			return p, nil
		case strings.HasPrefix(a, "--"):
			name, val, hasEq := strings.Cut(a[2:], "=")
			d, ok := defs[name]
			if !ok {
				return nil, fmt.Errorf("unknown flag: --%s", name)
			}
			if !hasEq {
				if d.kind == kindBool {
					val = "true"
				} else if i+1 < len(args) {
					i++
					val = args[i]
				} else {
					return nil, fmt.Errorf("flag needs an argument: --%s", name)
				}
			}
			if err := checkValue(d, val); err != nil {
				return nil, err
			}
			p.vals[name], p.set[name] = val, true
		case a == "-h":
			p.vals["help"], p.set["help"] = "true", true
		case len(a) > 1 && a[0] == '-':
			return nil, fmt.Errorf("unknown shorthand flag: %q in %s", a[1], a)
		default:
			p.pos = append(p.pos, a)
		}
	}
	p.help = p.boolean("help")
	return p, nil
}

// checkValue validates a value against the flag kind with pflag's wording.
func checkValue(d flagDef, val string) error {
	var err error
	switch d.kind {
	case kindInt:
		_, err = strconv.ParseInt(val, 0, 64)
	case kindBool:
		_, err = strconv.ParseBool(val)
	}
	if err != nil {
		return fmt.Errorf("invalid argument %q for %q flag: %v", val, "--"+d.name, err)
	}
	return nil
}

// firstPositional returns the index of the token cobra would treat as the
// subcommand name: the first non-flag token, where a flag that is not a known
// boolean swallows the next token (cobra's stripFlags, including its quirk that
// -h and --help are not yet registered at that point and so swallow it too).
func firstPositional(own []flagDef, args []string) int {
	defs := newDefs(own)
	delete(defs, "help")
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return -1
		case strings.HasPrefix(a, "--"):
			if strings.Contains(a, "=") {
				continue
			}
			if d, ok := defs[a[2:]]; !ok || d.kind != kindBool {
				i++
			}
		case strings.HasPrefix(a, "-") && len(a) == 2:
			i++
		case a != "" && !strings.HasPrefix(a, "-"):
			return i
		}
	}
	return -1
}
