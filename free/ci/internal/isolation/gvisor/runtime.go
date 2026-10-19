package gvisor

// Purpose: read the runsc entry of the daemon's runtime table and judge its arguments.
// Inputs:  the JSON of `docker info --format '{{json .Runtimes}}'`.
// Outputs: the entry, its platform (systrap, ptrace or kvm) or a refusal reason.
// Constraints: a runtime argument that widens the sandbox is refused, never stripped (the daemon
// would still start runsc with it); arguments are judged by an allow list (D-1066), matched on the flag name in
// every form the Go flag package accepts (-f, --f, f=v, f v).

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
)

// Stable reasons this package adds to the isolation ones.
const (
	ReasonRuntimeFlagDenied   = "isolation.runtime_flag_denied"
	ReasonPlatformUnsupported = "isolation.platform_unsupported"
)

// RuntimeName is the name the daemon registers gVisor under (`runsc install`).
const RuntimeName = "runsc"

// InstallHint is what `ci doctor` and an unavailable level tell the operator.
const InstallHint = "install gVisor (https://gvisor.dev/docs/user_guide/install/), run `sudo runsc install`, restart the Docker daemon"

// DefaultPlatform is what runsc uses when no --platform argument is set.
const DefaultPlatform = "systrap"

// flagKind says how a runsc flag takes its value.
type flagKind int

const (
	boolFlag  flagKind = iota // --f, --f=true or --f=false; a following word is not its value
	valueFlag                 // --f=v or --f v; v is any word that is not itself a flag
)

// flagSpec is one allowed runtime argument. values, when set, are the only accepted values.
type flagSpec struct {
	kind   flagKind
	values []string
}

// allowedFlags is the allow list of runsc arguments (D-1066). Anything else is refused, so a flag
// nobody reviewed (a new runsc release, nvproxy, ignore-cgroups, host-uds, net-raw, rootless,
// TESTONLY-*, profile, allow-flag-override) cannot widen the sandbox. These only choose the
// platform, add logging and tracing, or restate the safe default of an access setting. Epic D9's
// and the 2026-10-19 lead ruling's widening flags are a tripwire test, not this code's source.
var allowedFlags = map[string]*flagSpec{
	"platform":        {kind: valueFlag}, // judged by platformReason
	"debug":           {kind: boolFlag},
	"debug-log":       {kind: valueFlag},
	"log":             {kind: valueFlag},
	"log-format":      {kind: valueFlag},
	"log-packets":     {kind: boolFlag},
	"panic-log":       {kind: valueFlag},
	"strace":          {kind: boolFlag},
	"strace-syscalls": {kind: valueFlag},
	"strace-log-size": {kind: valueFlag},
	"file-access":     {kind: valueFlag, values: []string{"exclusive"}},
	"network":         {kind: valueFlag, values: []string{"sandbox", "none"}},
}

type runtimeEntry struct {
	Path string
	Args []string
}

// knownKeys are the entry keys this profile understands. Any other key (runtimeType and options
// of a containerd shim runtime, whose settings live in a file read by the shim, or a key a newer
// Docker adds) means the arguments are not all visible here, so the entry is refused.
var knownKeys = map[string]bool{"path": true, "runtimeArgs": true, "status": true}

// parseEntry returns the runsc entry. found is false when the daemon has none; foreign names a key
// of the entry this profile cannot judge ("" when every key is known).
func parseEntry(out []byte) (entry runtimeEntry, found bool, foreign string, err error) {
	var table map[string]map[string]json.RawMessage
	if err := json.Unmarshal(out, &table); err != nil {
		return runtimeEntry{}, false, "", err
	}
	raw, found := table[RuntimeName]
	if !found {
		return runtimeEntry{}, false, "", nil
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !knownKeys[k] {
			foreign = k
			break
		}
	}
	if v, ok := raw["path"]; ok {
		if err := json.Unmarshal(v, &entry.Path); err != nil {
			return runtimeEntry{}, true, "", err
		}
	}
	if v, ok := raw["runtimeArgs"]; ok {
		if err := json.Unmarshal(v, &entry.Args); err != nil {
			return runtimeEntry{}, true, "", err
		}
	}
	return entry, true, foreign, nil
}

// splitFlag normalises one runtime argument to (name, value, hasValue). A word that is not a flag
// (no dash, or only dashes) has an empty name.
func splitFlag(arg string) (string, string, bool) {
	trimmed := strings.TrimLeft(arg, "-")
	if trimmed == arg || trimmed == "" {
		return "", "", false
	}
	name, value, has := strings.Cut(trimmed, "=")
	return name, value, has
}

// checkArgs refuses every argument that is not on the allow list and returns the platform the
// arguments select. denial names the offending argument.
func checkArgs(args []string) (platform, denial string) {
	platform = DefaultPlatform
	for i := 0; i < len(args); i++ {
		name, value, has := splitFlag(args[i])
		if name == "" {
			return "", args[i] // a stray word, or a lone "--"
		}
		spec := allowedFlags[name]
		if spec == nil {
			return "", "--" + name
		}
		if spec.kind == boolFlag {
			if has && value != "true" && value != "false" {
				return "", "--" + name + "=" + value
			}
			continue
		}
		if !has { // the value is the next word, which must not itself be a flag
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				value = args[i]
			} else if i+1 < len(args) {
				return "", args[i+1]
			}
		}
		if spec.values != nil && !contains(spec.values, value) {
			return "", "--" + name + "=" + value
		}
		if name == "platform" {
			platform = value // the last one wins, as in runsc
		}
	}
	return platform, ""
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// platformReason refuses a platform this host cannot run: kvm needs /dev/kvm, and an unknown name is not ours to guess at.
func platformReason(platform string, hasKVM bool) (reason, why string) {
	switch platform {
	case "systrap", "ptrace":
		return "", ""
	case "kvm":
		if !hasKVM {
			return ReasonPlatformUnsupported, "platform kvm needs /dev/kvm, which this host lacks"
		}
		return "", ""
	}
	return ReasonPlatformUnsupported, "unknown runsc platform " + platform
}

// isRunsc reports whether the runtime entry's path names the runsc binary (a runtime called runsc
// that points at runc or a wrapper is not gVisor).
func isRunsc(runtimePath string) bool { return path.Base(runtimePath) == RuntimeName }
