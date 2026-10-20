package darwin

// Purpose: the darwin family against a fake Mac: registration, the install sequence, idempotence, slot
// retirement, the no-root and wrong-OS refusals, uninstall and the Xcode warning.
// Inputs:  recorded argv. Constraints: no network or disk; `sh -n` parses each script (syntax only).

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
)

func sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }

func TestDarwinFamilyIsRegistered(t *testing.T) {
	f, ok := bootstrap.Lookup("darwin")
	if !ok || f.Name() != "darwin" {
		t.Fatalf("Lookup(darwin) = %v, %v", f, ok)
	}
	if _, ok = bootstrap.Lookup("debian"); !ok {
		t.Fatal("the Linux families vanished")
	}
	if err := bootstrap.RegisterFamily(&Family{}); bootstrap.ReasonOf(err) != bootstrap.ReasonDuplicate {
		t.Fatalf("a second darwin family must be a duplicate, got %v", err)
	}
	if ref, _, ok, err := bootstrap.Detect(context.Background(), newFake().run); err != nil || !ok || ref.Name() != "darwin" {
		t.Fatalf("Detect on a Darwin node = %v, %v, %v", ref, ok, err)
	}
}

func TestScriptsParseAndAreSafeToSend(t *testing.T) {
	all := map[string]string{
		"users": usersScript(3), "dirs": dirsScript(3), "sudoers": sudoersScript(), "list": listJobUsersScript(),
		"remove": removeSlotScript(2), "agent": agentScript(".up"), "verify": verifyScript(3), "uninstall": uninstallScript(),
		"plist": plistScript(), "bootout": bootoutScript(), "xcode": xcodeProbeScript("/tmp/stub"),
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
		if strings.Contains(s, "sysadminctl") || strings.Contains(s, "<<") {
			t.Errorf("%s: uses sysadminctl or a heredoc", name)
		}
	}
}

func TestUsersAreHiddenRoleAccountsInTheIDRange(t *testing.T) {
	s := usersScript(2)
	for _, want := range []string{
		`UserShell /usr/bin/false`, `NFSHomeDirectory /var/empty`, `IsHidden 1`, `Password '*'`,
		`i=450`, `-le 499`, `dscl . -list /Users UniqueID`, `dscl . -list /Groups PrimaryGroupID`,
		`mk nself-ci`, `for n in 1 2; do mk nself-ci-job-$n; done`, `PrimaryGroupID "$id"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("users script lacks %q:\n%s", want, s)
		}
	}
	for _, bad := range []string{"GroupMembership", "admin", "sysadminctl", "-admin"} {
		if strings.Contains(s, bad) {
			t.Errorf("users script holds %q", bad)
		}
	}
}

func TestSudoersHoldsOneShimRulePerSlotAndNothingElse(t *testing.T) {
	got := Sudoers(3)
	var rules []string
	for _, l := range strings.Split(got, "\n") {
		if l != "" && !strings.HasPrefix(l, "#") {
			rules = append(rules, l)
		}
	}
	if len(rules) != 3 {
		t.Fatalf("want 3 rules, got %q", rules)
	}
	for i, r := range rules {
		want := fmt.Sprintf("nself-ci ALL=(nself-ci-job-%d) NOPASSWD: /opt/nself-ci-agent/bin/nself-ci-agent job-shim", i+1)
		if r != want {
			t.Errorf("rule %d = %q, want %q", i+1, r, want)
		}
		if strings.Contains(r, "(ALL)") || strings.Contains(r, "NOPASSWD:ALL") || strings.Contains(r, "SETENV") {
			t.Errorf("rule %d is broad: %q", i+1, r)
		}
	}
	s := sudoersScript()
	if !strings.Contains(s, "visudo -cf ./.nself-ci-sudoers.stage") || !strings.Contains(s, "visudo -c ") {
		t.Fatal("the drop-in is not validated before and after the install")
	}
	if !strings.Contains(s, "install -o root -g wheel -m 0440 ./.nself-ci-sudoers.stage /etc/sudoers.d/nself-ci") {
		t.Fatalf("the drop-in is not installed 0440 root:wheel:\n%s", s)
	}
}

func TestPlistIsWellFormedRunsAsTheAgentUserAndWaitsForJoin(t *testing.T) {
	p := Plist()
	for _, want := range []string{
		"<string>org.nself.ci-agent</string>", "<key>UserName</key>\n\t<string>nself-ci</string>", "<string>/opt/nself-ci-agent/bin/nself-ci-agent</string>",
		"<string>pull</string>", "<key>RunAtLoad</key>\n\t<true/>", "<key>PathState</key>", "/var/db/nself-ci-agent/pki/enrollment.json",
		"/var/log/nself-ci-agent/agent.log", "NSELF_CI_AGENT_HOME", "<key>HOME</key>\n\t\t<string>/var/db/nself-ci-agent</string>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist lacks %q", want)
		}
	}
	if strings.Contains(p, "<string>root</string>") {
		t.Fatal("the service must not run as root")
	}
	if err := wellFormed(p); err != nil {
		t.Fatalf("plist is not well-formed XML: %v", err)
	}
}

func fullInstall(t *testing.T, slots int, tweak func(*fakeMac)) (*fakeMac, *[]string, error) {
	t.Helper()
	n := newFake()
	if tweak != nil {
		tweak(n)
	}
	o, logs := n.options(t, slots)
	o.AgentBinary = t.TempDir() + "/agent"
	if err := writeFile(o.AgentBinary, "x"); err != nil {
		t.Fatal(err)
	}
	return n, logs, (&Family{}).Install(context.Background(), remote.Target{Dest: "ci@mac"}, o)
}

func TestInstallRunsTheStepsInOrderThroughSudo(t *testing.T) {
	n, logs, err := fullInstall(t, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range *logs {
		names = append(names, strings.SplitN(strings.TrimPrefix(l, "bootstrap "), ":", 2)[0])
	}
	want := "users directories sudoers agent launchd verify"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("steps = %q, want %q", got, want)
	}
	for _, c := range n.calls {
		if len(c) > 3 && c[len(c)-3] == "sh" && (c[0] != "sudo" || c[1] != "-n") {
			t.Fatalf("a script ran without sudo: %v", c[:3])
		}
	}
	if got := n.copies[SudoersStage]; got != Sudoers(2) {
		t.Fatalf("the staged sudoers is %q", got)
	}
	if got := n.copies[PlistStage]; got != Plist() {
		t.Fatalf("the staged plist differs from Plist()")
	}
	if _, ok := n.copies[".nself-ci-agent.upload"]; !ok {
		t.Fatal("the agent binary was not uploaded")
	}
}

func TestInstallAsRootSkipsSudo(t *testing.T) {
	n, _, err := fullInstall(t, 1, func(n *fakeMac) { n.uid = "0" })
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range n.calls {
		if c[0] == "sudo" && len(c) > 1 && c[1] == "-n" && c[len(c)-1] != "true" && !strings.Contains(strings.Join(c, " "), "xcode") {
			t.Fatalf("root used sudo: %v", c[:3])
		}
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	a, _, err := fullInstall(t, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := fullInstall(t, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(a.scripts(), "\x00") != strings.Join(b.scripts(), "\x00") {
		t.Fatal("two installs sent different scripts")
	}
	if !strings.Contains(plistScript(), "cmp -s") || !strings.Contains(plistScript(), `if [ "$loaded" = 0 ]`) {
		t.Fatal("the plist script reloads an unchanged service")
	}
}

func TestInstallRefusals(t *testing.T) {
	cases := []struct {
		name   string
		tweak  func(*fakeMac)
		slots  int
		reason string
	}{
		{"no root", func(n *fakeMac) { n.sudoErr = true }, 2, bootstrap.ReasonNoRoot},
		{"linux node", func(n *fakeMac) { n.os = "Linux" }, 2, bootstrap.ReasonUnsupported},
		{"zero slots", nil, 0, bootstrap.ReasonUsage},
		{"too many slots", nil, MaxSlots + 1, bootstrap.ReasonUsage},
		{"users fail", func(n *fakeMac) { n.failOn = "dscl . -create" }, 2, ReasonUsers},
		{"sudoers fail", func(n *fakeMac) { n.failOn = "visudo -cf" }, 2, ReasonSudoers},
		{"launchd fail", func(n *fakeMac) { n.failOn = "plutil -lint" }, 2, ReasonLaunchd},
		{"verify fail", func(n *fakeMac) { n.failOn = "sudo lists" }, 2, bootstrap.ReasonStep},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := fullInstall(t, c.slots, c.tweak)
			if bootstrap.ReasonOf(err) != c.reason {
				t.Fatalf("reason = %q (%v), want %q", bootstrap.ReasonOf(err), err, c.reason)
			}
			if strings.Contains(fmt.Sprint(err), "boom") && strings.Contains(fmt.Sprint(err), "second line") {
				t.Fatal("the node's raw output reached the error")
			}
		})
	}
}

func TestFewerSlotsRetireOnlyUsersThatOwnNoFile(t *testing.T) {
	n, logs, err := fullInstall(t, 1, func(n *fakeMac) {
		n.users = "nself-ci-job-1\nnself-ci-job-2\nnself-ci-job-3\nnself-ci-job-02\nnself-ci-job-x\n"
		n.kept = map[string]bool{"nself-ci-job-3": true}
	})
	if err != nil {
		t.Fatal(err)
	}
	var retire []string
	for _, s := range n.scripts() {
		if strings.Contains(s, "echo kept; exit 0") {
			retire = append(retire, strings.SplitN(strings.SplitN(s, "u=", 2)[1], "\n", 2)[0])
		}
	}
	if strings.Join(retire, ",") != "nself-ci-job-2,nself-ci-job-3" {
		t.Fatalf("retired %v", retire)
	}
	got := strings.Join(*logs, "|")
	if !strings.Contains(got, "nself-ci-job-2 removed") || !strings.Contains(got, "nself-ci-job-3 kept") {
		t.Fatalf("logs: %s", got)
	}
	if strings.Contains(n.copies[SudoersStage], "job-2") {
		t.Fatal("a retired slot keeps its sudoers rule")
	}
}

func TestSurplusSlotsIgnoresStrangeNames(t *testing.T) {
	got := surplusSlots("nself-ci-job-5 nself-ci-job-02 nself-ci-job-1 nself-ci-job-1000 other nself-ci-job-3 nself-ci-job-5", 2)
	if fmt.Sprint(got) != "[3 5]" {
		t.Fatalf("got %v", got)
	}
}

func TestUninstallRemovesEverythingInOrder(t *testing.T) {
	n := newFake()
	n.uid = "0"
	o, _ := n.options(t, 0)
	if err := (&Family{}).Uninstall(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(n.scripts(), "\n")
	for _, want := range []string{
		"launchctl bootout system/org.nself.ci-agent", "rm -f /Library/LaunchDaemons/org.nself.ci-agent.plist", "rm -f /etc/sudoers.d/nself-ci",
		"dscl . -delete \"/Users/$u\"", "dscl . -delete \"/Groups/$g\"", "/opt/nself-ci-agent", "/var/db/nself-ci-agent", "/var/db/nself-ci-work",
		"/var/db/nself-ci-jobs", "/var/log/nself-ci-agent", "accounts still present",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("uninstall does not touch %q", want)
		}
	}
	if strings.Index(all, "pkill") > strings.Index(all, "dscl . -delete") {
		t.Fatal("accounts are deleted before their processes are killed")
	}
}

func TestXcodeLicenseWarnsAndNeverFails(t *testing.T) {
	_, logs, err := fullInstall(t, 1, func(n *fakeMac) { n.xcode = "license" })
	if err != nil {
		t.Fatalf("an unaccepted license must not fail the install: %v", err)
	}
	last := (*logs)[len(*logs)-1]
	for _, want := range []string{"E669 xcode_license", "sudo xcodebuild -license accept", "without the xcode fact"} {
		if !strings.Contains(last, want) {
			t.Errorf("warning %q lacks %q", last, want)
		}
	}
	_, logs, err = fullInstall(t, 1, func(n *fakeMac) { n.xcode = "absent" })
	if err != nil || strings.Contains(strings.Join(*logs, "\n"), "xcode_license") {
		t.Fatalf("a node without Xcode is not a license problem: %v %v", err, *logs)
	}
	if !strings.Contains(xcodeProbeScript("/tmp/stub"), "'/tmp/stub:/usr/bin") {
		t.Fatal("the stub PATH prefix is not put first")
	}
}

var _ = bootstrap.CodeBootstrap
