package bootstrap

// Purpose: the scripts and the Install sequence of the Linux families against a fake node.
// Inputs:  recorded argv. Constraints: no network or disk; `sh -n` parses each script (syntax only).

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

type fakeNode struct {
	calls   [][]string
	uid     string
	sudoErr bool
	users   string // reply of the list-users script
	failOn  string // a step script fragment that fails
	kept    map[string]bool
}

func (n *fakeNode) run(_ context.Context, argv ...string) (string, error) {
	n.calls = append(n.calls, argv)
	joined := strings.Join(argv, " ")
	switch {
	case joined == "uname -s":
		return "Linux", nil
	case joined == "id -u":
		return n.uid, nil
	case joined == "sudo -n true":
		if n.sudoErr {
			return "", errors.New("sudo: a password is required")
		}
		return "", nil
	}
	script := argv[len(argv)-1]
	if n.failOn != "" && strings.Contains(script, n.failOn) {
		return "boom\nsecond line", errors.New("exit status 1")
	}
	if strings.Contains(script, "getent passwd | cut") && strings.Contains(script, "|| true") && !strings.Contains(script, "userdel") {
		return n.users, nil
	}
	if strings.Contains(script, "echo kept; exit 0") {
		for u := range n.kept {
			if strings.Contains(script, "u="+u+"\n") {
				return "kept", nil
			}
		}
		return "removed", nil
	}
	return "", nil
}

func (n *fakeNode) scripts() []string {
	var out []string
	for _, c := range n.calls {
		if len(c) >= 3 && c[len(c)-3] == "sh" {
			out = append(out, c[len(c)-1])
		}
	}
	return out
}

func apt() *pkgFamily { f, _ := Lookup("debian"); return f.(*pkgFamily) }

func TestScriptsParseAndAreSafeToSend(t *testing.T) {
	all := map[string]string{
		"packages": packagesScript(apt().install, apt().pkgs), "users": usersScript(3), "dirs": dirsScript(3),
		"sudoers": sudoersScript(3), "list": listJobUsersScript(), "remove": removeSlotScript(2), "agent": agentScript(".up"),
		"verify": verifyScript(3), "uninstall": uninstallScript(), "unit": unitScript(),
	}
	for name, s := range all {
		if strings.Contains(s, `\`) {
			t.Errorf("%s: holds a backslash, which RunArgv refuses", name)
		}
		cmd := exec.Command("sh", "-n")
		cmd.Stdin = strings.NewReader(s)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: sh -n: %v\n%s", name, err, out)
		}
		if !strings.HasPrefix(s, "set -eu\n") {
			t.Errorf("%s: does not start with set -eu", name)
		}
		if strings.Contains(s, "docker") && name != "verify" { // verify greps for the group to prove it is absent
			t.Errorf("%s: mentions docker (no docker group, ever)", name)
		}
	}
}

func TestSudoersRuleNamesTheShimAndNothingElse(t *testing.T) {
	s := sudoersScript(2)
	want := `echo "nself-ci ALL=(nself-ci-job-$n) NOPASSWD: /opt/nself-ci-agent/bin/nself-ci-agent job-shim"`
	if strings.Count(s, "NOPASSWD") != 1 || !strings.Contains(s, want) {
		t.Fatalf("sudoers script:\n%s", s)
	}
	if !strings.Contains(s, "visudo -cf") || !strings.Contains(s, "visudo -c ") {
		t.Fatal("rules are not validated with visudo")
	}
	if strings.Contains(s, "ALL=(ALL)") || strings.Contains(s, "NOPASSWD:ALL") {
		t.Fatal("a broad rule")
	}
	if !strings.Contains(sudoersScript(3), "for n in 1 2 3;") {
		t.Fatal("slot list")
	}
}

func TestUnitKeepsSudoWorking(t *testing.T) {
	u := Unit()
	for _, want := range []string{"User=nself-ci", "NSELF_CI_AGENT_HOME=/var/lib/nself-ci-agent", "ExecStart=/opt/nself-ci-agent/bin/nself-ci-agent pull", "Delegate=yes"} {
		if !strings.Contains(u, want) {
			t.Errorf("unit lacks %q", want)
		}
	}
	if strings.Contains(u, "NoNewPrivileges") {
		t.Error("NoNewPrivileges would break the sudo job shim")
	}
}

func TestInstallSequenceAsRootAndAsSudoUser(t *testing.T) {
	for _, uid := range []string{"0", "1000"} {
		n := &fakeNode{uid: uid}
		err := apt().Install(context.Background(), remote.Target{Dest: "u@h"}, Options{Slots: 2, Run: n.run})
		if err != nil {
			t.Fatalf("uid %s: %v", uid, err)
		}
		sc := n.scripts()
		for _, c := range n.calls {
			isSh := len(c) >= 3 && c[len(c)-3] == "sh"
			elevated := c[0] == "sudo"
			if isSh && (uid != "0") != elevated {
				t.Errorf("uid %s: script run with elevation=%v: %v", uid, elevated, c[:2])
			}
		}
		if !strings.Contains(strings.Join(sc, "\n"), "useradd --system --user-group") {
			t.Error("no users step")
		}
	}
}

func TestInstallRefusesWithoutRoot(t *testing.T) {
	n := &fakeNode{uid: "1000", sudoErr: true}
	err := apt().Install(context.Background(), remote.Target{Dest: "u@h"}, Options{Slots: 1, Run: n.run})
	if ReasonOf(err) != ReasonNoRoot {
		t.Fatalf("err = %v", err)
	}
	if len(n.scripts()) != 0 {
		t.Fatal("a script ran without root")
	}
}

func TestInstallRejectsBadSlotsAndStopsAtAFailedStep(t *testing.T) {
	for _, slots := range []int{0, -1, MaxSlots + 1} {
		if err := apt().Install(context.Background(), remote.Target{Dest: "u@h"}, Options{Slots: slots, Run: (&fakeNode{uid: "0"}).run}); ReasonOf(err) != ReasonUsage {
			t.Errorf("slots %d: %v", slots, err)
		}
	}
	n := &fakeNode{uid: "0", failOn: "useradd"}
	err := apt().Install(context.Background(), remote.Target{Dest: "u@h"}, Options{Slots: 1, Run: n.run})
	if ReasonOf(err) != ReasonStep || strings.Contains(err.Error(), "second line") {
		t.Fatalf("err = %v", err)
	}
	for _, s := range n.scripts() {
		if strings.Contains(s, "visudo") {
			t.Fatal("sudoers ran after the users step failed")
		}
	}
}

func TestSurplusSlots(t *testing.T) {
	got := surplusSlots("nself-ci-job-3\nnself-ci-job-1\nnself-ci-job-10\nnself-ci-job-02\nnself-ci-job-0\nroot\nnself-ci-job-x\nnself-ci-job-3\nnself-ci-job-1000", 2)
	if len(got) != 2 || got[0] != 3 || got[1] != 10 {
		t.Fatalf("surplus = %v", got)
	}
	if len(surplusSlots("nself-ci-job-1\nnself-ci-job-2", 2)) != 0 {
		t.Fatal("slots in range retired")
	}
}

func TestInstallRetiresSurplusSlots(t *testing.T) {
	n := &fakeNode{uid: "0", users: "nself-ci-job-1\nnself-ci-job-2\nnself-ci-job-3", kept: map[string]bool{"nself-ci-job-3": true}}
	var logs []string
	err := apt().Install(context.Background(), remote.Target{Dest: "u@h"}, Options{Slots: 2, Run: n.run, Logf: func(f string, a ...any) { logs = append(logs, f) }})
	if err != nil {
		t.Fatal(err)
	}
	retired := 0
	for _, s := range n.scripts() {
		if strings.Contains(s, "u=nself-ci-job-3\n") {
			retired++
			if !strings.Contains(s, "rm -f /etc/sudoers.d/nself-ci-job-3") || !strings.Contains(s, `find "$d" -xdev -user`) {
				t.Errorf("retire script:\n%s", s)
			}
		}
		if strings.Contains(s, "u=nself-ci-job-2\n") || strings.Contains(s, "u=nself-ci-job-1\n") {
			t.Error("a live slot was retired")
		}
	}
	if retired != 1 {
		t.Fatalf("retired %d", retired)
	}
}
