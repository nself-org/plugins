//go:build darwin && macosleg

package macos

// Purpose: the separation checks of the leg (Epic D20 acceptance 2): the agent creates its node key and
// age identity as `nself-ci`, no job user can read them (EACCES), job users cannot read each other's job
// roots, and the sudoers rule starts the job shim as a job user and as nothing else.
// Constraints: the agent runs as nself-ci with a closed stdin, so `serve --stdio` creates its keys and
// exits; every denial is checked by its Permission denied text, not by exit status alone.

import "strings"

func (l *leg) isolation() {
	t := l.t
	l.mustSudo("sudo -u " + agentUser + " env NSELF_CI_AGENT_HOME=" + stateDir + " NSELF_CI_AGENT_WORK=/var/db/nself-ci-work " + agentPath + " serve --stdio </dev/null || true")
	files := strings.Fields(l.mustSudo("ls " + stateDir))
	if len(files) == 0 {
		t.Fatalf("the agent created no key material in %s", stateDir)
	}
	for _, f := range files {
		path := stateDir + "/" + f
		mode := strings.Fields(l.mustSudo("stat -f '%Lp %Su' " + path))
		if len(mode) != 2 || mode[1] != agentUser || len(mode[0]) != 3 || mode[0][1:] != "00" {
			t.Errorf("%s is %v: not private to the agent user", path, mode)
		}
		for _, u := range []string{jobUser(1), jobUser(2)} {
			if out, rc := l.sudo("sudo -u " + u + " cat " + path + " 2>&1"); rc == 0 || !strings.Contains(out, "Permission denied") {
				t.Errorf("%s read %s: rc=%d %q (want EACCES)", u, path, rc, out)
			}
		}
	}
	l.mustSudo("sudo -u " + jobUser(2) + " sh -c 'echo job2-secret > " + jobRoot(2) + "/file'")
	if out, rc := l.sudo("sudo -u " + jobUser(1) + " cat " + jobRoot(2) + "/file 2>&1"); rc == 0 || !strings.Contains(out, "Permission denied") {
		t.Errorf("job-1 read job-2's job root: rc=%d %q", rc, out)
	}
	if out, rc := l.sudo("sudo -u " + jobUser(1) + " ls " + jobRoot(2) + " 2>&1"); rc == 0 || !strings.Contains(out, "Permission denied") {
		t.Errorf("job-1 listed job-2's job root: rc=%d %q", rc, out)
	}
	l.mustSudo("rm -f " + jobRoot(2) + "/file")
}

// asAgent is the command that runs the shim as job user n the way the agent does: nself-ci calls sudo.
func asAgent(n int) string {
	return "sudo -u " + agentUser + " sudo -n -u " + jobUser(n) + " " + agentPath + " job-shim"
}

func (l *leg) shim() {
	t := l.t
	req := `printf '%s\n' '{"argv":["/usr/bin/id","-un"]}' | ` + asAgent(1)
	if out, rc := l.sudo(req + " 2>&1"); rc != 0 || out != jobUser(1) {
		t.Errorf("job-shim as job-1: rc=%d %q", rc, out)
	}
	for _, denied := range []string{
		"sudo -u " + agentUser + " sudo -n -u " + jobUser(1) + " /usr/bin/true",
		"sudo -u " + agentUser + " sudo -n -u root /usr/bin/true",
		"sudo -u " + agentUser + " sudo -n -u " + jobUser(1) + " " + agentPath + " version",
		"sudo -u " + agentUser + " sudo -n -u " + jobUser(9) + " " + agentPath + " job-shim",
		"sudo -u " + jobUser(1) + " sudo -n -u " + jobUser(2) + " " + agentPath + " job-shim",
		"sudo -u " + agentUser + " " + agentPath + " job-shim </dev/null",
	} {
		if out, rc := l.sudo(denied + " 2>&1"); rc == 0 {
			t.Errorf("the rule allowed: %s\n%s", denied, out)
		}
	}
}
