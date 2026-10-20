//go:build darwin && macosleg

package macos

// Purpose: the first step of the leg: the real `nself ci nodes add --system` over the loopback sshd, and
// the layout checks on what it created (Epic D20 acceptance 1).
// Constraints: the checks read the system with dscl, stat and `sudo -l`; they change nothing.

import (
	"strconv"
	"strings"
)

func (l *leg) addSystem() {
	out, rc := l.ciCmd("nodes", "add", l.me+"@127.0.0.1", "--system", "--slots", strconv.Itoa(slots), "--host-key", l.hostKey,
		"--agent-binary", l.agent, "--project", "nself-org/plugins", "--name", "macos-leg", "--json")
	if rc != 0 {
		l.t.Fatalf("nodes add --system: rc=%d\n%s", rc, out)
	}
	l.t.Log(out)
}

func (l *leg) layout() {
	t := l.t
	for _, u := range append([]string{agentUser}, jobUser(1), jobUser(2)) {
		for attr, want := range map[string]string{"IsHidden": "1", "UserShell": "/usr/bin/false", "NFSHomeDirectory": "/var/empty"} {
			out, rc := l.sudo("dscl . -read /Users/" + u + " " + attr + " | awk '{print $NF}'")
			if rc != 0 || out != want {
				t.Errorf("%s %s = %q (rc %d), want %q", u, attr, out, rc, want)
			}
		}
		uid, _ := l.sudo("dscl . -read /Users/" + u + " UniqueID | awk '{print $NF}'")
		n, err := strconv.Atoi(uid)
		if err != nil || n < 450 || n > 499 {
			t.Errorf("%s UniqueID %q is outside 450-499", u, uid)
		}
		if groups, _ := l.sh("id -Gn " + u); strings.Contains(" "+groups+" ", " admin ") || strings.Contains(" "+groups+" ", " wheel ") {
			t.Errorf("%s is in a privileged group: %s", u, groups)
		}
	}
	if out, _ := l.sudo("dscl . -list /Users | grep -c '^nself-ci-job-'"); out != strconv.Itoa(slots) {
		t.Errorf("job users: %s, want %d", out, slots)
	}
	for path, want := range map[string]string{
		"/opt/nself-ci-agent": "755 " + agentUser, "/opt/nself-ci-agent/bin": "755 " + agentUser, agentPath: "755 " + agentUser,
		stateDir: "700 " + agentUser, jobsDir: "711 root", jobRoot(1): "700 " + jobUser(1), jobRoot(2): "700 " + jobUser(2),
		"/var/log/nself-ci-agent": "750 " + agentUser, "/etc/sudoers.d/nself-ci": "440 root",
	} {
		if got, _ := l.sudo("stat -f '%Lp %Su' " + path); got != want {
			t.Errorf("%s is %q, want %q", path, got, want)
		}
	}
	list, _ := l.sudo("sudo -l -U " + agentUser)
	if got := strings.Count(list, agentPath+" job-shim"); got != slots {
		t.Errorf("sudo -l lists %d job-shim rules, want %d:\n%s", got, slots, list)
	}
	if strings.Contains(list, "(ALL)") || strings.Contains(list, "NOPASSWD: ALL") {
		t.Errorf("a broad sudo right:\n%s", list)
	}
	if out, rc := l.sudo("visudo -c"); rc != 0 {
		t.Errorf("visudo -c: %s", out)
	}
}
