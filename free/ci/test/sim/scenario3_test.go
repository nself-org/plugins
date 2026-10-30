//go:build integration

package sim

// Purpose: TestScenario3, the VPS leg (P7-NODE-17, NODE acceptance 2): a Docker sshd node with the REAL nself-ci-agent
//          behind netem delay of 80 ms, reached by the REAL ssh driver with a pinned host key: logs stream live, the
//          lease dispatch overhead is measured, cancel kills the remote process group within 5 s, the job root is gone
//          after finalize, and a changed host key is refused with E655 before any login or command. A bare foreground run
//          then places an owner-trust job on the node by its explicit platform, and the trust floor holds: the node has
//          no container runtime, so the job is never run below container isolation.
// Inputs:  INTEGRATION=1, a reachable Docker Engine (Linux or a Linux VM), ssh and git. The node runs as a container
//          with NET_ADMIN so tc netem applies; nothing here touches a real host.
// Outputs: metrics node.vps_dispatch_ms (one lease delivered and finished over the link, 24 runs) and node.vps_banner_ms (the link, recorded).
// Constraints: no stubs. The owner-trust rule is not relaxed: a registered node always gets the container floor, and a plain
//          Docker sshd node has no container runtime (the harness never runs --privileged), so the bare-run leg asserts the
//          refusal to run below the floor. The operator's trust authority (registry.SetTrust) assigns the node container
//          isolation only after the node proves it is a container. netem delays everything the node sends, so the time to the ssh banner (sshd sends it from inside the
//          node) measures the link and must show at least the configured delay.

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

const (
	vpsAgentDir = "/home/nself/.nself/ci-agent"
	vpsDelay    = 80 * time.Millisecond
)

func TestScenario3(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	arch := dockerArch(t)
	platform := "linux/" + arch
	fleet := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{{Name: "vps", Image: simharness.ImageDebian, CapAdd: []string{"NET_ADMIN"}}}})
	node := fleet.Node(t, "vps")
	sh := func(script string) string { return strings.TrimSpace(fleet.Exec(t, "vps", "sh", "-c", script)) }
	sh("apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git procps iproute2 >/dev/null")
	sh("install -d " + vpsAgentDir + "/bin && chown -R nself:nself /home/nself/.nself")
	fleet.CopyTo(t, "vps", buildVPSAgent(t, arch), vpsAgentDir+"/bin/nself-ci-agent")
	sh("chmod 755 " + vpsAgentDir + "/bin/nself-ci-agent && chown nself:nself " + vpsAgentDir + "/bin/nself-ci-agent")

	// The node proves its isolation boundary before the operator assigns it: it is a container.
	if sh("test -f /.dockerenv && echo yes || echo no") != "yes" {
		t.Fatal("the node is not a container; owner-trust jobs need container isolation and none is provable")
	}

	f := newRepoFixture(t)
	key := strings.Fields(sh("cat /etc/ssh/ssh_host_ed25519_key.pub"))
	pins := remote.PinnedHostKeys{Path: filepath.Join(f.home, "known_hosts")}
	if err := pins.Replace("nself-ci-vps", key[0]+" "+key[1]); err != nil {
		t.Fatal(err)
	}
	fleet.Netem(t, "vps", vpsDelay, 0)
	var connects []float64
	for i := 0; i < 5; i++ {
		connects = append(connects, bannerMs(t, node))
	}
	if min := minOf(connects); min < float64(vpsDelay.Milliseconds())-5 {
		t.Fatalf("netem is not on the link: the ssh banner arrived after %.1f ms, want at least about %d ms", min, vpsDelay.Milliseconds())
	}
	recordPerf("scenario3", "node.vps_banner_ms", "ms", "time to the ssh banner of the Docker node with netem delay 80 ms on everything the node sends; "+runtime.GOOS+"/"+runtime.GOARCH+" host, node "+platform, connects)

	ctx := context.Background()
	rig := &vpsRig{t: t, sh: sh, node: node, home: f.home, repo: f.repo, sha: f.sha, key: fleet.KeyPath()}

	t.Run("logs stream live over the link and the job root is gone after finalize", func(t *testing.T) {
		sink := newLineSink(nil)
		l := rig.lease("s3-stream", "sh", "-c", `git rev-parse HEAD; for i in 1 2 3; do echo "line-$i $(date +%s%N)"; sleep 0.5; done`)
		if err := rig.driver().Run(ctx, rig.capability(), l, sink); err != nil {
			t.Fatal(err)
		}
		got := sink.lines()
		if len(got) < 4 || got[0] != f.sha {
			t.Fatalf("the job did not run on the leased revision: %q", got)
		}
		first, last := sink.arrival("line-1"), sink.arrival("line-3")
		if last.Sub(first) < 800*time.Millisecond {
			t.Errorf("lines arrived together (%v apart), not as they were printed", last.Sub(first))
		}
		rig.requireJobRootGone(5 * time.Second)
	})

	t.Run("dispatch overhead over an 80 ms link is measured", func(t *testing.T) {
		var samples []float64
		for i := 0; i < 24; i++ {
			begin := time.Now()
			if err := rig.driver().Run(ctx, rig.capability(), rig.lease("s3-d", "true"), newLineSink(nil)); err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
			samples = append(samples, ms(time.Since(begin)))
		}
		recordPerf("scenario3", "node.vps_dispatch_ms", "ms", "one lease delivered and finished: real ssh, pinned key, bundle, real agent, no-op job, result and finalize, over netem delay 80 ms on the node's egress, 24 sequential runs; host "+runtime.GOOS+"/"+runtime.GOARCH+", node "+platform, samples)
	})

	t.Run("cancel kills the remote process group within 5 s and leaves no job root", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()
		var cancelled time.Time
		var once sync.Once
		sink := newLineSink(func(s string) {
			if s == "started" {
				once.Do(func() { cancelled = time.Now(); cancel() })
			}
		})
		err := rig.driver().Run(cctx, rig.capability(), rig.lease("s3-cancel", "sh", "-c", "echo started; sleep 301 & sleep 302"), sink)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if !eventually(5*time.Second, func() bool { return sh("pgrep -f 'sleep 30[12]' || echo none") == "none" }) {
			t.Fatalf("job processes survived %v after cancel: %s", time.Since(cancelled), sh("ps -eo pid,pgid,args | grep '[s]leep 30'"))
		}
		t.Logf("remote process group gone within %v of the cancel", time.Since(cancelled).Round(time.Millisecond))
		rig.requireJobRootGone(5 * time.Second)
	})

	t.Run("a bare run places an owner-trust job by its platform and never runs it below the container floor", func(t *testing.T) {
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

	t.Run("a changed host key is refused with E655 before any login or command", func(t *testing.T) {
		before := rig.logins()
		sh("rm -f /etc/ssh/ssh_host_* && ssh-keygen -A >/dev/null && pkill -HUP -x sshd")
		time.Sleep(time.Second)
		err := rig.driver().Run(ctx, rig.capability(), rig.lease("s3-key", "sh", "-c", "touch /tmp/should-not-run"), newLineSink(nil))
		var te *ssh.Error
		if !errors.As(err, &te) || te.Code != ssh.CodeHostKey || !strings.Contains(te.Message, "nodes rotate") {
			t.Fatalf("err = %v, want %s with the rotate remediation", err, ssh.CodeHostKey)
		}
		if after := rig.logins(); after != before {
			t.Fatalf("sshd accepted a login (%d -> %d) before the host key was checked", before, after)
		}
		if sh("test -e /tmp/should-not-run && echo ran || echo no") != "no" {
			t.Fatal("a command ran on the node")
		}
	})
}
