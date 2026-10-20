//go:build darwin && macosleg

package macos

// Purpose: the family-level steps of the leg that need no ssh (the add step already proved the ssh path):
// slot changes (more slots add only the missing user, fewer retire a user only when it owns no file,
// ids stay put) and the Xcode license warning (E669 xcode_license, with a stub xcodebuild in the job
// user's PATH, and the install still succeeds).
// Constraints: the node is this Mac, so the family's Run and Copy seams run locally; the ssh user's home
// is the working directory of every script, as it is over ssh.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap/darwin"
)

// options binds the family to this Mac: Run executes locally in the user's home, Copy is a file copy.
func (l *leg) options(n int, logs *[]string) bootstrap.Options {
	home, err := os.UserHomeDir()
	if err != nil {
		l.t.Fatal(err)
	}
	return bootstrap.Options{Slots: n, AgentBinary: l.agent,
		Run: func(ctx context.Context, argv ...string) (string, error) {
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
			cmd.Dir = home
			out, err := cmd.CombinedOutput()
			return string(out), err
		},
		Copy: func(_ context.Context, local, remotePath string) error {
			b, err := os.ReadFile(local)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(home, remotePath), b, 0o600)
		},
		Logf: func(f string, a ...any) {
			line := strings.TrimSpace(strings.Join(strings.Fields(sprint(f, a...)), " "))
			*logs = append(*logs, line)
			l.t.Log(line)
		}}
}

func (l *leg) install(n int, f *darwin.Family) []string {
	l.t.Helper()
	var logs []string
	if err := f.Install(context.Background(), remote.Target{Dest: l.me + "@127.0.0.1"}, l.options(n, &logs)); err != nil {
		l.t.Fatalf("install with %d slots: %v", n, err)
	}
	return logs
}

func (l *leg) uid(user string) string {
	out, _ := l.sudo("dscl . -read /Users/" + user + " UniqueID 2>/dev/null | awk '{print $NF}'")
	return out
}

func (l *leg) rules() int {
	out, _ := l.sudo("sudo -l -U " + agentUser)
	return strings.Count(out, agentPath+" job-shim")
}

func (l *leg) slotChanges() {
	t := l.t
	f := &darwin.Family{}
	u1, u2 := l.uid(jobUser(1)), l.uid(jobUser(2))
	l.install(slots, f) // idempotent: nothing changes
	if l.uid(jobUser(1)) != u1 || l.uid(jobUser(2)) != u2 || l.rules() != slots {
		t.Fatalf("a second install changed ids or rules: %s %s -> %s %s, %d rules", u1, u2, l.uid(jobUser(1)), l.uid(jobUser(2)), l.rules())
	}
	l.install(3, f) // more slots: only job 3 and its rule are added
	if l.uid(jobUser(3)) == "" || l.rules() != 3 || l.uid(jobUser(1)) != u1 {
		t.Fatalf("after 2->3: job-3 uid %q, %d rules", l.uid(jobUser(3)), l.rules())
	}
	l.mustSudo("sudo -u " + jobUser(3) + " sh -c 'echo keep > " + jobRoot(3) + "/keep'")
	l.install(1, f) // fewer slots: job 2 owns no file and goes; job 3 owns one, so its user stays without a rule
	if l.uid(jobUser(2)) != "" || l.uid(jobUser(3)) == "" || l.rules() != 1 {
		t.Fatalf("after 3->1: job-2 uid %q, job-3 uid %q, %d rules", l.uid(jobUser(2)), l.uid(jobUser(3)), l.rules())
	}
	l.mustSudo("rm -f " + jobRoot(3) + "/keep")
	l.install(slots, f) // back to two: job 2 returns, job 3 stays inert (still no rule)
	if l.uid(jobUser(2)) == "" || l.rules() != slots || l.uid(jobUser(1)) != u1 {
		t.Fatalf("after 1->2: job-2 uid %q, %d rules, job-1 uid %q", l.uid(jobUser(2)), l.rules(), l.uid(jobUser(1)))
	}
	t.Log("slot users: " + strconv.Itoa(len(strings.Fields(l.mustSudo("dscl . -list /Users | grep '^nself-ci-job-'")))))
}

func (l *leg) xcodeLicense() {
	t := l.t
	stub := filepath.Join(l.stub, "xcodebuild")
	script := "#!/bin/sh\ncase \"$*\" in\n  \"-license check\") echo 'Agreeing to the Xcode license requires admin privileges' >&2; exit 69 ;;\nesac\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(stub)
	logs := l.install(slots, &darwin.Family{XcodePathPrefix: l.stub})
	last := logs[len(logs)-1]
	if !strings.Contains(last, "E669 xcode_license") || !strings.Contains(last, "sudo xcodebuild -license accept") {
		t.Errorf("no xcode_license warning with the one-line fix; last log line: %q", last)
	}
}
