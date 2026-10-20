package darwin

// Purpose: the dscl scripts of a macOS system node: the hidden role user `nself-ci`, its group, and the
// per-slot job users `nself-ci-job-<n>` (Epic D20). dscl is used because it runs non-interactively under
// sudo and creates a hidden role account without a home directory; sysadminctl would prompt.
// Inputs:  the slot count (and, for retirement, one slot number).
// Outputs: script text for `sh -c`.
// Constraints: POSIX sh; `set -eu`; no backslash (the sdk refuses one in an argument); every account gets
//          a UniqueID and a group id that is free in 450-499 (checked against users AND groups), shell
//          /usr/bin/false, home /var/empty, IsHidden 1, password `*`, and its own group; IsHidden is
//          written last and is the commit marker: a complete account is left alone, a partial one (an
//          interrupted install) and any leftover group of its name are deleted and built again with a
//          fresh id, so a stale group id can never give two accounts one uid; a slot user is removed
//          only when it owns no file (its dirhelper folders under /private/var/folders do not count and
//          are deleted with it). Cron: macOS lets any user install a crontab (measured on macos-15: no
//          policy file blocked a job user), and a crontab outlives the job and the account, so every
//          account is appended to /var/at/cron.deny; the file is created only when the host would
//          otherwise let a job user in (a root-only default must not be opened to everyone else).

import "fmt"

// usersScript creates the agent user and the job users 1..slots. The id search runs again for every
// account, so two accounts of one run never get the same id.
func usersScript(slots int) string {
	return fmt.Sprintf(scriptHead+`free_id() {
  used=$({ dscl . -list /Users UniqueID; dscl . -list /Groups PrimaryGroupID; } | awk '{print $NF}')
  i=%[2]d
  while [ "$i" -le %[3]d ]; do
    if ! echo "$used" | grep -qx "$i"; then
      held=$({ %[5]s; } 2>/dev/null | head -n 1)
      if [ -z "$held" ]; then echo "$i"; return 0; fi
    fi
    i=$((i + 1))
  done
  return 1
}
mk() {
  u=$1
  if dscl . -read "/Users/$u" IsHidden >/dev/null 2>&1; then return 0; fi
  dscl . -delete "/Users/$u" >/dev/null 2>&1 || true
  dscl . -delete "/Groups/$u" >/dev/null 2>&1 || true
  id=$(free_id) || { echo "no free id in %[2]d-%[3]d" >&2; exit 1; }
  dscl . -create "/Groups/$u"
  dscl . -create "/Groups/$u" PrimaryGroupID "$id"
  dscl . -create "/Groups/$u" RealName "nSelf CI"
  dscl . -create "/Users/$u"
  dscl . -create "/Users/$u" UserShell /usr/bin/false
  dscl . -create "/Users/$u" RealName "nSelf CI"
  dscl . -create "/Users/$u" UniqueID "$id"
  dscl . -create "/Users/$u" PrimaryGroupID "$id"
  dscl . -create "/Users/$u" NFSHomeDirectory /var/empty
  dscl . -create "/Users/$u" Password '*'
  dscl . -create "/Users/$u" IsHidden 1
}
mk %[1]s
for n in %[4]s; do mk %[1]s-job-$n; done
if [ -f /var/at/cron.deny ] || ! sudo -n -u %[1]s-job-1 crontab -l 2>&1 | grep -q 'not allowed'; then
  [ -f /var/at/cron.deny ] || { : > /var/at/cron.deny; chmod 0644 /var/at/cron.deny; }
  for u in %[1]s $(for n in %[4]s; do echo %[1]s-job-$n; done); do grep -qx "$u" /var/at/cron.deny || echo "$u" >> /var/at/cron.deny; done
fi
`, agentUser, IDLow, IDHigh, slotList(slots), ownedLoops(scanDirs, `-uid "$i"`, "-print"))
}

// dirsScript creates the directories with their owners and modes. Root only ever creates or changes an
// entry whose PARENT is root-owned, so the agent user cannot swap it for a symlink: /opt/nself-ci-agent is
// root's, and its bin directory (agent-owned, so `nodes upgrade` swaps the binary without root) cannot be
// renamed or replaced by the agent; the log directory is root's with one agent-owned agent.log inside
// (launchd opens it, so the agent must not control the name); the state directory is the agent's alone.
// Nothing here walks into a directory the agent user owns (no `chown -R`, no `chmod -R`): a root-run walk
// of a tree the agent controls is a symlink or a hard link away from changing a file it must not touch.
func dirsScript(slots int) string {
	return fmt.Sprintf(scriptHead+`install -d -o root -g wheel -m 0755 %[2]s
install -d -o %[1]s -g %[1]s -m 0755 %[2]s/bin
install -d -o %[1]s -g %[1]s -m 0700 %[3]s
install -d -o %[1]s -g %[1]s -m 0711 %[4]s
install -d -o root -g wheel -m 0755 %[5]s
[ -e %[5]s/agent.log ] || : > %[5]s/agent.log
chown -h %[1]s:%[1]s %[5]s/agent.log
chmod 0640 %[5]s/agent.log
install -d -o root -g wheel -m 0711 %[6]s
for n in %[7]s; do install -d -o %[1]s-job-$n -g %[1]s-job-$n -m 0700 %[6]s/$n; done
`, agentUser, agentDir, StateDir, WorkDir, logDir, JobsDir, slotList(slots))
}

// listJobUsersScript prints the job users that exist, one per line.
func listJobUsersScript() string {
	return fmt.Sprintf(scriptHead+"dscl . -list /Users | grep -E '^%s-job-[0-9]+$' || true\n", agentUser)
}

// removeSlotScript retires slot n. Its job root goes when empty; its per-user folders under
// /private/var/folders (the system makes them for any process run as the user) are deleted; its crontab
// file goes (`crontab -u` is refused for a denied user, so the file under /var/at/tabs is removed directly); its user and group go only when no process and no file of the user is left in the places a job
// user can write (scanDirs and the top-level mounted volumes). The sudoers drop-in has already lost the
// slot's rule (Install writes it first), so the agent cannot start a new shim for the user meanwhile.
// It prints "removed" or "kept".
func removeSlotScript(n int) string {
	return fmt.Sprintf(scriptHead+`u=%[1]s
pkill -KILL -u "$u" >/dev/null 2>&1 || true
sleep 1
pkill -KILL -u "$u" >/dev/null 2>&1 || true
if pgrep -u "$u" >/dev/null 2>&1; then echo kept; exit 0; fi
rm -f "/var/at/tabs/$u"
rmdir %[2]s 2>/dev/null || true
find /private/var/folders -xdev -user "$u" -depth -delete >/dev/null 2>&1 || true
left=$({ %[3]s; } 2>/dev/null | head -n 1)
if [ -n "$left" ]; then echo kept; exit 0; fi
dscl . -delete "/Users/$u" >/dev/null 2>&1 || true
dscl . -delete "/Groups/$u" >/dev/null 2>&1 || true
if dscl . -read "/Users/$u" UniqueID >/dev/null 2>&1; then echo kept; else echo removed; fi
`, jobUser(n), JobRoot(n), ownedLoops(scanDirs, `-user "$u"`, "-print"))
}
