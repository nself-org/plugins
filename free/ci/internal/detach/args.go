package detach

// Purpose: argv and environment rules of the detached child (P7-CI-63).
// Inputs:  the user's argv after `nself ci`, the parent environment.
// Outputs: the child argv (same argv minus --detach plus --json --stream), the
// coordinator target for the handle, the child environment.
// Constraints: only tokens before the first `--` are flags; the child never
// receives NSELF_MCP_TOKEN; nothing here starts a process or reads the disk.

import "strings"

// EnvMCPToken is the machine-surface token the child must not inherit.
const EnvMCPToken = "NSELF_MCP_TOKEN"

// EnvCoordinator is the coordinator target variable (P7-NODE D14).
const EnvCoordinator = "NSELF_CI_COORDINATOR"

// splitAdhoc splits args at the first `--`.
func splitAdhoc(args []string) (flags, rest []string, has bool) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i:], true
		}
	}
	return args, nil, false
}

func isDetachFlag(a string) bool {
	a = strings.TrimLeft(a, "-")
	return a == "detach" || strings.HasPrefix(a, "detach=")
}

// ChildArgs is the child's argv (without the executable): --json --stream
// first (Go's flag parsing stops at the first positional argument, such as a
// repository root, so anything appended after it would be ignored), then the
// user's argv without --detach, --json and --stream, then any `-- <cmd…>`.
func ChildArgs(args []string) []string {
	flags, rest, _ := splitAdhoc(args)
	out := make([]string, 0, len(args)+2)
	out = append(out, "--json", "--stream")
	for _, a := range flags {
		if strings.HasPrefix(a, "-") {
			name, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "=")
			if isDetachFlag(a) || name == "json" || name == "stream" { // the child always gets --json --stream, whatever the user wrote
				continue
			}
		}
		out = append(out, a)
	}
	return append(out, rest...)
}

// CoordinatorOf is the coordinator the run goes to: --coordinator <url> before
// any `--`, else NSELF_CI_COORDINATOR, else "local".
func CoordinatorOf(args []string, getenv func(string) string) string {
	flags, _, _ := splitAdhoc(args)
	for i, a := range flags {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name := strings.TrimLeft(a, "-")
		switch {
		case strings.HasPrefix(name, "coordinator="):
			if v := strings.TrimPrefix(name, "coordinator="); v != "" {
				return v
			}
		case name == "coordinator" && i+1 < len(flags) && flags[i+1] != "":
			return flags[i+1]
		}
	}
	if v := getenv(EnvCoordinator); v != "" {
		return v
	}
	return Local
}

// ChildEnv is the parent environment minus NSELF_MCP_TOKEN.
func ChildEnv(parent []string) []string {
	out := make([]string, 0, len(parent))
	for _, kv := range parent {
		if strings.HasPrefix(kv, EnvMCPToken+"=") || kv == EnvMCPToken {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// envLookup answers getenv from an environment slice.
func envLookup(env []string) func(string) string {
	return func(name string) string {
		for i := len(env) - 1; i >= 0; i-- {
			if v, ok := strings.CutPrefix(env[i], name+"="); ok {
				return v
			}
		}
		return ""
	}
}

// JobOf is the job whose logs the handle names: "adhoc" for `-- <cmd…>`, the
// --job value when given, else "" (not known before the run is planned).
func JobOf(args []string) string {
	flags, _, has := splitAdhoc(args)
	if has {
		return "adhoc"
	}
	for i, a := range flags {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name := strings.TrimLeft(a, "-")
		switch {
		case strings.HasPrefix(name, "job="):
			return strings.TrimPrefix(name, "job=")
		case name == "job" && i+1 < len(flags):
			return flags[i+1]
		}
	}
	return ""
}
