package darwin

// Purpose: the scripts that install the agent binary and remove the whole darwin system layout (Epic
// D20: `nself-ci-agent uninstall --system` is the family's Uninstall).
// Inputs:  the uploaded file name (agent install); nothing for the removal.
// Outputs: script text for `sh -c`.
// Constraints: POSIX sh, `set -eu`, no backslash; removal order is service, processes, accounts and
//          groups, sudoers drop-in, then directories, so no running process or rule outlives its user;
//          it leaves no user, group, sudoers file, plist, /opt/nself-ci-agent, /var/db/nself-ci-* or
//          /var/log/nself-ci-agent behind, and the script fails if an account is still there at the end.

import "fmt"

// agentScript installs the uploaded binary (the file sits in the ssh user's home, the script's cwd).
// The agent user owns it, so `nodes upgrade` swaps it atomically without root; it stays world-readable
// and executable so a job user can run the shim through sudo.
func agentScript(uploaded string) string {
	return fmt.Sprintf(`set -eu
install -o %[3]s -g %[3]s -m 0755 ./%[1]s %[2]s
rm -f ./%[1]s
`, uploaded, agentPath, agentUser)
}

// uninstallScript removes the accounts, groups, sudoers drop-in and directories (the service is booted
// out by bootoutScript first).
func uninstallScript() string {
	return fmt.Sprintf(`set -eu
for u in $(dscl . -list /Users | grep -E '^%[1]s(-job-[0-9]+)?$' || true); do
  pkill -KILL -u "$u" >/dev/null 2>&1 || true
done
rm -f %[2]s
for u in $(dscl . -list /Users | grep -E '^%[1]s(-job-[0-9]+)?$' || true); do
  dscl . -delete "/Users/$u" >/dev/null 2>&1 || true
done
for g in $(dscl . -list /Groups | grep -E '^%[1]s(-job-[0-9]+)?$' || true); do
  dscl . -delete "/Groups/$g" >/dev/null 2>&1 || true
done
rm -rf %[3]s %[4]s %[5]s %[6]s %[7]s
left=$(dscl . -list /Users | grep -E '^%[1]s(-job-[0-9]+)?$' || true)
[ -z "$left" ] || { echo "accounts still present: $left" >&2; exit 1; }
`, agentUser, SudoersPath, agentDir, StateDir, WorkDir, JobsDir, logDir)
}
