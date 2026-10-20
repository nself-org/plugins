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
		"install -o nself-ci -g nself-ci -m 0755 ./.up /opt/nself-ci-agent/bin/nself-ci-agent",
		"install -d -o root -g wheel -m 0755 /Library/PrivilegedHelperTools",
		"install -o root -g wheel -m 0755 ./.up /Library/PrivilegedHelperTools/org.nself.ci-agent")
	mustContain(t, "uninstall", uninstallScript(), "rm -f /Library/PrivilegedHelperTools/org.nself.ci-agent")
	if TrustedPath == agentPath || strings.HasPrefix(TrustedPath, agentDir) {
		t.Fatal("the trusted copy must not live where the agent user can replace it")
	}
}

func TestPerUserSystemFoldersDoNotKeepASlotAlive(t *testing.T) {
	r := removeSlotScript(2)
	mustContain(t, "remove", r,
		`find /private/var/folders -xdev -user "$u" -depth -delete`,
		`find /private/var -xdev -path /private/var/folders -prune -o -user "$u" -print`,
		`/Volumes/*`)
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
