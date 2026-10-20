package lifecycle

// Purpose: regression tests for the cross-family review findings of P7-NODE-05.

import (
	"context"
	"debug/elf"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestFailedRevokerPrintsNoRevocationReport(t *testing.T) {
	t.Cleanup(resetExtensions)
	k := newKit(t)
	k.addNode("rv9")
	RegisterRevoker(func(context.Context, *store.RevokeTx, string) error { return errBoom })
	code, out, errs := k.run1("revoke", "rv9")
	if code == 0 || strings.Contains(out, "revoked") && !strings.Contains(errs, "boom") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	if strings.Contains(out, "rv9") {
		t.Fatalf("a rolled-back revoke printed a report: %q", out)
	}
}

func TestConcurrentAddsKeepTheWinnersPin(t *testing.T) {
	k := newKit(t)
	bin := elfFixture(t, elf.EM_X86_64)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = k.d.Add(context.Background(), AddOptions{Dest: "ci@race1", HostKey: fpA, Source: LocalFile{Path: bin}, Trust: DefaultTrust()})
		}()
	}
	wg.Wait()
	won := 0
	for _, e := range errs {
		if e == nil {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("%d adds succeeded, want 1: %v", won, errs)
	}
	if p, _ := k.d.pins().Lookup(pinAlias("race1")); len(p) != 1 {
		t.Fatalf("winner lost its pin: %v", p)
	}
}

func TestRevokeWinsOverARacingRotate(t *testing.T) {
	k := newKit(t)
	k.addNode("rr1")
	fs := FlagSet("rotate")
	_ = fs.Set("host-key", fpA)
	if _, err := k.d.Revoke(context.Background(), "rr1"); err != nil {
		t.Fatal(err)
	}
	errCode(t, k.d.Rotate(context.Background(), RotateOptions{NodeID: "rr1", Flags: fs}), CodeRevoked)
	if p, _ := k.d.pins().Lookup(pinAlias("rr1")); len(p) != 0 {
		t.Fatal("rotate restored the pin of a revoked node")
	}
}

func TestPlatformErrorsNeverEchoRemoteOutput(t *testing.T) {
	for _, in := range []string{"Linux " + secretFixtures[0], secretFixtures[1] + " amd64", "a b c " + secretFixtures[2]} {
		_, err := ParsePlatform(in)
		if err == nil || strings.Contains(err.Error(), "FAKE") || strings.Contains(err.Error(), "FFFF") {
			t.Fatalf("%q -> %v", in, err)
		}
	}
}

func TestPromptGoesToStderrAndJSONStaysClean(t *testing.T) {
	k := newKit(t)
	var so, se strings.Builder
	k.d.TTY, k.d.Stdin, k.d.Stdout, k.d.Stderr = true, strings.NewReader("yes\n"), &so, &se
	if _, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@pr1", Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()}); err != nil {
		t.Fatal(err)
	}
	if so.Len() != 0 || !strings.Contains(se.String(), fpA) {
		t.Fatalf("stdout=%q stderr=%q", so.String(), se.String())
	}
}

func TestRotateUsesHostKeyOptionAndToleratesNilFlags(t *testing.T) {
	k := newKit(t)
	k.addNode("rk1")
	if err := k.d.Rotate(context.Background(), RotateOptions{NodeID: "rk1", HostKey: fpA}); err != nil {
		t.Fatalf("HostKey option with nil flags: %v", err)
	}
	errCode(t, k.d.Rotate(context.Background(), RotateOptions{NodeID: "rk1", HostKey: fpB}), CodeHostKey)
}

func TestBadFlagStillEmitsJSONEnvelope(t *testing.T) {
	k := newKit(t)
	code, out, _ := k.run1("list", "--json", "--bogus")
	e := decode(t, out)
	if code != 1 || e.Error == nil || e.Error.Code != CodeInvalid {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if code, _, _ = k.run1("list", "-h"); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
}

func TestTestForcesAFreshProbe(t *testing.T) {
	k := newKit(t)
	k.addNode("fp1") // the add probed a moment ago: a cached prober would hide the drift below
	k.mutate = func(c *model.Capability) {
		c.Resources.CPU = known(3.0, "probe", time.Now().UTC())
	}
	res, err := k.d.Test(context.Background(), "fp1", nil)
	if err != nil || len(res.Drift) == 0 {
		t.Fatalf("drift within the cache window not reported: %+v %v", res, err)
	}
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), "resources/cpu") {
		t.Fatalf("drift = %s", b)
	}
}

func TestRunHooksIsPublic(t *testing.T) {
	t.Cleanup(resetExtensions)
	n := 0
	RegisterAddHook(PointPair, func(_ context.Context, e HookEvent) error { n++; return nil })
	if err := RunHooks(context.Background(), PointPair, HookEvent{NodeID: "x"}); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestNonSeparatedNodeHoldsNoSecretClass(t *testing.T) {
	k := newKit(t)
	k.d.Separation = fixedSeparation{SeparationFacts{Reason: "probe_unavailable"}}
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@nsc", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)},
		Trust: TrustFlags{Accepts: []string{"owner"}, SecretClasses: []string{"release"}, Ownership: "operator", AcceptsSet: true}})
	errCode(t, err, CodeInvalid)
	if p, _ := k.d.pins().Lookup(pinAlias("nsc")); len(p) != 0 {
		t.Fatal("refused add left a pin")
	}
}

func TestOneLineCutsOnRuneBoundary(t *testing.T) {
	if got := oneLine(errString(strings.Repeat("é", 200))); len([]rune(got)) != 160 || strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("len=%d", len([]rune(got)))
	}
}

func TestPinFileChangesAreSerializedAcrossProcesses(t *testing.T) {
	k := newKit(t)
	unlock, err := k.d.lockPins() // another process holds the pin lock
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- k.d.pin("pl1", []remote.HostKey{keyA}) }()
	select {
	case err := <-done:
		t.Fatalf("pin returned %v while another process held the pin lock", err)
	case <-time.After(300 * time.Millisecond):
	}
	unlock()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if p, _ := k.d.pins().Lookup(pinAlias("pl1")); len(p) != 1 {
		t.Fatalf("pins = %v", p)
	}
}

func TestNodeIDIsValidatedBeforeAnyLockFile(t *testing.T) {
	k := newKit(t)
	outside := filepath.Join(filepath.Dir(k.d.Home), "pwn")
	for _, id := range []string{"../pwn", "a/b", "bad id"} {
		code, _, errs := k.run1("revoke", id)
		if code != 1 || !strings.Contains(errs, "E651") {
			t.Errorf("%q: code=%d err=%q", id, code, errs)
		}
	}
	if _, err := os.Stat(outside + ".lock"); err == nil {
		t.Fatal("a lock file was created outside the CI home")
	}
	k.addNode("mixedcase")
	if code, _, _ := k.run1("show", "MixedCase"); code != 0 {
		t.Fatalf("upper-case id is lower-cased: code=%d", code)
	}
}

func TestEnableRefusesAQuarantinedNode(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("qn1")
	if _, err := k.d.Registry.SetState(ctx, "qn1", "maintenance", quarantineReason, "cli:test"); err != nil {
		t.Fatal(err)
	}
	errCode(t, func() error { _, e := k.d.Enable(ctx, "qn1"); return e }(), CodeInvalid)
}

func TestRemoveDropsAnOrphanPin(t *testing.T) {
	k := newKit(t)
	if err := k.d.pin("orph1", []remote.HostKey{keyA}); err != nil {
		t.Fatal(err)
	}
	errCode(t, k.d.pin("orph1", []remote.HostKey{keyA}), CodeHostKey) // an add would refuse
	rep, err := k.d.Remove(context.Background(), "orph1")
	if err != nil || !rep.PinRemoved {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
	if p, _ := k.d.pins().Lookup(pinAlias("orph1")); len(p) != 0 {
		t.Fatal("orphan pin survived")
	}
	if _, err = k.d.Remove(context.Background(), "orph1"); CodeOf(err) != CodeNotFound {
		t.Fatalf("second remove: %v", err)
	}
}

func TestPartialRevokeJSONCarriesTheReport(t *testing.T) {
	t.Cleanup(resetExtensions)
	k := newKit(t)
	k.addNode("pj1")
	RegisterSessionCloser(func(context.Context, string) error { return errBoom })
	code, out, _ := k.run1("revoke", "pj1", "--json")
	e := decode(t, out)
	if code != 3 || e.Error == nil || e.Error.Code != CodeRevoked {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "sessions_closed") || !strings.Contains(out, "remaining") {
		t.Fatalf("report missing from the envelope: %s", out)
	}
}

func TestRevokeAndRemoveNeedAnAdminActor(t *testing.T) {
	k := newKit(t)
	k.addNode("ac1")
	for _, actor := range []string{"agent", "probe", ""} {
		if _, err := k.st.NodeRemove(context.Background(), "ac1", actor, nil); err == nil {
			t.Errorf("actor %q removed a node", actor)
		}
	}
}

func TestDrainRefusesAQuarantinedNode(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("qn2")
	if _, err := k.d.Registry.SetState(ctx, "qn2", "maintenance", quarantineReason, "cli:test"); err != nil {
		t.Fatal(err)
	}
	errCode(t, func() error { _, e := k.d.Drain(ctx, "qn2"); return e }(), CodeInvalid)
	n, _ := k.d.Registry.Get(ctx, "qn2")
	if n.Record.Reason != quarantineReason {
		t.Fatalf("quarantine reason lost: %q", n.Record.Reason)
	}
}

func TestRevokerErrorTextNeverReachesOutputAndRollbackClearsCounts(t *testing.T) {
	t.Cleanup(resetExtensions)
	k := newKit(t)
	k.addNode("hk9")
	RegisterRevoker(func(context.Context, *store.RevokeTx, string) error { return errString("echo " + secretFixtures[0]) })
	rep, err := k.d.Revoke(context.Background(), "hk9")
	if err == nil || strings.Contains(err.Error(), "FAKE-SECRET") || CodeOf(err) != "E607" {
		t.Fatalf("err = %v", err)
	}
	if rep.LeasesExpired != 0 || rep.Removed {
		t.Fatalf("rollback reported counts: %+v", rep)
	}
	code, out, errs := k.run1("revoke", "hk9", "--json")
	if code == 0 || strings.Contains(out+errs, "FAKE-SECRET") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestBadFlagWithJSONEqualsTrueStillEmitsEnvelope(t *testing.T) {
	k := newKit(t)
	_, out, _ := k.run1("list", "--bogus", "--json=true")
	if e := decode(t, out); e.Error == nil {
		t.Fatalf("out = %s", out)
	}
}

func TestOrphanPinSurvivesANonNotFoundRegistryError(t *testing.T) {
	k := newKit(t)
	if err := k.d.pin("orph2", []remote.HostKey{keyA}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // every registry read fails with a context error, not E650
	if _, err := k.d.Remove(ctx, "orph2"); err == nil {
		t.Fatal("remove with a dead context succeeded")
	}
	if p, _ := k.d.pins().Lookup(pinAlias("orph2")); len(p) != 1 {
		t.Fatal("a registry read error deleted the pin")
	}
}
