package exec

// Purpose: run `env [-u NAME]... [NAME=VALUE]... cmd args` as `cmd args` with the
// same environment, without starting env. The git callers scrub GIT_DIR and
// friends this way; every call paid an extra exec (and a PATH search) for it.
// Inputs:  the command and the environment the job would start with.
// Outputs: the command without the env prefix and the environment env would
//          have built, or both unchanged when the prefix is anything else.
// Constraints: only exact `env` followed by -u NAME and NAME=VALUE words is
// folded; any other option, a PATH change, a command that is not on PATH or a
// missing command keeps the original argv, so the result is what env would do.

import (
	osexec "os/exec"
	"strings"
)

// foldEnvPrefix removes a leading env wrapper from argv and applies its -u and
// NAME=VALUE words, in order, to a copy of env.
func foldEnvPrefix(argv, env []string) ([]string, []string) {
	if len(argv) < 2 || argv[0] != "env" {
		return argv, env
	}
	type op struct {
		name, value string
		unset       bool
	}
	var ops []op
	i := 1
parse:
	for i < len(argv) {
		w := argv[i]
		switch {
		case w == "-u" && i+1 < len(argv):
			name := argv[i+1]
			if !envName(name) {
				return argv, env
			}
			ops = append(ops, op{name: name, unset: true})
			i += 2
		case !strings.HasPrefix(w, "-") && strings.Contains(w, "="):
			name, value, _ := strings.Cut(w, "=")
			if !envName(name) {
				return argv, env
			}
			ops = append(ops, op{name: name, value: value})
			i++
		default:
			break parse
		}
	}
	rest := argv[i:]
	if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
		return argv, env
	}
	for _, o := range ops {
		if o.name == "PATH" {
			return argv, env // the command would be found by another search path
		}
	}
	if _, err := osexec.LookPath(rest[0]); err != nil {
		return argv, env // keep env's own exit 127 for a missing command
	}
	out := make([]string, 0, len(env)+len(ops))
	out = append(out, env...)
	for _, o := range ops {
		kept := out[:0]
		for _, item := range out {
			if name, _, _ := strings.Cut(item, "="); name != o.name {
				kept = append(kept, item)
			}
		}
		out = kept
		if !o.unset {
			out = append(out, o.name+"="+o.value)
		}
	}
	return rest, out
}

// envName reports whether name can be an environment variable name that env
// would take literally (non-empty, no '=' or NUL).
func envName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "=\x00")
}
