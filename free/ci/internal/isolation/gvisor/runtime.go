package gvisor

// Purpose: read the runsc entry of the daemon's runtime table and judge its arguments.
// Inputs:  the JSON of `docker info --format '{{json .Runtimes}}'`.
// Outputs: the entry, its platform (systrap, ptrace or kvm) or a refusal reason.
// Constraints: a runtime argument that widens the sandbox is refused, never stripped (the daemon
// would still start runsc with it); the deny list is Epic D9's, matched on the flag name in
// every form the Go flag package accepts (-f, --f, f=v, f v).

import (
	"encoding/json"
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

// denied are the flags that open the sandbox to the host or drop a limit; presence alone is a
// refusal (so `--net-raw=false` is refused too, which is safe and simple). The first four are
// Epic D9's list; ignore-cgroups (the pids, memory and cpu limits would not be enforced) and
// rootless were added by the lead ruling of 2026-10-19; TESTONLY-* flags and `file-access=shared`
// are matched in checkArgs.
var denied = map[string]bool{"host-uds": true, "host-fifo": true, "net-raw": true, "allow-flag-override": true, "ignore-cgroups": true, "rootless": true}

type runtimeEntry struct {
	Path string   `json:"path"`
	Args []string `json:"runtimeArgs"`
}

// parseEntry returns the runsc entry; ok is false when the daemon has none.
func parseEntry(out []byte) (entry runtimeEntry, ok bool, err error) {
	var table map[string]runtimeEntry
	if err := json.Unmarshal(out, &table); err != nil {
		return runtimeEntry{}, false, err
	}
	entry, ok = table[RuntimeName]
	return entry, ok, nil
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

// flagValue returns the value of flag i: its `=value`, else the next argument.
func flagValue(args []string, i int, value string, has bool) string {
	if has {
		return value
	}
	if i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

// checkArgs refuses a deny-listed argument and returns the platform the arguments select.
func checkArgs(args []string) (platform, denial string) {
	platform = DefaultPlatform
	for i, arg := range args {
		name, value, has := splitFlag(arg)
		switch {
		case name == "":
		case denied[name], strings.HasPrefix(strings.ToLower(name), "testonly"):
			return "", "--" + name
		case name == "file-access" && flagValue(args, i, value, has) == "shared":
			return "", "--file-access=shared"
		case name == "network" && flagValue(args, i, value, has) == "host":
			return "", "--network=host"
		case name == "platform":
			platform = flagValue(args, i, value, has) // the last one wins, as in runsc
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
