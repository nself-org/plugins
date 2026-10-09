//go:build integration

package probe

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestProbe(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1")
	}
	f := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{{Name: "debian", Image: simharness.ImageDebian}}})
	f.Exec(t, "debian", "sh", "-c", `printf '#!/bin/sh\necho go version go1.23\n' >/usr/local/bin/go && chmod 755 /usr/local/bin/go`)
	node := f.Node(t, "debian")
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	r := registry.New(s)
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var local, ssh model.Capability
	if err = json.Unmarshal(b, &local); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &ssh); err != nil {
		t.Fatal(err)
	}
	ssh.Identity.ID = "01J00000000000000000000001"
	ssh.Identity.Name = "sim-debian-ssh"
	ssh.Identity.Provider = "ssh"
	ssh.Identity.Transport = "ssh"
	ssh.Identity.SSH = &model.CapabilitySSH{Hostname: node.Host, Port: node.Port, User: node.User}
	for _, c := range []model.Capability{local, ssh} {
		if _, err = r.Register(context.Background(), c, "operator"); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := remote.ScanHostKeys(context.Background(), node.Host, node.Port)
	if err != nil || len(keys) == 0 {
		t.Fatalf("no sim host keys: %v", err)
	}
	pins := remote.PinnedHostKeys{Path: filepath.Join(t.TempDir(), "known_hosts")}
	for _, key := range keys {
		if err = pins.Add("nself-ci-"+ssh.Identity.ID, key.Line); err != nil {
			t.Fatal(err)
		}
	}
	version, err := remote.SSHVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	localRun := func(ctx context.Context, command string) (string, error) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Harness execution is the local view of the same container. Only the
		// literal probe command is interpolated; optional absence exits zero.
		return f.Exec(t, "debian", "sh", "-c", command+" 2>/dev/null || true"), nil
	}
	count := parseNproc(f.Exec(t, "debian", "nproc"))
	if count == nil {
		t.Fatal("nproc unavailable in sim")
	}
	lp := &LocalProber{Registry: r, runner: localRun, capacity: func() ciexec.Capacity { return ciexec.Capacity{CPUs: int(*count), BatteryPercent: -1} }}
	sp := &SSHProber{Registry: r, PinnedFile: pins.Path, KeyPath: f.KeyPath(), Version: version}
	var ln, sn registry.Node
	t.Run("identical shared facts", func(t *testing.T) {
		ln, err = lp.Probe(context.Background(), local.Identity.ID)
		if err != nil {
			t.Fatal(err)
		}
		sn, err = sp.Probe(context.Background(), ssh.Identity.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(ln.Capability.Platform.OS.Value, sn.Capability.Platform.OS.Value) || !reflect.DeepEqual(ln.Capability.Platform.Arch.Value, sn.Capability.Platform.Arch.Value) || !reflect.DeepEqual(ln.Capability.Platform.Kernel.Value, sn.Capability.Platform.Kernel.Value) || !reflect.DeepEqual(ln.Capability.Resources.DiskFreeMB.Value, sn.Capability.Resources.DiskFreeMB.Value) || !reflect.DeepEqual(ln.Capability.Resources.CPU.Value, sn.Capability.Resources.CPU.Value) || !reflect.DeepEqual(ln.Capability.Tools.Toolchains["go"].Value, sn.Capability.Tools.Toolchains["go"].Value) {
			t.Fatal("local and SSH views diverged")
		}
	})
	t.Run("removed go drifts", func(t *testing.T) {
		f.Exec(t, "debian", "rm", "/usr/local/bin/go")
		c := sn.Capability
		c.Verified.At = time.Now().Add(-cacheTTL - time.Second)
		if _, _, err = r.UpdateCapability(context.Background(), ssh.Identity.ID, c, "probe"); err != nil {
			t.Fatal(err)
		}
		changed, err := sp.Probe(context.Background(), ssh.Identity.ID)
		if err != nil {
			t.Fatal(err)
		}
		if changed.Capability.Tools.Toolchains["go"].Value != nil {
			t.Fatal("removed go was not observed as drift")
		}
	})
	t.Run("missing power and GPU unknown", func(t *testing.T) {
		if ln.Capability.Availability.BatteryPct.Value != nil || ln.Capability.Availability.PluggedIn.Value != nil || sn.Capability.Resources.Accelerators.Value != nil {
			t.Fatal("missing facts must be null")
		}
	})
	t.Run("isolation only lowers", func(t *testing.T) {
		TestProbeDarwinFactsAndIsolation(t)
	})
	t.Run("concurrency bounds", func(t *testing.T) {
		TestProbeConcurrentLimits(t)
	})
}
