package darwin

// Purpose: the Xcode license probe of a macOS system install (Epic D20, acceptance "xcode_license").
// A job user cannot run xcodebuild until an administrator accepts the license system-wide, so the
// install asks `xcodebuild -license check` AS the first job user and, when the answer is no, warns with
// the one-line fix instead of failing: the node is usable for everything that is not Xcode.
// Inputs:  the node's first job user and an optional PATH prefix (tests put a stub xcodebuild in it).
// Outputs: a Warning, or "" when the license is accepted or Xcode is not installed at all.
// Constraints: the probe runs as root (`sudo -u <job user>`, no sudoers rule involved), changes nothing,
//          and is bounded to one command; a node without Xcode is not a license problem.

import (
	"fmt"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

const defaultPath = "/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin:/opt/homebrew/bin"

// XcodeFix is the one-line fix the warning carries.
const XcodeFix = "sudo xcodebuild -license accept"

// xcodeProbeScript prints "ok", "absent" or "license" (the license is not accepted system-wide).
func xcodeProbeScript(pathPrefix string) string {
	path := defaultPath
	if pathPrefix != "" {
		path = pathPrefix + ":" + path
	}
	return fmt.Sprintf(`set -eu
P=%[2]s
if ! sudo -n -u %[1]s env PATH="$P" HOME=/var/empty sh -c 'command -v xcodebuild' >/dev/null 2>&1; then echo absent; exit 0; fi
if sudo -n -u %[1]s env PATH="$P" HOME=/var/empty xcodebuild -license check >/dev/null 2>&1; then echo ok; else echo license; fi
`, jobUser(1), remote.ShellQuote(path))
}

// xcodeWarning turns the probe's answer into the warning text ("" when there is nothing to warn about).
func xcodeWarning(answer string) string {
	if answer != "license" {
		return ""
	}
	return fmt.Sprintf("E669 %s: the Xcode license is not accepted system-wide, so %s cannot run xcodebuild; the node is recorded without the xcode fact. Fix: %s",
		ReasonXcodeLicense, jobUser(1), XcodeFix)
}
