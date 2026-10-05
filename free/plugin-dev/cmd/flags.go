package main

// Purpose: a small argv parser that reproduces the cobra/pflag behaviour the
// core `nself plugin <author command>` commands show: flag forms, error
// wording, `--help`, the exact-one-argument check, and the deprecated-alias
// notice. The binary has no third-party dependencies, so cobra is not linked;
// scripts/parity-with-core.sh proves the two agree on a scripted matrix.
// Inputs:  a command spec and the argv that follows the command name.
// Outputs: parsed flag values and positional args, or an error worded as pflag
// words it ("unknown flag: --x", "flag needs an argument: --port", ...).
// Constraints: only long flags plus the -h shorthand exist (core defines no
// other); flags may appear before or after positionals; "--" ends flags.

import (
	"fmt"
	"strconv"
	"strings"
)

// kind is a flag value type.
type kind int

const (
	kString kind = iota
	kInt
	kBool
)

// flagSpec declares one flag and its default (the text pflag would print).
type flagSpec struct {
	name string
	kind kind
	def  string
}

// globalFlags are the persistent flags the core root defines. They are
// accepted and ignored here: the nself root owns them (and refuses --json on
// commands that cannot produce JSON).
var globalFlags = []flagSpec{
	{"no-deprecation-warnings", kBool, "false"},
	{"no-monorepo", kBool, "false"},
}

// parsed holds the result of parsing one command's argv.
type parsed struct {
	vals map[string]string
	args []string
	help bool
}

func (p *parsed) str(name string) string { return p.vals[name] }

func (p *parsed) boolean(name string) bool { return p.vals[name] == "true" }

func (p *parsed) integer(name string) int {
	n, _ := strconv.ParseInt(p.vals[name], 0, 64)
	return int(n)
}

// parse parses argv against the command's flags plus the global flags and -h.
func parse(specs []flagSpec, argv []string) (*parsed, error) {
	byName := map[string]flagSpec{"help": {"help", kBool, "false"}}
	for _, s := range globalFlags {
		byName[s.name] = s
	}
	for _, s := range specs {
		byName[s.name] = s
	}
	p := &parsed{vals: map[string]string{}}
	for _, s := range byName {
		p.vals[s.name] = s.def
	}
	set := func(s flagSpec, v string) error {
		switch s.kind {
		case kInt:
			if _, err := strconv.ParseInt(v, 0, 64); err != nil {
				return fmt.Errorf("invalid argument %q for %q flag: %v", v, "--"+s.name, err)
			}
		case kBool:
			if _, err := strconv.ParseBool(v); err != nil {
				return fmt.Errorf("invalid argument %q for %q flag: %v", v, "--"+s.name, err)
			}
			b, _ := strconv.ParseBool(v)
			v = strconv.FormatBool(b)
		}
		p.vals[s.name] = v
		return nil
	}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			p.args = append(p.args, argv[i+1:]...)
			i = len(argv)
		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a[2:], "=")
			s, ok := byName[name]
			if !ok || name == "" {
				return nil, fmt.Errorf("unknown flag: --%s", name)
			}
			switch {
			case hasVal:
			case s.kind == kBool:
				val = "true"
			case i+1 < len(argv):
				i++
				val = argv[i]
			default:
				return nil, fmt.Errorf("flag needs an argument: --%s", name)
			}
			if err := set(s, val); err != nil {
				return nil, err
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for _, ch := range a[1:] {
				if ch != 'h' {
					return nil, fmt.Errorf("unknown shorthand flag: %q in -%s", ch, a[1:])
				}
				p.vals["help"] = "true"
			}
		default:
			p.args = append(p.args, a)
		}
	}
	p.help = p.boolean("help")
	return p, nil
}

// exactOneArg is cobra.ExactArgs(1).
func exactOneArg(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
	}
	return nil
}
