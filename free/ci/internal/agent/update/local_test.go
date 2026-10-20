package update

// Purpose: the agent-side upgrade (ApplyLocal), the verified release source and the audit writer against real signed releases,
//          real stub binaries and a real store.

import (
	"context"
	"errors"
	"os"
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
	return LocalOptions{StateDir: s.state(), BinPath: s.bin(), Current: current, Version: target, Platform: hostPlatform, Source: src,
		Smoke: func(ctx context.Context, bin string) (string, error) { return smokeIn(s, ctx, bin) }}
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

func TestApplyLocalDowngradeNeedsTheFlagExceptRestoringPrev(t *testing.T) {
	s, rel := newSim(t, "1.5.0"), newRelease(t, "1.4.0", 1, 1)
	_, err := ApplyLocal(context.Background(), localOpts(s, "1.5.0", "1.4.0", rel.source()))
	if ReasonOf(err) != ReasonDowngrade || installed(t, s) != "1.5.0" {
		t.Fatalf("err %v", err)
	}
	// with a kept previous binary of exactly that version, the move back is a restore: no download, no flag
	if err = Swap(s.bin(), stubBinary(t, "1.4.0")); err != nil { // bin=1.4.0, .prev=1.5.0
		t.Fatal(err)
	}
	if err = Swap(s.bin(), stubBinary(t, "1.5.0")); err != nil { // bin=1.5.0, .prev=1.4.0
		t.Fatal(err)
	}
	res, err := ApplyLocal(context.Background(), localOpts(s, "1.5.0", "1.4.0", failingSource{}))
	if err != nil || res.Direction != Downgrade || installed(t, s) != "1.4.0" {
		t.Fatalf("res %+v err %v version %s", res, err, installed(t, s))
	}
}

type failingSource struct{}

func (failingSource) Prepare(context.Context, lifecycle.Platform) (Prepared, error) {
	return Prepared{}, errors.New("the source must not be used")
}

func TestApplyLocalRestoresWhenTheNewBinaryDoesNotRun(t *testing.T) {
	s, rel := newSim(t, "1.4.0"), newRelease(t, "1.5.0", 1, 1)
	o := localOpts(s, "1.4.0", "1.5.0", rel.source())
	o.Smoke = func(ctx context.Context, bin string) (string, error) {
		if bin == s.bin() { // after the swap the installed file fails; the staged copy ran fine before it
			return "", errors.New("exit status 1")
		}
		return smokeIn(s, ctx, bin)
	}
	res, err := ApplyLocal(context.Background(), o)
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
