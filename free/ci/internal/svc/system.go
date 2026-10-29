package svc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// SystemHome is private watcher state, separate from agent and job accounts.
func SystemHome(manager string) string {
	if manager == "launchd" {
		return "/var/db/nself-ci-watch"
	}
	return "/var/lib/nself-ci-watch"
}

func (c Config) systemGuard() error {
	if !c.System {
		return nil
	}
	if c.Home != SystemHome(c.Manager) || c.CIHome != c.Home {
		return fmt.Errorf("system watcher requires its dedicated home %s", SystemHome(c.Manager))
	}
	if c.DryRun {
		return nil
	}
	if os.Getenv("CI") == "1" {
		return fmt.Errorf("system service mutation is disabled under CI=1")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("--system requires sudo; rerun with sudo and explicit --binary and --repo")
	}
	agent := "/etc/systemd/system/nself-ci-agent.service"
	if c.Manager == "launchd" {
		agent = "/Library/LaunchDaemons/org.nself.ci-agent.plist"
	}
	if fi, err := os.Lstat(agent); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("--system requires an installed node agent")
	}
	return nil
}

func (i Installer) prepareSystem(ctx context.Context) error {
	c := i.Config
	if err := c.systemGuard(); err != nil {
		return err
	}
	if !c.System {
		return nil
	}
	// Constant scripts follow NODE-06/NODE-31: a locked role account, no home
	// inherited from sudo, no job-user group, and no sudoers changes.
	script := linuxAccount
	if c.Manager == "launchd" {
		script = darwinAccount
	}
	if c.DryRun {
		fmt.Fprintf(i.writer(), "/bin/sh -c %q\n", script)
		return nil
	}
	if err := i.Run(ctx, "/bin/sh", "-c", script); err != nil {
		return err
	}
	for _, path := range []string{c.Home, filepath.Join(c.Home, "logs"), filepath.Join(c.Home, "logs", "watch")} {
		fi, err := os.Lstat(path)
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid system watcher directory %s", path)
		}
	}
	return nil
}

const linuxAccount = `set -eu
PATH=/usr/bin:/bin:/usr/sbin:/sbin
export PATH
u=nself-ci-watch
id -u "$u" >/dev/null 2>&1 || useradd --system --user-group --no-create-home --home-dir /var/lib/nself-ci-watch --shell /bin/false "$u"
uid=$(id -u "$u")
[ "$uid" != 0 ] && [ "$uid" != "$(id -u nself-ci)" ]
for job in $(getent passwd | cut -d: -f1 | grep '^nself-ci-job-' || true); do [ "$uid" != "$(id -u "$job")" ]; done
if id -nG "$u" | grep -qwE 'docker|sudo|wheel|adm|nself-ci|nself-ci-job-[0-9]+'; then exit 1; fi
[ ! -L /var/lib/nself-ci-watch ] || exit 1
install -d -o "$u" -g "$u" -m 0700 /var/lib/nself-ci-watch
# Descendants of the role's home are created without root privileges.
sudo -n -u "$u" /bin/mkdir -p /var/lib/nself-ci-watch/logs/watch
`

const darwinAccount = `set -eu
PATH=/usr/bin:/bin:/usr/sbin:/sbin
export PATH
u=nself-ci-watch
if ! dscl . -read /Users/$u IsHidden >/dev/null 2>&1; then
  used=$({ dscl . -list /Users UniqueID; dscl . -list /Groups PrimaryGroupID; } | awk '{print $NF}')
  i=450
  while echo "$used" | grep -qx "$i"; do i=$((i + 1)); done
  [ "$i" -le 499 ]
  dscl . -create /Groups/$u
  dscl . -create /Groups/$u PrimaryGroupID "$i"
  dscl . -create /Users/$u
  dscl . -create /Users/$u UniqueID "$i"
  dscl . -create /Users/$u PrimaryGroupID "$i"
  dscl . -create /Users/$u UserShell /usr/bin/false
  dscl . -create /Users/$u NFSHomeDirectory /var/db/nself-ci-watch
  dscl . -create /Users/$u Password '*'
  dscl . -create /Users/$u IsHidden 1
fi
uid=$(id -u "$u")
[ "$uid" != 0 ] && [ "$uid" != "$(id -u nself-ci)" ]
for job in $(dscl . -list /Users | grep '^nself-ci-job-' || true); do [ "$uid" != "$(id -u "$job")" ]; done
if id -Gn "$u" | grep -qwE 'admin|wheel|nself-ci|nself-ci-job-[0-9]+'; then exit 1; fi
[ ! -L /var/db/nself-ci-watch ] || exit 1
install -d -o "$u" -g "$u" -m 0700 /var/db/nself-ci-watch
# Descendants of the role's home are created without root privileges.
sudo -n -u "$u" /bin/mkdir -p /var/db/nself-ci-watch/logs/watch
`
