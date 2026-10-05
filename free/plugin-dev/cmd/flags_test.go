package main

import "testing"

var specs = []flagSpec{{"name", kString, "d"}, {"n", kInt, "7"}, {"on", kBool, "false"}}

func TestParseForms(t *testing.T) {
	p, err := parse(specs, []string{"a", "--name", "x", "--n=0x10", "--on", "b", "--", "--c"})
	if err != nil {
		t.Fatal(err)
	}
	if p.str("name") != "x" || p.integer("n") != 16 || !p.boolean("on") {
		t.Fatalf("vals %v", p.vals)
	}
	if len(p.args) != 3 || p.args[0] != "a" || p.args[1] != "b" || p.args[2] != "--c" {
		t.Fatalf("args %v", p.args)
	}
}

func TestParseDefaultsAndBoolValue(t *testing.T) {
	p, _ := parse(specs, []string{"--on=false", "--no-monorepo"})
	if p.str("name") != "d" || p.integer("n") != 7 || p.boolean("on") || !p.boolean("no-monorepo") {
		t.Fatalf("vals %v", p.vals)
	}
}

func TestParseValueMayLookLikeFlag(t *testing.T) {
	p, err := parse(specs, []string{"--name", "--on"})
	if err != nil || p.str("name") != "--on" || p.boolean("on") {
		t.Fatalf("%v %v", p, err)
	}
}

func TestParseErrors(t *testing.T) {
	for argv, want := range map[string]string{
		"--zzz":       "unknown flag: --zzz",
		"--zzz=1":     "unknown flag: --zzz",
		"-q":          "unknown shorthand flag: 'q' in -q",
		"--name":      "flag needs an argument: --name",
		"--n=abc":     `invalid argument "abc" for "--n" flag: strconv.ParseInt: parsing "abc": invalid syntax`,
		"--on=maybe":  `invalid argument "maybe" for "--on" flag: strconv.ParseBool: parsing "maybe": invalid syntax`,
		"--json":      "unknown flag: --json",
		"--help=what": `invalid argument "what" for "--help" flag: strconv.ParseBool: parsing "what": invalid syntax`,
	} {
		_, err := parse(specs, []string{argv})
		if err == nil || err.Error() != want {
			t.Errorf("%s: got %v want %s", argv, err, want)
		}
	}
}

func TestHelpFlags(t *testing.T) {
	for _, a := range []string{"-h", "--help", "--help=true"} {
		p, err := parse(specs, []string{a})
		if err != nil || !p.help {
			t.Errorf("%s: %v %v", a, p, err)
		}
	}
}

func TestExactOneArg(t *testing.T) {
	if exactOneArg([]string{"a"}) != nil {
		t.Fatal("one arg rejected")
	}
	if err := exactOneArg(nil); err == nil || err.Error() != "accepts 1 arg(s), received 0" {
		t.Fatalf("%v", err)
	}
	if err := exactOneArg([]string{"a", "b"}); err == nil || err.Error() != "accepts 1 arg(s), received 2" {
		t.Fatalf("%v", err)
	}
}
