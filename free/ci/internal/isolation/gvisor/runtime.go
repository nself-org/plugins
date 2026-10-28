package gvisor

// Purpose: read the runsc entry of the daemon's runtime table and judge its arguments.
// Inputs:  the JSON of `docker info --format '{{json .Runtimes}}'`.
// Outputs: the entry, its platform (systrap, ptrace or kvm) or a refusal reason.
// Constraints: only known-safe runtime arguments are accepted; unknown flags or
// positional arguments are refused because the daemon passes them to runsc.

import (
	"encoding/json"
	"path"
	"sort"
	"strconv"
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

// splitFlag normalises one runtime argument to (name, value, hasValue).
func splitFlag(arg string) (string, string, bool) {
	trimmed := strings.TrimLeft(arg, "-")
	if trimmed == arg || trimmed == "" {
		return "", "", false // not a flag
	}
	name, value, has := strings.Cut(trimmed, "=")
	return name, value, has
}

// checkArgs accepts only platform, debug-log and strace. String flags consume
// their next argument; boolean flags use Go flag syntax (bare or =value).
func checkArgs(args []string) (platform, denial string) {
	platform = DefaultPlatform
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value, has := splitFlag(arg)
		if strings.HasPrefix(arg, "---") {
			return "", arg
		}
		switch name {
		case "platform", "debug-log":
			if !has {
				i++
				if i >= len(args) || strings.HasPrefix(args[i], "-") {
					return "", "--" + name
				}
				value = args[i]
			}
			if value == "" {
				return "", "--" + name
			}
			if name == "platform" {
				platform = value
			}
		case "strace":
			if has {
				if _, err := strconv.ParseBool(value); err != nil {
					return "", "--strace"
				}
			}
		default:
			return "", "unexpected runtime argument: " + arg
		}
	}
	return platform, ""
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
