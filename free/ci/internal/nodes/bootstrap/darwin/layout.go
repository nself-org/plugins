package darwin

// Purpose: the fixed layout of a macOS system node (Epic D20). The account names, the agent directory
// and the sudoers command are the ones P7-NODE-06 fixed for Linux (`nself-ci`, `nself-ci-job-<n>`,
// /opt/nself-ci-agent), so TRUST-09's job shim maps users the same way; only the places that differ
// on macOS (state under /var/db, the LaunchDaemon, the dscl id range) are defined here.
// Inputs:  none (constants and small helpers).
// Outputs: the names and paths every darwin script, the plist and the tests agree on.
// Constraints: the E669 reasons of this package are plain keys (the code itself is registered once, by
//          internal/nodes/bootstrap); a path or name here is never built from a node's reply.

import (
	"fmt"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
)

// Names shared with the Linux families (P7-NODE-06), so TRUST-09's job-shim mapping works unchanged.
const (
	agentUser  = bootstrap.AgentUser
	agentDir   = bootstrap.AgentDir
	agentPath  = bootstrap.AgentPath
	logDir     = bootstrap.LogDir
	sudoersDir = bootstrap.SudoersDir
)

// jobUser is the account of slot n (n = 1..slots).
func jobUser(n int) string { return bootstrap.JobUser(n) }

// The layout of a macOS system node.
const (
	// StateDir holds the node key and the age identity: mode 0700, owned by the agent user alone.
	StateDir = "/var/db/nself-ci-agent"
	// WorkDir is the agent's work base (executor work areas).
	WorkDir = "/var/db/nself-ci-work"
	// JobsDir holds one 0700 job root per slot, owned by that slot's job user; a job's HOME lives in it.
	JobsDir = "/var/db/nself-ci-jobs"
	// TrustedPath is a root-owned copy of the agent, installed beside Apple's own privileged helpers. The
	// agent under /opt is owned by the agent user (so an upgrade needs no root), which means a root
	// shell must never run it; `uninstall --system` runs only from this copy.
	TrustedPath = "/Library/PrivilegedHelperTools/org.nself.ci-agent"
	// Label is the launchd label of the pull-mode service.
	Label = "org.nself.ci-agent"
	// PlistPath is the LaunchDaemon of pull mode.
	PlistPath = "/Library/LaunchDaemons/org.nself.ci-agent.plist"
	// SudoersPath is the one drop-in holding a rule per slot user.
	SudoersPath = bootstrap.SudoersDir + "/" + bootstrap.AgentUser
	// EnrollmentFile is written last by `nself-ci-agent join`; the service starts when it exists.
	EnrollmentFile = StateDir + "/pki/enrollment.json"
	// IDLow and IDHigh bound the UniqueID (and group id) of every account the install creates.
	IDLow, IDHigh = 450, 499
	// MaxSlots is the most job users one Mac gets: the id range holds the agent user and its slots.
	MaxSlots = IDHigh - IDLow
)

// E669 reasons of the darwin family (the code is bootstrap.CodeBootstrap).
const (
	ReasonUsers        = "darwin_users"   // an account could not be created, found or removed
	ReasonSudoers      = "darwin_sudoers" // the sudoers drop-in did not validate or install
	ReasonLaunchd      = "launchd"        // the LaunchDaemon did not validate, load or unload
	ReasonXcodeLicense = "xcode_license"  // the Xcode license is not accepted system-wide (a warning)
	ReasonUninstall    = "uninstall_unsupported"
)

// scriptHead starts every script sent to a node: strict mode and a fixed PATH, so a root script never runs
// a program that the invoking user's PATH (or a directory the agent user can write) points at.
const scriptHead = "set -eu\nPATH=/usr/bin:/bin:/usr/sbin:/sbin\nexport PATH\n"

// scanDirs are the places a job user can leave a file by permission: its job root, the work area and the
// world-writable folders (the mounted volumes are added by the scripts). A whole-disk scan would walk
// Xcode and the system volumes for minutes and find nothing.
const scanDirs = JobsDir + " " + WorkDir + " /private/tmp /private/var/tmp /Users/Shared /Library/Caches"

// volumeDepth bounds how deep a scan walks into each top-level mounted volume: an external disk can hold
// millions of files, and a job user can only have written where a folder is world-writable, which is near
// the top. The job root, work area and temporary folders are scanned in full.
const volumeDepth = 4

// ownedLoops is shell text that runs `find` for files matching match (for example `-user "$u"`) with the
// given action (`-print` or `-depth -delete`) over dirs in full and over each mounted volume to volumeDepth.
// Every find ends in `|| true`: macOS /bin/sh runs a command substitution with errexit on, so one find
// that fails (an unreadable folder under /Library/Caches) must not end the loop before the volumes.
func ownedLoops(dirs, match, action string) string {
	return fmt.Sprintf(`for d in %[1]s; do if [ -d "$d" ]; then find "$d" -xdev %[2]s %[3]s 2>/dev/null || true; fi; done; for d in /Volumes/*; do if [ -d "$d" ]; then find "$d" -xdev -maxdepth %[4]d %[2]s %[3]s 2>/dev/null || true; fi; done`,
		dirs, match, action, volumeDepth)
}

// JobRoot is the job root of slot n (n = 1..slots).
func JobRoot(n int) string { return JobsDir + "/" + strconv.Itoa(n) }

// slotList is "1 2 3" for the shell loops of the scripts.
func slotList(slots int) string {
	s := ""
	for n := 1; n <= slots; n++ {
		if n > 1 {
			s += " "
		}
		s += strconv.Itoa(n)
	}
	return s
}
