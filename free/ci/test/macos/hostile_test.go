//go:build darwin && macosleg

package macos

// Purpose: a compromised agent user must not turn a routine `nodes add --system` re-run into root
// (adversarial review of P7-NODE-31). As `nself-ci` the test replants the agent binary as a symlink to a
// root-owned decoy, hard-links the decoy into the state directory and tries to swap the log file for a
// symlink and to leave a symlink on the upload name; then the install runs again as root. Every decoy must
// keep its owner, mode and content, the agent binary must be a regular file again, and job users must not
// be able to use cron.
// Constraints: the decoy is a throwaway file under /var/tmp; nothing here touches a real root file.

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap/darwin"
)

const decoy = "/var/tmp/nself-ci-decoy"

func (l *leg) hostileAgent() {
	t := l.t
	l.mustSudo("printf decoy > " + decoy + " && chown root:wheel " + decoy + " && chmod 0644 " + decoy)
	defer l.sudo("rm -f " + decoy + " " + stateDir + "/hard")
	// What a compromised agent can still do inside the directories it owns.
	linked := false
	if out, rc := l.sudo("sudo -u " + agentUser + " ln " + decoy + " " + stateDir + "/hard 2>&1"); rc == 0 {
		linked = true
		if n, _ := l.sudo("stat -f %l " + decoy); n != "2" {
			t.Fatalf("the hard link exists but the decoy has %s links", n)
		}
	} else {
		t.Logf("the OS refused the agent's hard link to a root file (fine): %s", out)
	}
	l.mustSudo("sudo -u " + agentUser + " sh -c 'rm -f /opt/nself-ci-agent/bin/nself-ci-agent /opt/nself-ci-agent/bin/.upload.new && ln -s " + decoy + " /opt/nself-ci-agent/bin/nself-ci-agent && ln -s " + decoy + " /opt/nself-ci-agent/bin/.upload.new'")
	// What it must not be able to do: take over the name launchd writes its log to.
	if out, rc := l.sudo("sudo -u " + agentUser + " sh -c 'rm -f /var/log/nself-ci-agent/agent.log; ln -s " + decoy + " /var/log/nself-ci-agent/agent.log' 2>&1"); rc == 0 {
		t.Errorf("the agent user replaced agent.log in the root-owned log directory: %s", out)
	}
	l.install(slots, &darwin.Family{})
	if got, _ := l.sudo("stat -f '%Lp %Su %Sg' " + decoy); got != "644 root wheel" {
		t.Fatalf("the re-run changed the decoy to %q: root followed a link the agent planted", got)
	}
	t.Logf("hard link planted: %v", linked)
	if got, _ := l.sudo("cat " + decoy); got != "decoy" {
		t.Fatalf("the decoy now reads %q: root or the agent wrote through a planted link", got)
	}
	if _, rc := l.sudo("test -f " + agentPath + " && ! test -L " + agentPath); rc != 0 {
		t.Error("the agent binary is still the planted symlink after the re-run")
	}
	if got, _ := l.sudo("stat -f '%Lp %Su' " + trusted); got != "755 root" {
		t.Errorf("the trusted copy is %q after the re-run", got)
	}
	// cron: a hostile job must not leave a crontab behind.
	for _, u := range []string{jobUser(1), agentUser} {
		out, rc := l.sudo("sudo -u " + u + " sh -c 'echo \"@daily /usr/bin/true\" | crontab - 2>&1'")
		if rc == 0 {
			l.sudo("sudo -u " + u + " crontab -r")
			t.Errorf("%s can install a crontab: %s", u, strings.TrimSpace(out))
		} else if !strings.Contains(out, "not allowed") {
			t.Errorf("%s: crontab failed for a reason other than the cron policy: %s", u, strings.TrimSpace(out))
		}
	}
}
