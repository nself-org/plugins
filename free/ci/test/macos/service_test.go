//go:build darwin && macosleg

package macos

// Purpose: pull mode on the leg (Epic D20 acceptance 4, 5): the LaunchDaemon is installed and loaded as
// `nself-ci`, it waits for `join` (no crash loop on a node that has not enrolled), starts when the
// enrollment file appears, and `nodes upgrade` can swap the agent binary without root while a job user
// cannot touch it; `nself-ci-agent uninstall --system` then leaves nothing (acceptance 5, 8).
// Constraints: the enrollment file is a placeholder written as nself-ci (the real join needs a
// coordinator); the pull command rejects it, which is enough to see launchd start the job.

import (
	"strings"
	"time"
)

func (l *leg) service() {
	t := l.t
	if out, rc := l.sudo("plutil -lint " + plistPath); rc != 0 {
		t.Fatalf("plutil -lint: %s", out)
	}
	if got, _ := l.sudo("plutil -extract UserName raw -o - " + plistPath); got != agentUser {
		t.Errorf("the service runs as %q, want %s", got, agentUser)
	}
	if got, _ := l.sudo("stat -f '%Lp %Su' " + plistPath); got != "644 root" {
		t.Errorf("plist is %q, want 644 root", got)
	}
	out, rc := l.sudo("launchctl print system/" + label)
	if rc != 0 {
		t.Fatalf("the service is not loaded: %s", out)
	}
	// Not enrolled: launchd ran `pull` once at load (RunAtLoad), then must leave it alone. Wait out the
	// first run and two throttle intervals, and the run count must not move.
	time.Sleep(15 * time.Second)
	first, _ := l.sudo("launchctl print system/" + label)
	runs := launchRuns(first)
	time.Sleep(25 * time.Second)
	if later, _ := l.sudo("launchctl print system/" + label); launchRuns(later) != runs {
		t.Fatalf("the service restarts without an enrollment (runs %d -> %d): a crash loop on a node that has not joined", runs, launchRuns(later))
	}
	l.mustSudo("sudo -u " + agentUser + " sh -c 'mkdir -p " + stateDir + "/pki && echo {} > " + stateDir + "/pki/enrollment.json'")
	defer l.mustSudo("rm -f " + stateDir + "/pki/enrollment.json")
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		now, _ := l.sudo("launchctl print system/" + label)
		if launchRuns(now) > runs {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("launchd did not start the service after the enrollment file appeared (runs stayed %d)", runs)
}

// launchRuns reads the `runs = N` line of `launchctl print`.
func launchRuns(out string) int {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "runs = "); ok {
			n := 0
			for _, c := range rest {
				if c < '0' || c > '9' {
					break
				}
				n = n*10 + int(c-'0')
			}
			return n
		}
	}
	return 0
}

func (l *leg) upgradeSwap() {
	t := l.t
	next := agentPath + ".next"
	l.mustSudo("sudo -u " + agentUser + " sh -c 'cp " + agentPath + " " + next + " && mv -f " + next + " " + agentPath + "'")
	if out, rc := l.sudo(agentPath + " version"); rc != 0 || !strings.HasPrefix(out, "nself-ci-agent ") {
		t.Errorf("the swapped binary does not run: rc=%d %q", rc, out)
	}
	// The agent can swap files inside bin, but not the bin directory itself (its parent belongs to root).
	if out, rc := l.sudo("sudo -u " + agentUser + " mv /opt/nself-ci-agent/bin /opt/nself-ci-agent/bin.old 2>&1"); rc == 0 {
		t.Errorf("the agent user renamed its own bin directory: %s", out)
	}
	if out, rc := l.sudo("sudo -u " + jobUser(1) + " sh -c 'cp " + agentPath + " " + next + "' 2>&1"); rc == 0 {
		t.Errorf("a job user could write into the agent directory: %s", out)
	}
	if out, rc := l.sudo("sudo -u " + jobUser(1) + " " + agentPath + " version"); rc != 0 || !strings.HasPrefix(out, "nself-ci-agent ") {
		t.Errorf("a job user cannot run the agent binary: rc=%d %q", rc, out)
	}
}

func (l *leg) uninstall() {
	t := l.t
	// The installed agent belongs to nself-ci: a root shell must refuse to run it, and run the root-owned copy.
	if out, rc := l.sudo(agentPath + " uninstall --system 2>&1"); rc == 0 || !strings.Contains(out, "refusing to run as root") {
		t.Fatalf("uninstall ran as root from the agent-owned binary: rc=%d\n%s", rc, out)
	}
	if got, _ := l.sudo("stat -f '%Lp %Su' " + trusted); got != "755 root" {
		t.Fatalf("the trusted copy is %q, want 755 root", got)
	}
	uid1, _ := l.sudo("dscl . -read /Users/" + jobUser(1) + " UniqueID | awk '{print $NF}'")
	if out, rc := l.sudo(trusted + " uninstall --system 2>&1"); rc != 0 {
		t.Fatalf("uninstall --system: rc=%d\n%s", rc, out)
	}
	for _, gone := range []string{"/opt/nself-ci-agent", stateDir, jobsDir, "/var/db/nself-ci-work", "/var/log/nself-ci-agent", "/etc/sudoers.d/nself-ci", plistPath, trusted} {
		if _, rc := l.sudo("test -e " + gone); rc == 0 {
			t.Errorf("%s survived uninstall", gone)
		}
	}
	if out, _ := l.sudo("dscl . -list /Users | grep -c '^nself-ci' || true"); out != "0" {
		t.Errorf("%s nself-ci users survived uninstall", out)
	}
	if out, _ := l.sudo("dscl . -list /Groups | grep -c '^nself-ci' || true"); out != "0" {
		t.Errorf("%s nself-ci groups survived uninstall", out)
	}
	if uid1 != "" {
		if left, _ := l.sudo("find /private/var/folders /private/var/tmp /private/tmp /Users /Library -xdev -uid " + uid1 + " 2>/dev/null | head -n 3"); left != "" {
			t.Errorf("files owned by the removed uid %s survived uninstall:\n%s", uid1, left)
		}
	}
	if _, rc := l.sudo("launchctl print system/" + label); rc == 0 {
		t.Error("the service is still loaded after uninstall")
	}
	if _, rc := l.sudo("visudo -c"); rc != 0 {
		t.Error("sudoers does not validate after uninstall")
	}
}
