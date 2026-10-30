//go:build integration

package sim

// Purpose: TestScenario2Arm64, the other-architecture leg of scenario 2 (P7-NODE-17, NODE acceptance 1): a Docker sshd
//          node of platform linux/arm64 (qemu-user on an amd64 host, native on an arm64 host) runs the REAL arm64
//          nself-ci-agent. The real ssh driver delivers a lease whose job is the matrix entry linux/arm64 and the
//          agent runs it on that node; a bare foreground run then steers the same matrix entry to the node by platform.
// Inputs:  INTEGRATION=1, a reachable Docker Engine, and (on an amd64 host) qemu-user binfmt for arm64
//          (docker/setup-qemu-action in ci-node-scenarios.yml).
// Outputs: none beyond the assertions.
// Constraints: no stubs and no netem here (the link is TestScenario3's). The owner-trust floor is not relaxed: the
//          node has no container runtime, so the bare run is asserted to place the job and then refuse to run it
//          below the floor; the job that runs to completion is the lease the driver delivers directly.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestScenario2Arm64(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	const platform, arch = "linux/arm64", "arm64"
	fleet := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{{Name: "vps", Image: simharness.ImageDebian, Platform: platform}}})
	node := fleet.Node(t, "vps")
	sh := func(script string) string { return strings.TrimSpace(fleet.Exec(t, "vps", "sh", "-c", script)) }
	if got := sh("uname -m"); got != "aarch64" {
		t.Fatalf("the node reports %q, want aarch64", got)
	}
	sh("apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git procps >/dev/null")
	sh("install -d " + vpsAgentDir + "/bin && chown -R nself:nself /home/nself/.nself")
	fleet.CopyTo(t, "vps", buildVPSAgent(t, arch), vpsAgentDir+"/bin/nself-ci-agent")
	sh("chmod 755 " + vpsAgentDir + "/bin/nself-ci-agent && chown nself:nself " + vpsAgentDir + "/bin/nself-ci-agent")
	// ELF e_machine at offset 18: 0xb7 is AArch64, so the agent that runs the job is a real arm64 build.
	if machine := sh("od -An -tx1 -j18 -N2 " + vpsAgentDir + "/bin/nself-ci-agent | tr -d ' '"); machine != "b700" {
		t.Fatalf("agent e_machine = %q, want b700 (aarch64)", machine)
	}

	f := newRepoFixture(t)
	key := strings.Fields(sh("cat /etc/ssh/ssh_host_ed25519_key.pub"))
	pins := remote.PinnedHostKeys{Path: filepath.Join(f.home, "known_hosts")}
	if err := pins.Replace("nself-ci-vps", key[0]+" "+key[1]); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rig := &vpsRig{t: t, sh: sh, node: node, home: f.home, repo: f.repo, sha: f.sha, key: fleet.KeyPath()}

	t.Run("the arm64 node receives a matrix linux/arm64 job and runs it under the real agent", func(t *testing.T) {
		sink := newLineSink(nil)
		l := rig.lease("a64-run", "sh", "-c", `echo "machine=$(uname -m)"; echo "revision=$(git rev-parse HEAD)"; echo "pwd=$(pwd)"`)
		l.Job.Matrix = &model.JobMatrix{Platform: []any{platform}}
		if err := rig.driver().Run(ctx, rig.capability(), l, sink); err != nil {
			t.Fatal(err)
		}
		text := strings.Join(sink.lines(), "\n")
		t.Logf("job output on the arm64 node:\n%s", text)
		for _, want := range []string{"machine=aarch64", "revision=" + f.sha} {
			if !strings.Contains(text, want) {
				t.Errorf("job output lacks %q", want)
			}
		}
		rig.requireJobRootGone(10 * time.Second)
	})

	t.Run("a bare run steers the matrix linux/arm64 entry to the arm64 node and holds the container floor", func(t *testing.T) {
		registerVPS(t, f, node, arch)
		useRealSSH(t, f, fleet.KeyPath())
		c := matrixConfig(platform)
		j := c.Jobs["unit"]
		j.Run = []string{"sh", "-c", "touch /tmp/ran-below-floor"}
		c.Jobs["unit"] = j
		ch, out := f.startConfig(ctx, c, []string{platform})
		res := wait(t, ch, "the run")
		lines := explainJob(t, f, res.RunID, "unit@"+platform)
		t.Logf("run output:\n%s\nci explain:\n%s", out, lines)
		for _, want := range []string{"vps", platform, "placed"} {
			if !strings.Contains(lines, want) {
				t.Errorf("explain does not mention %q", want)
			}
		}
		if res.Exit == 0 || sh("test -e /tmp/ran-below-floor && echo ran || echo no") != "no" {
			t.Fatalf("exit %d: an owner-trust job ran on a node without a container runtime", res.Exit)
		}
	})
}
