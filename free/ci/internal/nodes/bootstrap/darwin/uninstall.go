package darwin

// Purpose: the scripts that install the agent binary and remove the whole darwin system layout (Epic
// D20: `nself-ci-agent uninstall --system` is the family's Uninstall).
// Inputs:  the uploaded file name (agent install); nothing for the removal.
// Outputs: script text for `sh -c`.
// Constraints: POSIX sh, `set -eu`, a fixed PATH, no backslash; removal order is service, processes,
//          sudoers drop-in, accounts and groups, then directories, so no running process or rule outlives its user;
//          it leaves no user, group, sudoers file, plist, /opt/nself-ci-agent, /var/db/nself-ci-* or
//          /var/log/nself-ci-agent behind (the root-owned agent copy goes last), and the script fails if an
//          account is still there at the end. Every script sets PATH to the system directories first, so
//          dscl, launchctl and pkill are never found in a directory the agent user can write.

import (
	"fmt"
	"path"
)

// agentScript installs the uploaded binary (the file sits in the ssh user's home, the script's cwd).
// The binary goes into the agent-owned bin directory AS the agent user (root only opens the upload; a
// symlink the agent plants there reaches nothing the agent could not already reach) and stays
// world-readable and executable so a job user can run the shim through sudo; the agent user can swap it
// atomically without root (`nodes upgrade`). A second, root-owned copy goes to TrustedPath, in a root-owned
// directory: that is the only file `uninstall --system` runs as root.
func agentScript(uploaded string) string {
	return fmt.Sprintf(scriptHead+`sudo -n -u %[3]s sh -c 'rm -f %[6]s/bin/.upload.new && cat > %[6]s/bin/.upload.new && chmod 0755 %[6]s/bin/.upload.new && mv -f %[6]s/bin/.upload.new %[2]s' < ./%[1]s
install -d -o root -g wheel -m 0755 %[4]s
install -o root -g wheel -m 0755 ./%[1]s %[5]s
rm -f ./%[1]s
`, uploaded, agentPath, agentUser, path.Dir(TrustedPath), TrustedPath, agentDir)
}

// uninstallScript removes the accounts, groups, sudoers drop-in and directories (the service is booted
// out by bootoutScript first). Before each account goes, its processes, crontab and files in the places a
// job user can write are removed, so no id is left owning a file that a later account could inherit.
func uninstallScript() string {
	return fmt.Sprintf(scriptHead+`for u in $(dscl . -list /Users | grep -E '^%[1]s(-job-[0-9]+)?$' || true); do
  pkill -KILL -u "$u" >/dev/null 2>&1 || true
done
sleep 1
rm -f %[2]s
for u in $(dscl . -list /Users | grep -E '^%[1]s(-job-[0-9]+)?$' || true); do
  pkill -KILL -u "$u" >/dev/null 2>&1 || true
  rm -f "/var/at/tabs/$u"
  find /private/var/folders -xdev -user "$u" -depth -delete >/dev/null 2>&1 || true
  %[9]s
  dscl . -delete "/Users/$u" >/dev/null 2>&1 || true
done
for g in $(dscl . -list /Groups | grep -E '^%[1]s(-job-[0-9]+)?$' || true); do
  dscl . -delete "/Groups/$g" >/dev/null 2>&1 || true
done
if [ -f /var/at/cron.deny ]; then
  keep=$(grep -Ev '^%[1]s(-job-[0-9]+)?$' /var/at/cron.deny || true)
  if [ -n "$keep" ]; then echo "$keep" > /var/at/cron.deny; else : > /var/at/cron.deny; fi
fi
rm -rf %[3]s %[4]s %[5]s %[6]s %[7]s
rm -f %[8]s
left=$(dscl . -list /Users | grep -E '^%[1]s(-job-[0-9]+)?$' || true)
[ -z "$left" ] || { echo "accounts still present: $left" >&2; exit 1; }
`, agentUser, SudoersPath, agentDir, StateDir, WorkDir, JobsDir, logDir, TrustedPath, ownedLoops(scanDirs, `-user "$u"`, "-depth -delete"))
}
