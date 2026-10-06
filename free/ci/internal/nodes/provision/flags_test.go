package provision

// Purpose: the flag parser reproduces the pflag behaviour core relied on,
//   including its error text. Core runs against the same argv in
//   scripts/parity-nodes.sh; these are the fast offline cases.

import (
	"strings"
	"testing"
)

func newTestSet() (*flagSet, map[string]*flagDef) {
	fs := newFlagSet()
	m := map[string]*flagDef{
		"host": fs.add("host", kindSlice), "key": fs.add("ssh-key", kindString),
		"n": fs.add("instances", kindInt), "json": fs.add("json", kindBool),
	}
	m["n"].num = 1
	return fs, m
}

func TestFlags_ValuesAndForms(t *testing.T) {
	fs, m := newTestSet()
	err := fs.parse([]string{"pos", "--host", "a,b", "--host=c", "--ssh-key=/k", "--instances", "0x3", "--json", "tail", "--", "--host", "z"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m["host"].list, "|"); got != "a|b|c" {
		t.Errorf("host = %q", got)
	}
	if m["key"].str != "/k" || m["n"].num != 3 || !m["json"].on {
		t.Errorf("key=%q n=%d json=%v", m["key"].str, m["n"].num, m["json"].on)
	}
}

func TestFlags_QuotedCSVAndEmpty(t *testing.T) {
	fs, m := newTestSet()
	if err := fs.parse([]string{"--host", `"a,b",c`}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m["host"].list, "|"); got != "a,b|c" {
		t.Errorf("host = %q", got)
	}
	fs, m = newTestSet()
	if err := fs.parse([]string{"--host", ""}); err != nil || len(m["host"].list) != 0 || !m["host"].changed {
		t.Errorf("empty value: %v %v", m["host"].list, err)
	}
}

func TestFlags_ErrorsMatchPflag(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--bogus"}, "unknown flag: --bogus"},
		{[]string{"--bogus=1"}, "unknown flag: --bogus"},
		{[]string{"--instances", "x"}, `invalid argument "x" for "--instances" flag: strconv.ParseInt: parsing "x": invalid syntax`},
		{[]string{"--json=maybe"}, `invalid argument "maybe" for "--json" flag: strconv.ParseBool: parsing "maybe": invalid syntax`},
		{[]string{"--host"}, "flag needs an argument: --host"},
		{[]string{"---x"}, "bad flag syntax: ---x"},
		{[]string{"--=x"}, "bad flag syntax: --=x"},
		{[]string{"-x"}, "unknown shorthand flag: 'x' in -x"},
		{[]string{"-hx"}, "unknown shorthand flag: 'x' in -x"},
		{[]string{"--help", "--bogus"}, "unknown flag: --bogus"},
	}
	for _, tc := range cases {
		fs, _ := newTestSet()
		if err := fs.parse(tc.args); err == nil || err.Error() != tc.want {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestFlags_HelpAndTerminator(t *testing.T) {
	for args, want := range map[string]bool{"-h": true, "--help": true, "--help=false": false} {
		fs, _ := newTestSet()
		if err := fs.parse([]string{args}); err != nil || fs.help.on != want {
			t.Errorf("%s: help=%v err=%v", args, fs.help.on, err)
		}
	}
	fs, _ := newTestSet()
	if err := fs.parse([]string{"--", "--help", "--bogus"}); err != nil || fs.help.on {
		t.Errorf("after --: help=%v err=%v", fs.help.on, err)
	}
}
