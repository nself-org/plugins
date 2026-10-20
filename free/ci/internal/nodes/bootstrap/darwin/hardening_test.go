package darwin

// Purpose: the review-round hardening of the darwin scripts, one test per defect class so a regression
// names itself: partial accounts, leftover groups, shared ids, a root shell running the agent-owned
// binary, per-user system folders blocking slot retirement, sudoers include detection and the Xcode
// probe's exit code.
// Inputs:  script text. Constraints: no node, no disk; `sh -n` parsing lives in family_test.go.

import (
	"strings"
	"testing"
)

func mustContain(t *testing.T, name, script string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(script, w) {
			t.Errorf("%s lacks %q:\n%s", name, w, script)
		}
	}
}

func TestAnAccountIsCompleteOnlyWhenIsHiddenIsWritten(t *testing.T) {
	s := usersScript(1)
	mustContain(t, "users", s,
		`if dscl . -read "/Users/$u" IsHidden >/dev/null 2>&1; then return 0; fi`,
		`dscl . -delete "/Users/$u"`, `dscl . -delete "/Groups/$u"`)
	if strings.Index(s, `IsHidden 1`) < strings.Index(s, `NFSHomeDirectory`) || strings.Index(s, `IsHidden 1`) < strings.Index(s, `UniqueID "$id"`) {
		t.Fatal("IsHidden must be the last attribute written: it is the commit marker of a complete account")
	}
	if strings.Contains(s, `PrimaryGroupID 2>/dev/null`) {
		t.Fatal("an existing group's id is reused: it can collide with another account's uid")
	}
}

func TestVerifyChecksTheRangeAndTheUniquenessOfEveryId(t *testing.T) {
	v := verifyScript(2)
	mustContain(t, "verify", v,
		`{ [ "$uid" -ge 450 ] && [ "$uid" -le 499 ]; } || {`,
		`awk -v id="$uid" '$2 == id'`, `awk -v id="$gid" '$2 == id'`,
		`/Library/PrivilegedHelperTools/org.nself.ci-agent`, `"755 root"`)
}

func TestTheRootShellRunsOnlyARootOwnedCopyOfTheAgent(t *testing.T) {
	a := agentScript(".up")
	mustContain(t, "agent", a,
		"install -d -o root -g wheel -m 0755 /Library/PrivilegedHelperTools",
		"install -o root -g wheel -m 0755 ./.up /Library/PrivilegedHelperTools/org.nself.ci-agent")
	mustContain(t, "uninstall", uninstallScript(), "rm -f /Library/PrivilegedHelperTools/org.nself.ci-agent")
	if TrustedPath == agentPath || strings.HasPrefix(TrustedPath, agentDir) {
		t.Fatal("the trusted copy must not live where the agent user can replace it")
	}
}

// Root must never create, chown or write an entry INSIDE a directory the agent user owns: the agent
// could have swapped the name for a symlink (or hard-linked a root file in), and root would follow it.
func TestRootWritesNothingInsideADirectoryTheAgentOwns(t *testing.T) {
	a := agentScript(".up")
	mustContain(t, "agent", a,
		"sudo -n -u nself-ci sh -c 'rm -f /opt/nself-ci-agent/bin/.upload.new && cat > /opt/nself-ci-agent/bin/.upload.new && chmod 0755 /opt/nself-ci-agent/bin/.upload.new && mv -f /opt/nself-ci-agent/bin/.upload.new /opt/nself-ci-agent/bin/nself-ci-agent' < ./.up")
	if strings.Contains(a, "install -o nself-ci") {
		t.Fatal("root installs the binary straight into the agent-owned bin directory")
	}
	d := dirsScript(2)
	mustContain(t, "dirs", d,
		"install -d -o root -g wheel -m 0755 /opt/nself-ci-agent\n", "install -d -o nself-ci -g nself-ci -m 0755 /opt/nself-ci-agent/bin",
		"install -d -o root -g wheel -m 0755 /var/log/nself-ci-agent", "chown -h nself-ci:nself-ci /var/log/nself-ci-agent/agent.log")
	if strings.Contains(d, "-R") {
		t.Fatalf("a recursive walk in the dirs script:\n%s", d)
	}
}

func TestEveryScriptFixesItsPATH(t *testing.T) {
	for name, s := range map[string]string{
		"users": usersScript(2), "dirs": dirsScript(2), "sudoers": sudoersScript(), "list": listJobUsersScript(), "remove": removeSlotScript(2),
		"agent": agentScript(".up"), "verify": verifyScript(2), "uninstall": uninstallScript(), "plist": plistScript(), "bootout": bootoutScript(), "xcode": xcodeProbeScript(""),
	} {
		if !strings.HasPrefix(s, "set -eu\nPATH=/usr/bin:/bin:/usr/sbin:/sbin\nexport PATH\n") {
			t.Errorf("%s: does not fix PATH first:\n%s", name, s)
		}
	}
}

func TestTheSudoersRuleGoesBeforeASurplusUserIsRetired(t *testing.T) {
	n, _, err := fullInstall(t, 1, func(n *fakeMac) { n.users = "nself-ci-job-1\nnself-ci-job-2\n" })
	if err != nil {
		t.Fatal(err)
	}
	sudoers, retire := -1, -1
	for i, s := range n.scripts() {
		if strings.Contains(s, "visudo -cf") {
			sudoers = i
		}
		if strings.Contains(s, "echo kept; exit 0") {
			retire = i
		}
	}
	if sudoers < 0 || retire < 0 || sudoers > retire {
		t.Fatalf("sudoers script at %d, retire at %d: the rule must go first", sudoers, retire)
	}
}

func TestAnIdThatStillOwnsFilesIsNotReused(t *testing.T) {
	u := usersScript(1)
	mustContain(t, "users", u, `find "$d" -xdev -uid "$i" -print`, "/Users/Shared", "/Volumes/*")
	un := uninstallScript()
	mustContain(t, "uninstall", un, `find "$d" -xdev -user "$u" -depth -delete`, "/private/tmp")
}

// cron is root-only on a stock macOS; the install must not create cron.allow or cron.deny (a deny file
// opens cron to every user it does not list), and a leftover crontab file goes with its account.
func TestCronPolicyIsLeftAloneAndLeftoverCrontabsGo(t *testing.T) {
	for name, s := range map[string]string{"users": usersScript(2), "uninstall": uninstallScript(), "remove": removeSlotScript(2)} {
		if strings.Contains(s, "cron.deny") || strings.Contains(s, "cron.allow") || strings.Contains(s, "at.deny") || strings.Contains(s, "sed -i") {
			t.Errorf("%s edits the host's cron or at policy:\n%s", name, s)
		}
	}
	mustContain(t, "uninstall", uninstallScript(), `rm -f "/var/at/tabs/$u"`)
	mustContain(t, "remove", removeSlotScript(2), `rm -f "/var/at/tabs/$u"`, `pgrep -u "$u"`)
}

func TestPerUserSystemFoldersDoNotKeepASlotAlive(t *testing.T) {
	r := removeSlotScript(2)
	mustContain(t, "remove", r,
		`find /private/var/folders -xdev -user "$u" -depth -delete`,
		`/var/db/nself-ci-jobs`, `/private/tmp /private/var/tmp /Users/Shared /Library/Caches /Volumes/*`)
	if strings.Contains(r, "/Applications") || strings.Contains(r, "find /private/var -xdev") {
		t.Fatal("the ownership scan walks Xcode or the whole system volume: minutes per slot on a real Mac")
	}
	mustContain(t, "uninstall", uninstallScript(), `find /private/var/folders -xdev -user "$u" -depth -delete`)
	if strings.Index(r, "-depth -delete") > strings.Index(r, "left=") {
		t.Fatal("the folders must be deleted before the ownership scan")
	}
}

func TestSudoersIncludeMatchIsExact(t *testing.T) {
	s := sudoersScript()
	mustContain(t, "sudoers", s, `(/private)?/etc/sudoers.d([[:space:]]|$)`, "rm -f ./.nself-ci-sudoers.stage")
}

func TestPlistScriptWaitsForTheOldJobAndCleansItsStage(t *testing.T) {
	mustContain(t, "plist", plistScript(), "trap 'rm -f ./.nself-ci-agent.plist.stage' EXIT", "sleep 1")
	mustContain(t, "bootout", bootoutScript(), "sleep 1")
}

func TestXcodeLicenseIsExitCode69Only(t *testing.T) {
	s := xcodeProbeScript("")
	mustContain(t, "xcode", s, "0) echo ok", "69) echo license", "*) echo absent")
	if strings.Contains(s, "command -v") {
		t.Fatal("the always-present /usr/bin/xcodebuild shim made a Command Line Tools Mac look like a license problem")
	}
	if got := xcodeWarning("absent"); got != "" {
		t.Fatalf("absent Xcode warned: %q", got)
	}
}

func TestRootNeverWalksATreeTheAgentOwns(t *testing.T) {
	d := dirsScript(2)
	if strings.Contains(d, "chown -R") || strings.Contains(d, "chmod -R") {
		t.Fatalf("a root-run recursive chown or chmod over the agent's state directory:\n%s", d)
	}
	mustContain(t, "dirs", d, "install -d -o nself-ci -g nself-ci -m 0700 /var/db/nself-ci-agent")
}
