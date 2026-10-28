package bootstrap

// Purpose: the shell scripts a Linux family runs on the node. Each is one idempotent `sh -c` script
// built from constants and integers, so a test can read the exact text and `sh -n` can parse it.
// Inputs:  the slot count (and, for removal, one slot number).
// Outputs: script text.
// Constraints: POSIX sh only (Alpine's ash, Debian's dash); `set -eu`; no backslash anywhere (the sdk's
//          RunArgv refuses one); every file is installed with an explicit owner and mode; a sudoers
//          file is checked with `visudo -cf` before it is installed and the whole set with `visudo -c`
//          after; the rule names the job shim and nothing else, with no docker group and no extra sudo.

import (
	"fmt"
	"strings"
)

// packagesScript installs the family's toolchain manifest.
func packagesScript(install string, pkgs []string) string {
	return "set -eu\n" + install + " " + strings.Join(pkgs, " ") + "\n"
}

// usersScript creates the agent user and the job users 1..slots (existing users are left alone).
func usersScript(slots int) string {
	return fmt.Sprintf(`set -eu
NOLOGIN=/bin/false
for p in /usr/sbin/nologin /sbin/nologin; do [ -x "$p" ] && NOLOGIN=$p && break; done
mk() {
  id -u "$1" >/dev/null 2>&1 || useradd --system --user-group --no-create-home --home-dir "$2" --shell "$NOLOGIN" "$1"
}
mk %[1]s %[2]s
for n in %[3]s; do mk %[1]s-job-$n /nonexistent; done
`, AgentUser, StateDir, slotList(slots))
}

// dirsScript creates the directories with their owners and modes.
func dirsScript(slots int) string {
	return fmt.Sprintf(`set -eu
install -d -o root -g root -m 0755 %[2]s %[2]s/bin
install -d -o %[1]s -g %[1]s -m 0700 %[3]s
install -d -o %[1]s -g %[1]s -m 0711 %[4]s
install -d -o %[1]s -g %[1]s -m 0750 %[5]s
install -d -o root -g root -m 0711 %[6]s
for n in %[7]s; do install -d -o %[1]s-job-$n -g %[1]s-job-$n -m 0700 %[6]s/$n; done
`, AgentUser, AgentDir, StateDir, WorkDir, LogDir, JobsDir, slotList(slots))
}

// sudoersScript writes one rule file per slot user. Each rule lets the agent user run the job shim as
// that one job user and nothing else. Every rule is staged and checked with `visudo -cf` before
// /etc/sudoers or /etc/sudoers.d is touched, and a failure at any later point restores /etc/sudoers.
func sudoersScript(slots int) string {
	return fmt.Sprintf(`set -eu
umask 077
tmp=$(mktemp -d)
cp /etc/sudoers "$tmp/sudoers.orig"
ok=0
cleanup() {
  if [ "$ok" != 1 ] && ! cmp -s "$tmp/sudoers.orig" /etc/sudoers; then cp "$tmp/sudoers.orig" /etc/sudoers 2>/dev/null || true; fi
  rm -rf "$tmp"
}
trap cleanup EXIT
for n in %[2]s; do
  echo "%[3]s ALL=(%[3]s-job-$n) NOPASSWD: %[4]s job-shim" > "$tmp/%[3]s-job-$n"
  visudo -cf "$tmp/%[3]s-job-$n" >/dev/null
done
grep -Eq '^[#@]includedir[[:space:]]+%[1]s' /etc/sudoers || { echo >> /etc/sudoers; echo '#includedir %[1]s' >> /etc/sudoers; }
mkdir -p %[1]s
fresh=""
for n in %[2]s; do
  [ -e %[1]s/%[3]s-job-$n ] || fresh="$fresh %[1]s/%[3]s-job-$n"
  install -o root -g root -m 0440 "$tmp/%[3]s-job-$n" %[1]s/%[3]s-job-$n
done
if ! visudo -c >/dev/null 2>&1; then
  for f in $fresh; do rm -f "$f"; done
  echo 'sudoers did not validate; nothing left in place' >&2
  exit 1
fi
ok=1
`, SudoersDir, slotList(slots), AgentUser, AgentPath)
}

// listJobUsersScript prints the job users that exist, one per line.
func listJobUsersScript() string {
	return fmt.Sprintf("set -eu\ngetent passwd | cut -d: -f1 | grep -E '^%s-job-[0-9]+$' || true\n", AgentUser)
}

// removeSlotScript retires slot n: its rule goes always; its user and group go only when the user owns
// no file anywhere on the root filesystem. It prints "removed" or "kept".
func removeSlotScript(n int) string {
	return fmt.Sprintf(`set -eu
u=%[1]s
rm -f %[2]s
pkill -KILL -u "$u" >/dev/null 2>&1 || true
rmdir %[3]s 2>/dev/null || true
left=$(for d in / /var /tmp /home /opt /srv /usr/local /root; do [ -d "$d" ] && find "$d" -xdev -user "$u" 2>/dev/null | head -n 1; done | head -n 1)
if [ -n "$left" ]; then echo kept; exit 0; fi
userdel "$u" >/dev/null 2>&1 || true
getent group "$u" >/dev/null 2>&1 && groupdel "$u" >/dev/null 2>&1 || true
if id -u "$u" >/dev/null 2>&1; then echo kept; else echo removed; fi
`, JobUser(n), SudoersFile(n), JobRoot(n))
}

// agentScript installs the uploaded binary (the file sits in the ssh user's home, the script's cwd).
func agentScript(uploaded string) string {
	return fmt.Sprintf(`set -eu
install -o root -g root -m 0755 ./%[1]s %[2]s
rm -f ./%[1]s
`, uploaded, AgentPath)
}

// verifyScript checks the layout after an install: every user exists and none is in the docker group,
// the state directory is the agent's alone, each rule file is in place and sudo itself lists exactly
// one job-shim rule per slot for the agent user (so a rule sudo ignores, or a surplus rule, fails here).
func verifyScript(slots int) string {
	return fmt.Sprintf(`set -eu
id -u %[1]s >/dev/null
[ "$(stat -c '%%a %%U' %[2]s)" = "700 %[1]s" ]
for n in %[3]s; do
  id -u %[1]s-job-$n >/dev/null
  [ -f %[4]s/%[1]s-job-$n ]
done
for u in $(getent passwd | cut -d: -f1 | grep -E '^%[1]s(-job-[0-9]+)?$'); do
  if id -nG "$u" | grep -qwE 'docker|sudo|wheel|adm'; then echo "$u is in a privileged group" >&2; exit 1; fi
done
rules=$(sudo -n -l -U %[1]s 2>/dev/null | grep -c '%[5]s job-shim' || true)
[ "$rules" = %[6]d ] || { echo "sudo lists $rules job-shim rules, want %[6]d" >&2; exit 1; }
all=$(sudo -n -l -U %[1]s 2>/dev/null | grep -cE '^[[:space:]]*[(]' || true)
[ "$all" = %[6]d ] || { echo "sudo lists $all rules for %[1]s, want only the %[6]d job-shim rules" >&2; exit 1; }
`, AgentUser, StateDir, slotList(slots), SudoersDir, AgentPath, slots)
}

// uninstallScript removes the whole system layout.
func uninstallScript() string {
	return fmt.Sprintf(`set -eu
if [ -d /run/systemd/system ]; then systemctl disable --now nself-ci-agent >/dev/null 2>&1 || true; fi
rm -f %[1]s
if [ -d /run/systemd/system ]; then systemctl daemon-reload >/dev/null 2>&1 || true; fi
for f in %[2]s/%[3]s-job-*; do [ -e "$f" ] && rm -f "$f"; done
for u in $(getent passwd | cut -d: -f1 | grep -E '^%[3]s(-job-[0-9]+)?$' || true); do
  pkill -KILL -u "$u" >/dev/null 2>&1 || true
  userdel "$u" >/dev/null 2>&1 || true
  getent group "$u" >/dev/null 2>&1 && groupdel "$u" >/dev/null 2>&1 || true
done
rm -rf %[4]s %[5]s %[6]s %[7]s %[8]s
`, UnitPath, SudoersDir, AgentUser, AgentDir, StateDir, WorkDir, JobsDir, LogDir)
}

// unitScript writes the pull-mode service unit (systemd families only) and reloads systemd when it runs.
func unitScript() string {
	return "set -eu\nmkdir -p /etc/systemd/system\ncat > " + UnitPath + " <<'NSELF_UNIT'\n" + Unit() +
		"NSELF_UNIT\nchmod 0644 " + UnitPath + "\nif [ -d /run/systemd/system ]; then systemctl daemon-reload >/dev/null 2>&1 || true; fi\n"
}

// Unit is the text of the pull-mode service unit. NoNewPrivileges is deliberately absent: the agent
// starts every job through sudo (the job shim), which needs the setuid bit. Delegate=yes hands the unit's
// cgroup (v2) to the agent user: contention.Isolator steps into <own>/nself-ci-agent and enables cpu and
// memory for its children only in a delegated cgroup, else the reservation cap is reported unknown.
func Unit() string {
	return fmt.Sprintf(`[Unit]
Description=nSelf CI agent (pull mode)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%[1]s
Group=%[1]s
Delegate=yes
Environment=NSELF_CI_AGENT_HOME=%[2]s
Environment=NSELF_CI_AGENT_WORK=%[3]s
ExecStart=%[4]s pull
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`, AgentUser, StateDir, WorkDir, AgentPath)
}
