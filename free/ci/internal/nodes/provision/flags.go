package provision

// Purpose: the flag parser for `nodes provision` and `nodes verify`. Core parses
//   these commands with cobra and pflag; this file reproduces the pflag rules
//   the two commands used (interspersed positionals, --flag value and
//   --flag=value, repeatable CSV string slices, base-0 ints, bool values,
//   "--" terminator, the -h shorthand) with pflag's error text, so a typo or
//   a bad value gives the same message and exit code as core.
// Inputs:  argv after the subcommand key and the flag definitions.
// Outputs: parsed values, a help request, or an error whose text is pflag's.
// Constraints: no dependency beyond the standard library. Positional
//   arguments are collected and ignored, as the core commands ignore them.

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// flagKind is the value type of a flag.
type flagKind int

const (
	kindString flagKind = iota
	kindInt
	kindBool
	kindSlice
)

// flagDef is one flag and its parsed value.
type flagDef struct {
	name    string
	kind    flagKind
	str     string
	num     int
	on      bool
	list    []string
	changed bool
}

// flagSet is the set of flags one command accepts, plus its parse result.
type flagSet struct {
	defs []*flagDef
	help *flagDef
}

// newFlagSet returns a set that already holds the help flag.
func newFlagSet() *flagSet {
	fs := &flagSet{}
	fs.help = fs.add("help", kindBool)
	return fs
}

func (fs *flagSet) add(name string, kind flagKind) *flagDef {
	d := &flagDef{name: name, kind: kind}
	fs.defs = append(fs.defs, d)
	return d
}

func (fs *flagSet) lookup(name string) *flagDef {
	for _, d := range fs.defs {
		if d.name == name {
			return d
		}
	}
	return nil
}

// set applies a value to a flag with pflag's invalid-argument text.
func (d *flagDef) set(val string) error {
	bad := func(err error) error {
		return fmt.Errorf("invalid argument %q for %q flag: %v", val, "--"+d.name, err)
	}
	switch d.kind {
	case kindString:
		d.str = val
	case kindInt:
		n, err := strconv.ParseInt(val, 0, 0)
		if err != nil {
			return bad(err)
		}
		d.num = int(n)
	case kindBool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return bad(err)
		}
		d.on = b
	case kindSlice:
		var vals []string
		if val != "" {
			r, err := csv.NewReader(strings.NewReader(val)).Read()
			if err != nil {
				return bad(err)
			}
			vals = r
		}
		if !d.changed {
			d.list = vals
		} else {
			d.list = append(d.list, vals...)
		}
	}
	d.changed = true
	return nil
}

// parse reads args. A -h or --help sets fs.help.on (the first parse error still
// wins, as in pflag). Positional arguments are accepted and dropped.
func (fs *flagSet) parse(args []string) error {
	for len(args) > 0 {
		a := args[0]
		args = args[1:]
		if len(a) == 0 || a[0] != '-' || len(a) == 1 {
			continue
		}
		if a[1] == '-' {
			if len(a) == 2 { // "--": the rest is positional
				return nil
			}
			var err error
			if args, err = fs.parseLong(a[2:], args); err != nil {
				return err
			}
			continue
		}
		if err := fs.parseShort(a[1:]); err != nil {
			return err
		}
	}
	return nil
}

func (fs *flagSet) parseLong(s string, rest []string) ([]string, error) {
	if len(s) == 0 || s[0] == '-' || s[0] == '=' {
		return rest, fmt.Errorf("bad flag syntax: %s", "--"+s)
	}
	name, val, hasVal := strings.Cut(s, "=")
	d := fs.lookup(name)
	if d == nil {
		return rest, fmt.Errorf("unknown flag: --%s", name)
	}
	switch {
	case hasVal:
	case d.kind == kindBool:
		val = "true"
	case len(rest) > 0:
		val, rest = rest[0], rest[1:]
	default:
		return rest, fmt.Errorf("flag needs an argument: --%s", name)
	}
	return rest, d.set(val)
}

// parseShort handles a cluster of shorthands; only -h exists.
func (fs *flagSet) parseShort(s string) error {
	for len(s) > 0 {
		if s[0] != 'h' {
			return fmt.Errorf("unknown shorthand flag: %q in -%s", s[0], s)
		}
		s = s[1:]
		if len(s) > 1 && s[0] == '=' {
			b, err := strconv.ParseBool(s[1:])
			if err != nil {
				return fmt.Errorf("invalid argument %q for %q flag: %v", s[1:], "-h", err)
			}
			fs.help.on = b
			return nil
		}
		fs.help.on = true
	}
	return nil
}
