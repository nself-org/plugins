package update

// Purpose: the agent-side upgrade (ApplyLocal), the verified release source and the audit writer against real signed releases,
//          real stub binaries and a real store.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func localOpts(s *sim, current, target string, src Source) LocalOptions {
	return LocalOptions{StateDir: s.state(), BinPath: s.bin(), Current: current, Version: target, Platform: hostPlatform, Source: src}
}

// smokeIn runs `<bin> version` with the sim's state dir, as the agent would see it.
func smokeIn(s *sim, ctx context.Context, bin string) (string, error) {
	t := s.t
	t.Setenv("STUB_STATE", s.state())
	return runVersion(ctx, bin)
}

func installed(t *testing.T, s *sim) string {
	t.Helper()
	v, err := s.version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestApplyLocalUpgradesAndKeepsPrevious(t *testing.T) {
	s, rel := newSim(t, "1.4.0"), newRelease(t, "1.5.0", 1, 1)
	res, err := ApplyLocal(context.Background(), localOpts(s, "1.4.0", "1.5.0", rel.source()))
	if err != nil || res.Noop || installed(t, s) != "1.5.0" {
		t.Fatalf("res %+v err %v version %s", res, err, installed(t, s))
	}
	if v, _ := smokeIn(s, context.Background(), s.bin()+PrevSuffix); v != "1.4.0" {
		t.Fatalf("previous binary not kept: %q", v)
	}
}

func TestApplyLocalRefusesWhileAJobRuns(t *testing.T) {
	s, rel := newSim(t, "1.4.0"), newRelease(t, "1.5.0", 1, 1)
	tbl, err := slots.Open(s.state(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tbl.Acquire(slots.Self("c1", "lease-1")); err != nil {
		t.Fatal(err)
	}
	_, err = ApplyLocal(context.Background(), localOpts(s, "1.4.0", "1.5.0", rel.source()))
	if ReasonOf(err) != ReasonRunningJob || installed(t, s) != "1.4.0" {
		t.Fatalf("err %v version %s", err, installed(t, s))
	}
}

func TestApplyLocalRefusesPastTheCoordinatorCeiling(t *testing.T) {
	s, rel := newSim(t, "1.4.0"), newRelease(t, "2.0.0", 2, 2)
	if err := session.RecordWelcome(s.state(), session.Coordinator{ID: "c1"}, protocol.Welcome{Version: 1}, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := ApplyLocal(context.Background(), localOpts(s, "1.4.0", "2.0.0", rel.source()))
	if ReasonOf(err) != ReasonCeiling || installed(t, s) != "1.4.0" {
		t.Fatalf("err %v version %s", err, installed(t, s))
	}
	if _, serr := os.Stat(s.bin() + PrevSuffix); serr == nil {
		t.Fatal("a refused upgrade left a .prev")
	}
}

func TestApplyLocalDowngradeNeedsTheFlag(t *testing.T) {
	s, rel := newSim(t, "1.5.0"), newRelease(t, "1.4.0", 1, 1)
	_, err := ApplyLocal(context.Background(), localOpts(s, "1.5.0", "1.4.0", rel.source()))
	if ReasonOf(err) != ReasonDowngrade || installed(t, s) != "1.5.0" {
		t.Fatalf("err %v", err)
	}
	o := localOpts(s, "1.5.0", "1.4.0", rel.source())
	o.AllowDowngrade = true
	if res, err := ApplyLocal(context.Background(), o); err != nil || res.Direction != Downgrade || installed(t, s) != "1.4.0" {
		t.Fatalf("res %+v err %v version %s", res, err, installed(t, s))
	}
}

func TestApplyLocalRestoreOfThePreviousBinaryNeedsAnExplicitRequest(t *testing.T) {
	s := newSim(t, "1.5.0")
	keepPrev(t, s, "1.4.0", "1.5.0") // bin=1.5.0, .prev=1.4.0 recorded as 1.4.0
	// an operator typing `upgrade --version 1.4.0` without the flag is a downgrade like any other
	_, err := ApplyLocal(context.Background(), localOpts(s, "1.5.0", "1.4.0", failingSource{}))
	if ReasonOf(err) != ReasonDowngrade || installed(t, s) != "1.5.0" {
		t.Fatalf("err %v version %s", err, installed(t, s))
	}
	// a coordinator that raised its minimum agent version past the kept binary cannot be sent back to it
	if err = session.RecordWelcome(s.state(), session.Coordinator{ID: "c1"}, protocol.Welcome{Version: 1, MinAgentVersion: "1.5.0"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	o := localOpts(s, "1.5.0", "1.4.0", failingSource{})
	o.AllowRestore = true
	if _, err = ApplyLocal(context.Background(), o); ReasonOf(err) != ReasonCeiling || installed(t, s) != "1.5.0" {
		t.Fatalf("err %v version %s", err, installed(t, s))
	}
	_ = os.Remove(filepath.Join(s.state(), session.CoordinatorsFile))
	res, err := ApplyLocal(context.Background(), o)
	if err != nil || res.Direction != Downgrade || installed(t, s) != "1.4.0" {
		t.Fatalf("res %+v err %v version %s", res, err, installed(t, s))
	}
	if _, serr := os.Stat(s.bin() + PrevVersionSuffix); serr == nil {
		t.Fatal("the marker outlived the restore")
	}
}

func TestApplyLocalIgnoresAStaleMarker(t *testing.T) {
	s := newSim(t, "1.5.0")
	keepPrev(t, s, "1.3.0", "1.4.0")
	if err := Swap(s.bin(), stubBinary(t, "1.5.0")); err != nil { // a later swap by another carrier: .prev is now 1.4.0
		t.Fatal(err)
	}
	o := localOpts(s, "1.5.0", "1.3.0", failingSource{})
	o.AllowRestore = true
	if _, err := ApplyLocal(context.Background(), o); ReasonOf(err) != ReasonDowngrade || installed(t, s) != "1.5.0" {
		t.Fatalf("a marker from an older swap restored the wrong binary: %v (version %s)", err, installed(t, s))
	}
}

// busyDuringPrepare leases a job while the release downloads, like a second coordinator would.
type busyDuringPrepare struct {
	Source
	state string
	t     *testing.T
}

func (b busyDuringPrepare) Prepare(ctx context.Context, p lifecycle.Platform) (Prepared, error) {
	tbl, err := slots.Open(b.state, 2)
	if err != nil {
		b.t.Fatal(err)
	}
	if _, err = tbl.Acquire(slots.Self("c2", "lease-2")); err != nil {
		b.t.Fatal(err)
	}
	return b.Source.Prepare(ctx, p)
}

func TestApplyLocalLooksAgainForJobsRightBeforeTheSwap(t *testing.T) {
	s, rel := newSim(t, "1.4.0"), newRelease(t, "1.5.0", 1, 1)
	src := busyDuringPrepare{Source: rel.source(), state: s.state(), t: t}
	_, err := ApplyLocal(context.Background(), localOpts(s, "1.4.0", "1.5.0", src))
	if ReasonOf(err) != ReasonRunningJob || installed(t, s) != "1.4.0" {
		t.Fatalf("err %v version %s", err, installed(t, s))
	}
}

type failingSource struct{}

func (failingSource) Prepare(context.Context, lifecycle.Platform) (Prepared, error) {
	return Prepared{}, errors.New("the source must not be used")
}

// keepPrev leaves bin at version cur and the kept previous binary at version prev, as a finished ApplyLocal does.
func keepPrev(t *testing.T, s *sim, prev, cur string) {
	t.Helper()
	for _, v := range []string{prev, cur} {
		if err := Swap(s.bin(), stubBinary(t, v)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(s.bin()+PrevVersionSuffix, []byte(prev+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// lyingSource hands over a file that is not the one its manifest signed.
type lyingSource struct{ real Prepared }

func (l lyingSource) Prepare(context.Context, lifecycle.Platform) (Prepared, error) {
	return l.real, nil
}

func TestApplyLocalRestoresWhenTheInstalledFileIsNotTheVerifiedOne(t *testing.T) {
	s, rel := newSim(t, "1.4.0"), newRelease(t, "1.5.0", 1, 1)
	p, err := rel.source().Prepare(context.Background(), hostPlatform)
	if err != nil {
		t.Fatal(err)
	}
	p.Manifest.SHA256 = strings.Repeat("0", 64) // the signed digest is not the file's
	res, err := ApplyLocal(context.Background(), localOpts(s, "1.4.0", "1.5.0", lyingSource{real: p}))
	if err == nil || !res.RolledBack || installed(t, s) != "1.4.0" {
		t.Fatalf("res %+v err %v version %s", res, err, installed(t, s))
	}
}

func TestApplyLocalNeverInstallsAnUnverifiedRelease(t *testing.T) {
	s := newSim(t, "1.4.0")
	good, other := newRelease(t, "1.5.0", 1, 1), newRelease(t, "1.5.0", 1, 1)
	src := ReleaseSource{Release: releaseOf(good)}
	src.Release.Verifier = other.Verifier // trusts another key than the one that signed
	_, err := ApplyLocal(context.Background(), localOpts(s, "1.4.0", "1.5.0", src))
	if err == nil || !strings.HasPrefix(err.Error(), "E665:") || installed(t, s) != "1.4.0" {
		t.Fatalf("err %v version %s", err, installed(t, s))
	}
}

func TestReleaseSourceReturnsTheVerifiedManifest(t *testing.T) {
	rel := newRelease(t, "1.5.0", 1, 3)
	p, err := rel.source().Prepare(context.Background(), hostPlatform)
	if err != nil {
		t.Fatal(err)
	}
	if p.Manifest.Version != "1.5.0" || p.Manifest.ProtocolMin != 1 || p.Manifest.ProtocolMax != 3 {
		t.Fatalf("manifest %+v", p.Manifest)
	}
	if _, err = os.Stat(p.Path); err != nil {
		t.Fatal(err)
	}
	p.Cleanup()
	if _, err = os.Stat(filepath.Dir(p.Path)); err == nil {
		t.Fatal("cleanup left the private directory")
	}
}

func TestStoreAuditorWritesRowsTheAuditTableAccepts(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	a := StoreAuditor{Store: st, Actor: "cli:admin"}
	if err = a.Step(context.Background(), StepDrain, "n1", "was_online=true"); err != nil {
		t.Fatal(err)
	}
	rows, err := st.AuditEntries(context.Background(), 0, 10)
	if err != nil || len(rows) != 1 || rows[0].Action != "node.upgrade.drain" || rows[0].Target != "n1" || rows[0].Actor != "cli:admin" {
		t.Fatalf("rows %+v err %v", rows, err)
	}
}

// runVersion runs `<bin> version` like cmd/agent's smoke does.
func runVersion(ctx context.Context, bin string) (string, error) {
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	if err != nil {
		return "", err
	}
	v, ok := ParseVersionLine(string(out))
	if !ok {
		return "", errors.New("unexpected version line")
	}
	return v, nil
}
