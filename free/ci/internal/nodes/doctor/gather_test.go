package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// A fixed, throwaway ed25519 host key line (synthetic; it guards nothing).
const fakeHostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okmM+Pmz0ysGiv"

type fakes struct {
	dialed []string
	ran    [][]string
	skew   time.Duration
}

func (fk *fakes) deps(open func(string) (*store.Store, error)) Deps {
	return Deps{
		Now:        func() time.Time { return t0 },
		OpenStore:  open,
		SSHVersion: func(context.Context) (remote.Version, error) { return remote.Version{Major: 9, Minor: 6}, nil },
		Dial: func(_ context.Context, addr string) error {
			fk.dialed = append(fk.dialed, addr)
			if strings.HasPrefix(addr, "down.") {
				return errors.New("connection refused")
			}
			return nil
		},
		RunRemote: func(_ context.Context, _ remote.Target, argv ...string) (string, error) {
			fk.ran = append(fk.ran, argv)
			return strconv.FormatInt(t0.Add(fk.skew).Unix(), 10), nil
		},
	}
}

func env(home string) doctor.Env {
	return doctor.Env{Root: home, Getenv: func(k string) string {
		if k == "NSELF_CI_HOME" {
			return home
		}
		return ""
	}}
}

func TestGatherWithoutStoreCreatesNothing(t *testing.T) {
	home := t.TempDir()
	fk := &fakes{}
	f := Gather(context.Background(), env(home), fk.deps(func(string) (*store.Store, error) {
		t.Fatal("the store must not be opened when its file does not exist")
		return nil, nil
	}))
	if f.StoreNote == "" || f.StoreErr != nil || len(f.Nodes) != 0 {
		t.Fatalf("facts = %+v", f)
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("a doctor run created the state database: %v", err)
	}
	if out := Evaluate(f); doctor.ExitCode(out) != doctor.ExitClean {
		t.Fatalf("an empty home must be clean: %+v", out)
	}
}

func TestGatherReadsRegistryPinsAndClocks(t *testing.T) {
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	reg := registry.New(st)
	for _, id := range []string{"01J0000000000000000000SSH1", "01J0000000000000000000DOWN"} {
		n := sshNode(t, id)
		n.Cap.Identity.SSH.Hostname = map[bool]string{true: "down.lan.test", false: "up.lan.test"}[strings.HasSuffix(id, "DOWN")]
		n.Cap.Identity.HostKeyFingerprints = nil
		if _, err = reg.Register(ctx, n.Cap, "operator"); err != nil {
			t.Fatal(err)
		}
	}
	ag := agentNode(t, "01J00000000000000000AGENT1")
	if _, err = reg.Register(ctx, ag.Cap, "operator"); err != nil {
		t.Fatal(err)
	}
	if err = st.PKICertPut(ctx, store.PKICert{Serial: "ab01", Kind: "node", NodeID: ag.ID(), NotAfter: t0.Add(30 * time.Hour).UnixNano()}); err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	pins := remote.PinnedHostKeys{Path: filepath.Join(home, "known_hosts")}
	if err = pins.Add("nself-ci-01J0000000000000000000SSH1", fakeHostKey); err != nil {
		t.Fatal(err)
	}

	fk := &fakes{skew: 200 * time.Second}
	f := Gather(ctx, env(home), fk.deps(openExisting))
	if f.StoreErr != nil || len(f.Nodes) != 3 {
		t.Fatalf("store err %v, %d nodes", f.StoreErr, len(f.Nodes))
	}
	out := Evaluate(f)
	want := map[string]doctor.Status{
		IDReach + "/01J0000000000000000000DOWN":   doctor.Fail,
		IDReach + "/01J0000000000000000000SSH1":   doctor.Pass,
		IDHostKey + "/01J0000000000000000000DOWN": doctor.Fail, // reachable or not, no pin means no trust
		IDHostKey + "/01J0000000000000000000SSH1": doctor.Pass,
		IDSkew + "/01J0000000000000000000SSH1":    doctor.Fail,
		IDCert + "/01J00000000000000000AGENT1":    doctor.Warn,
	}
	for key, st := range want {
		id, subject, _ := strings.Cut(key, "/")
		found := false
		for _, c := range byID(out, id) {
			if c.Subject == subject {
				found = true
				if c.Status != st {
					t.Errorf("%s = %s (%s), want %s", key, c.Status, c.Message, st)
				}
			}
		}
		if !found {
			t.Errorf("%s: no result in %+v", key, byID(out, id))
		}
	}
	// Only the pinned, reachable node was asked for its clock, with the one fixed argv.
	if len(fk.ran) != 1 || strings.Join(fk.ran[0], " ") != "date -u +%s" {
		t.Errorf("remote commands = %v", fk.ran)
	}
	if len(fk.dialed) != 2 {
		t.Errorf("dialed = %v", fk.dialed)
	}
}

func TestGatherDiscoverySwitch(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ci:\n  discovery: off\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fk := &fakes{}
	f := Gather(context.Background(), env(home), fk.deps(openExisting))
	if !f.DiscoveryOff || f.DiscoveryErr != nil {
		t.Fatalf("off config: off=%v err=%v", f.DiscoveryOff, f.DiscoveryErr)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ci: [unclosed"), 0o600); err != nil {
		t.Fatal(err)
	}
	f = Gather(context.Background(), env(home), fk.deps(openExisting))
	if !f.DiscoveryOff || f.DiscoveryErr == nil {
		t.Fatalf("broken config: off=%v err=%v", f.DiscoveryOff, f.DiscoveryErr)
	}
	if rows := byID(Evaluate(f), IDDiscovery); worst(rows) != doctor.Warn {
		t.Fatalf("broken config must warn: %+v", rows)
	}
}
