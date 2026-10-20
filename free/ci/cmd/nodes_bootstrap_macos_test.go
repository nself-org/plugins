package main

// Purpose: `nodes add --system` on a macOS node (P7-NODE-31): the blank import in nodes_bootstrap_macos.go
// registers the darwin family in this binary, so the installer drives dscl, the sudoers drop-in, the
// agent upload and the LaunchDaemon on a fake Darwin node, links the agent for the lifecycle, and the
// node stays recorded as NOT uid-separated until the agent runs jobs through the job shim (D-1230).
// Inputs:  fake lifecycle.Deps seams. Constraints: no network, no ssh, no real dscl.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

func TestSystemInstallOnDarwinUsesTheDarwinFamily(t *testing.T) {
	if _, ok := bootstrap.Lookup("darwin"); !ok {
		t.Fatal("the darwin family is not registered in the nself ci binary")
	}
	bin := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	d, calls, errb := fakeDeps(t, "Darwin")
	fs, _ := addFlags(t, "--system", "--slots", "2")
	req := lifecycle.InstallRequest{NodeID: "mac-sys-1", Platform: lifecycle.Platform{OS: "darwin", Arch: "arm64"}, Dir: ".nself/ci-agent", Source: fakeSource{bin}, Flags: fs}
	if err := systemInstall(context.Background(), d, req); err != nil {
		t.Fatal(err)
	}
	all := ""
	for _, c := range *calls {
		all += strings.Join(c, " ") + "\n"
	}
	for _, want := range []string{
		"dscl . -create", "IsHidden 1", "for n in 1 2; do mk nself-ci-job-$n; done", "visudo -cf ./.nself-ci-sudoers.stage",
		"copy .nself-ci-sudoers.stage .nself-ci-sudoers.stage", "copy .nself-ci-agent.plist.stage .nself-ci-agent.plist.stage",
		"copy agent .nself-ci-agent.upload", "launchctl bootstrap system /Library/LaunchDaemons/org.nself.ci-agent.plist",
		"ln -sf /opt/nself-ci-agent/bin/nself-ci-agent '.nself/ci-agent/bin/nself-ci-agent'",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("the node never saw %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "useradd") || strings.Contains(all, "brew install") || strings.Contains(errb.String(), "brew install") {
		t.Error("a Linux or brew step ran on the Mac")
	}
	if facts := (lifecycle.AgentSeparation{}).Detect(context.Background(), nil, lifecycle.Platform{OS: "darwin"}); facts.UIDSeparation {
		t.Fatalf("a Mac whose jobs do not run through the shim yet was marked separated: %+v", facts)
	}
	if _, ok := installed.LoadAndDelete("mac-sys-1"); !ok {
		t.Fatal("the guard would refuse a finished install")
	}
}
