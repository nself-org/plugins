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
	if got, _ := l.sudo("stat -f '%Lp %Su' " + plistPath); got != "644 root" {
		t.Errorf("plist is %q, want 644 root", got)
	}
	out, rc := l.sudo("launchctl print system/" + label)
	if rc != 0 {
		t.Fatalf("the service is not loaded: %s", out)
	}
	runs := launchRuns(out)
	l.mustSudo("sudo -u " + agentUser + " sh -c 'mkdir -p " + stateDir + "/pki && echo {} > " + stateDir + "/pki/enrollment.json'")
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		now, _ := l.sudo("launchctl print system/" + label)
		if launchRuns(now) > runs {
			l.mustSudo("rm -f " + stateDir + "/pki/enrollment.json")
			return
		}
		time.Sleep(2 * time.Second)
	}
	l.mustSudo("rm -f " + stateDir + "/pki/enrollment.json")
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
	if out, rc := l.sudo("sudo -u " + jobUser(1) + " sh -c 'cp " + agentPath + " " + next + "' 2>&1"); rc == 0 {
		t.Errorf("a job user could write into the agent directory: %s", out)
	}
	if out, rc := l.sudo("sudo -u " + jobUser(1) + " " + agentPath + " version"); rc != 0 || !strings.HasPrefix(out, "nself-ci-agent ") {
		t.Errorf("a job user cannot run the agent binary: rc=%d %q", rc, out)
	}
}

func (l *leg) uninstall() {
	t := l.t
	if out, rc := l.sudo(agentPath + " uninstall --system 2>&1"); rc != 0 {
		t.Fatalf("uninstall --system: rc=%d\n%s", rc, out)
	}
	for _, gone := range []string{"/opt/nself-ci-agent", stateDir, jobsDir, "/var/db/nself-ci-work", "/var/log/nself-ci-agent", "/etc/sudoers.d/nself-ci", plistPath} {
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
	if _, rc := l.sudo("launchctl print system/" + label); rc == 0 {
		t.Error("the service is still loaded after uninstall")
	}
	if _, rc := l.sudo("visudo -c"); rc != 0 {
		t.Error("sudoers does not validate after uninstall")
	}
}
