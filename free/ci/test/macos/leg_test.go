//go:build darwin && macosleg

package macos

// Purpose: the macOS leg (P7-NODE-31, Epic D20): the rig and the order of the steps. The Mac that runs
// the test is the node: a loopback sshd on port 2222 with a throwaway key lets the real
// `nself ci nodes add --system` install the system layout over ssh, the other files check it.
// Inputs:  NSELF_MACOS_LEG_SSH_CONFIG (an ssh_config with `Host 127.0.0.1`, port 2222 and the throwaway
//          identity), NSELF_MACOS_LEG_HOSTKEY (the sshd host key as SHA256:...), NSELF_MACOS_LEG_AGENT
//          (a darwin nself-ci-agent), NSELF_MACOS_LEG_CI (the nself ci binary), NSELF_MACOS_LEG_USER
//          (the ssh user, an admin with passwordless sudo), NSELF_MACOS_LEG_STUB (an empty directory the
//          Xcode-license step may fill with a stub xcodebuild; world-readable).
// Outputs: pass or fail; the layout is removed at the end by `nself-ci-agent uninstall --system`.
// Constraints: a missing prerequisite is t.Fatal, never t.Skip; the test creates users and sudo rules, so
//          it runs only on a runner or on an operator's own Mac, started by hand with these variables.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	agentUser = "nself-ci"
	agentPath = "/opt/nself-ci-agent/bin/nself-ci-agent"
	stateDir  = "/var/db/nself-ci-agent"
	jobsDir   = "/var/db/nself-ci-jobs"
	plistPath = "/Library/LaunchDaemons/org.nself.ci-agent.plist"
	// trusted is the root-owned copy of the agent: the only binary a root shell may run (uninstall).
	trusted = "/Library/PrivilegedHelperTools/org.nself.ci-agent"
	label   = "org.nself.ci-agent"
	slots   = 2
)

func jobUser(n int) string { return fmt.Sprintf("nself-ci-job-%d", n) }
func jobRoot(n int) string { return fmt.Sprintf("%s/%d", jobsDir, n) }

// leg is the rig: where the binaries are and how to reach the node.
type leg struct {
	t                                 *testing.T
	sshConfig, hostKey, agent, ci, me string
	stub, home                        string
}

func need(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Fatalf("%s is not set: the macOS leg needs the loopback sshd and the binaries the workflow builds", name)
	}
	return v
}

func newLeg(t *testing.T) *leg {
	t.Helper()
	l := &leg{t: t, sshConfig: need(t, "NSELF_MACOS_LEG_SSH_CONFIG"), hostKey: need(t, "NSELF_MACOS_LEG_HOSTKEY"),
		agent: need(t, "NSELF_MACOS_LEG_AGENT"), ci: need(t, "NSELF_MACOS_LEG_CI"), me: need(t, "NSELF_MACOS_LEG_USER"),
		stub: need(t, "NSELF_MACOS_LEG_STUB"), home: t.TempDir()}
	// ssh and scp read the throwaway config (port 2222, key, no known_hosts) through two wrappers on PATH.
	bin := filepath.Join(l.home, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"ssh", "scp"} {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		script := "#!/bin/sh\nexec " + real + " -F " + l.sshConfig + " \"$@\"\n"
		if err = os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// `nodes add` reads the project's deploy inventory through `nself deploy targets --json` (the production
	// boundary, P7-NODE-14) and refuses a node when it cannot. A runner has no nself CLI and no deploy host,
	// so a stub answers the two read-only calls with the contract's shape and an empty inventory.
	stub := "#!/bin/sh\ncase \"$*\" in\n" +
		"  \"help --json deploy targets\") echo '{\"data\":{\"commands\":[{\"path\":\"deploy targets\",\"side_effect\":\"read\"}]}}' ;;\n" +
		"  \"deploy targets --json\") echo '{\"command\":\"deploy targets\",\"data\":{\"targets\":[]}}' ;;\n" +
		"  *) exit 2 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "nself"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return l
}

// sh runs a script as the current user; sudo runs it as root. Both return the combined output and the
// exit status; mustSudo fails the test on a non-zero status.
func (l *leg) sh(script string) (string, int) { return l.exec("/bin/sh", "-c", script) }
func (l *leg) sudo(script string) (string, int) {
	return l.exec("/usr/bin/sudo", "-n", "/bin/sh", "-c", script)
}

func (l *leg) exec(prog string, args ...string) (string, int) {
	l.t.Helper()
	cmd := exec.Command(prog, args...)
	out, err := cmd.CombinedOutput()
	rc := 0
	if err != nil {
		rc = 1
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		}
	}
	return strings.TrimSpace(string(out)), rc
}

func (l *leg) mustSudo(script string) string {
	l.t.Helper()
	out, rc := l.sudo(script)
	if rc != 0 {
		l.t.Fatalf("sudo sh -c %q: rc=%d\n%s", script, rc, out)
	}
	return out
}

// ciCmd runs the nself ci binary in v1.5 mode with its own home.
func (l *leg) ciCmd(args ...string) (string, int) {
	l.t.Helper()
	cmd := exec.Command(l.ci, args...)
	cmd.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_CI_HOME="+filepath.Join(l.home, "ci"))
	out, err := cmd.CombinedOutput()
	rc := 0
	if err != nil {
		rc = 1
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		}
	}
	return strings.TrimSpace(string(out)), rc
}

func TestMacOSNode(t *testing.T) {
	l := newLeg(t)
	steps := []struct {
		name string
		run  func(*leg)
	}{
		{"add --system", (*leg).addSystem}, {"layout", (*leg).layout}, {"isolation", (*leg).isolation},
		{"job shim", (*leg).shim}, {"service", (*leg).service}, {"jobs as the job user", (*leg).jobs},
		{"slots", (*leg).slotChanges}, {"xcode license", (*leg).xcodeLicense}, {"upgrade swap", (*leg).upgradeSwap},
		{"hostile agent", (*leg).hostileAgent},
		{"uninstall", (*leg).uninstall},
	}
	// A later step does not depend on an earlier one passing (only on the add), so every step runs and one
	// run reports all the failures; the uninstall is last and the cleanup below also runs when add fails.
	for _, s := range steps {
		ok := t.Run(s.name, func(t *testing.T) { l.t = t; s.run(l) })
		if !ok && s.name == "add --system" {
			// Leave nothing behind on a runner that is reused (an operator's own Mac): best-effort removal.
			l.t = t
			l.sudo(trusted + " uninstall --system")
			t.FailNow()
		}
	}
}

func sprint(f string, a ...any) string { return fmt.Sprintf(f, a...) }
