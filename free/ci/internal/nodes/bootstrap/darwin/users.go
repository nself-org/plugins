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
//          are deleted with it).

import "fmt"

// usersScript creates the agent user and the job users 1..slots. The id search runs again for every
// account, so two accounts of one run never get the same id.
func usersScript(slots int) string {
	return fmt.Sprintf(`set -eu
free_id() {
  used=$({ dscl . -list /Users UniqueID; dscl . -list /Groups PrimaryGroupID; } | awk '{print $NF}')
  i=%[2]d
  while [ "$i" -le %[3]d ]; do
    if ! echo "$used" | grep -qx "$i"; then echo "$i"; return 0; fi
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
`, agentUser, IDLow, IDHigh, slotList(slots))
}

// dirsScript creates the directories with their owners and modes. The agent directory belongs to the
// agent user so `nodes upgrade` swaps the binary without root; the state directory is its alone.
func dirsScript(slots int) string {
	return fmt.Sprintf(`set -eu
install -d -o %[1]s -g %[1]s -m 0755 %[2]s %[2]s/bin
install -d -o %[1]s -g %[1]s -m 0700 %[3]s
install -d -o %[1]s -g %[1]s -m 0711 %[4]s
install -d -o %[1]s -g %[1]s -m 0750 %[5]s
install -d -o root -g wheel -m 0711 %[6]s
for n in %[7]s; do install -d -o %[1]s-job-$n -g %[1]s-job-$n -m 0700 %[6]s/$n; done
chown -R %[1]s:%[1]s %[3]s
chmod -R go-rwx %[3]s
`, agentUser, agentDir, StateDir, WorkDir, logDir, JobsDir, slotList(slots))
}

// listJobUsersScript prints the job users that exist, one per line.
func listJobUsersScript() string {
	return fmt.Sprintf("set -eu\ndscl . -list /Users | grep -E '^%s-job-[0-9]+$' || true\n", agentUser)
}

// removeSlotScript retires slot n. Its job root goes when empty; its per-user folders under
// /private/var/folders (the system makes them for any process run as the user) are deleted; its user and
// group go only when the user owns no other file in the places a job user can write: its job root and the
// work area, the temporary and shared folders, the shared caches and the top-level mounted volumes. A scan
// of the whole disk would walk Xcode and the system volumes for minutes per slot and find nothing: the
// account has no write permission anywhere else. It prints "removed" or "kept".
func removeSlotScript(n int) string {
	return fmt.Sprintf(`set -eu
u=%[1]s
pkill -KILL -u "$u" >/dev/null 2>&1 || true
rmdir %[2]s 2>/dev/null || true
find /private/var/folders -xdev -user "$u" -depth -delete >/dev/null 2>&1 || true
left=$(for d in %[3]s %[4]s /private/tmp /private/var/tmp /Users/Shared /Library/Caches /Volumes/*; do [ -d "$d" ] && find "$d" -xdev -user "$u" -print; done 2>/dev/null | head -n 1)
if [ -n "$left" ]; then echo kept; exit 0; fi
dscl . -delete "/Users/$u" >/dev/null 2>&1 || true
dscl . -delete "/Groups/$u" >/dev/null 2>&1 || true
if dscl . -read "/Users/$u" UniqueID >/dev/null 2>&1; then echo kept; else echo removed; fi
`, jobUser(n), JobRoot(n), JobsDir, WorkDir)
}
