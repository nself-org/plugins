package exec

import (
	"reflect"
	"sort"
	"testing"
)

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestFoldEnvPrefix(t *testing.T) {
	base := []string{"HOME=/h", "GIT_DIR=/x", "GIT_WORK_TREE=/y", "KEEP=1"}
	cases := []struct {
		name     string
		argv     []string
		wantArgv []string
		wantEnv  []string
	}{
		{"scrub and set", []string{"env", "-u", "GIT_DIR", "-u", "GIT_WORK_TREE", "GIT_TERMINAL_PROMPT=0", "git", "status"},
			[]string{"git", "status"}, []string{"HOME=/h", "KEEP=1", "GIT_TERMINAL_PROMPT=0"}},
		{"set replaces", []string{"env", "KEEP=2", "git", "x"}, []string{"git", "x"}, []string{"HOME=/h", "GIT_DIR=/x", "GIT_WORK_TREE=/y", "KEEP=2"}},
		{"order wins", []string{"env", "KEEP=2", "-u", "KEEP", "git"}, []string{"git"}, []string{"HOME=/h", "GIT_DIR=/x", "GIT_WORK_TREE=/y"}},
		{"unset an absent name", []string{"env", "-u", "NOPE", "git"}, []string{"git"}, base},
		{"not env", []string{"git", "status"}, []string{"git", "status"}, base},
		{"other option", []string{"env", "-i", "git"}, []string{"env", "-i", "git"}, base},
		{"no command", []string{"env", "A=1"}, []string{"env", "A=1"}, base},
		{"command is an option", []string{"env", "A=1", "--version"}, []string{"env", "A=1", "--version"}, base},
		{"path change", []string{"env", "PATH=/nowhere", "git"}, []string{"env", "PATH=/nowhere", "git"}, base},
		{"missing command", []string{"env", "-u", "GIT_DIR", "no-such-tool-xyz"}, []string{"env", "-u", "GIT_DIR", "no-such-tool-xyz"}, base},
		{"bad name", []string{"env", "-u", "A=B", "git"}, []string{"env", "-u", "A=B", "git"}, base},
		{"dangling -u", []string{"env", "git", "-u"}, []string{"git", "-u"}, base},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := append([]string(nil), base...)
			gotArgv, gotEnv := foldEnvPrefix(c.argv, input)
			if !reflect.DeepEqual(gotArgv, c.wantArgv) {
				t.Fatalf("argv = %q, want %q", gotArgv, c.wantArgv)
			}
			if !reflect.DeepEqual(sorted(gotEnv), sorted(c.wantEnv)) {
				t.Fatalf("env = %q, want %q", gotEnv, c.wantEnv)
			}
			if !reflect.DeepEqual(input, base) {
				t.Fatalf("the caller's environment was modified: %q", input)
			}
		})
	}
}
