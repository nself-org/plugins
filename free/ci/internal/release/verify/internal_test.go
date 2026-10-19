package verify

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/authz"
)

func TestReadFileHonoursTheSizeCap(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"empty": "", "ten": "0123456789", "eleven": "0123456789a"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, ok := range map[string]bool{"empty": true, "ten": true, "eleven": false, "missing": false} {
		b, err := readFile(dir, name, 10)
		if (err == nil) != ok {
			t.Errorf("%s: err = %v", name, err)
		}
		if ok && len(b) != len(map[string]string{"empty": "", "ten": "0123456789"}[name]) {
			t.Errorf("%s: read %d bytes", name, len(b))
		}
	}
}

func TestSafePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real", "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skip("no symlinks here")
	}
	for rel, ok := range map[string]bool{
		"real/f": true, "real": true, "": false, "..": false, "../x": false, "real/../real/f": false, "./real": false, "real//f": false,
		"/real/f": false, "real\\f": false, "real/f\x00": false, "link": false, "link/x": false, "nope": false,
	} {
		_, err := safePath(dir, rel)
		if (err == nil) != ok {
			t.Errorf("%q: err = %v", rel, err)
		}
	}
	if _, _, err := open(dir, "real"); err == nil {
		t.Error("a directory opened as a file")
	}
	if _, err := digest(dir, "link"); err == nil {
		t.Error("a symlink was hashed")
	}
	info, err := digest(dir, "real/f")
	if err != nil || info.size != 1 || info.sha != sum([]byte("x")) {
		t.Errorf("digest = %+v, %v", info, err)
	}
}

func TestParseSums(t *testing.T) {
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	good := a + "  artifacts/a\n" + b + "  evidence.json\n"
	lines, err := parseSums([]byte(good))
	if err != nil || len(lines) != 2 || lines[0] != (sumLine{a, "artifacts/a"}) || lines[1] != (sumLine{b, "evidence.json"}) {
		t.Fatalf("%v %+v", err, lines)
	}
	for name, doc := range map[string]string{
		"empty":            "",
		"no final newline": a + "  x",
		"one space":        a + " x\n",
		"short digest":     a[:63] + "  x\n",
		"upper case":       strings.ToUpper(b) + "  x\n",
		"not hex":          strings.Repeat("g", 64) + "  x\n",
		"empty path":       a + "  \n",
		"descending":       a + "  b\n" + a + "  a\n",
		"duplicate":        a + "  a\n" + a + "  a\n",
		"blank line":       a + "  a\n\n",
	} {
		if _, err := parseSums([]byte(doc)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := parseSums([]byte(a + "  a\n" + a + "  b\n")); err != nil {
		t.Errorf("ascending paths with equal digests refused: %v", err)
	}
}

func TestIsHex(t *testing.T) {
	for s, ok := range map[string]bool{"": true, "09afaf": true, "/": false, ":": false, "`": false, "g": false, "@": false, "A": false, "F": false, "0": true, "9": true, "a": true, "f": true} {
		if isHex(s) != ok {
			t.Errorf("isHex(%q) != %v", s, ok)
		}
	}
}

func TestForeignPurposeAndClip(t *testing.T) {
	for id, want := range map[string]string{"plugins-0123": "plugins", "agent-x": "agent", "ci-node-1": "ci-node", "ci-audit-1": "ci-audit"} {
		if p, ok := foreignPurpose(id); !ok || string(p) != want {
			t.Errorf("%s: %q %v", id, p, ok)
		}
	}
	for _, id := range []string{"ci-release-0123", "plugins", "pluginsx-1", "", "xplugins-1", "unknown-1"} {
		if _, ok := foreignPurpose(id); ok {
			t.Errorf("%q is not another purpose", id)
		}
	}
	if clip(strings.Repeat("x", 80)) != strings.Repeat("x", 80) || clip(strings.Repeat("x", 81)) != strings.Repeat("x", 80)+"..." {
		t.Error("clip boundary")
	}
}

func TestClassifyEveryEvidenceReason(t *testing.T) {
	for reason, want := range map[attest.Reason]struct {
		key  string
		rank int
	}{
		attest.ReasonRevisionMismatch: {ReasonEvidenceMismatch, rankMismatch}, attest.ReasonRepoMismatch: {ReasonEvidenceMismatch, rankMismatch},
		authz.ReasonArtifactUnbound: {ReasonEvidenceMismatch, rankMismatch},
		attest.ReasonUnbound:        {ReasonEvidenceUnbound, rankUnbound}, attest.ReasonStale: {ReasonEvidenceStale, rankStale},
		attest.ReasonAdhoc: {ReasonEvidenceFailing, rankFailing}, attest.ReasonSelectionPartial: {ReasonEvidenceFailing, rankFailing},
		attest.ReasonNotPassing: {ReasonEvidenceFailing, rankFailing},
		attest.ReasonInvalid:    {ReasonEvidenceInvalid, rankInvalid}, attest.ReasonPolicyMismatch: {ReasonEvidenceInvalid, rankInvalid},
		attest.ReasonJobsMismatch: {ReasonEvidenceInvalid, rankInvalid}, attest.ReasonTrustBelowMin: {ReasonEvidenceInvalid, rankInvalid},
		attest.ReasonSelfReportedNotAdmitted: {ReasonEvidenceInvalid, rankInvalid}, attest.ReasonSignatureUnverifiable: {ReasonEvidenceInvalid, rankInvalid},
		"made.up": {ReasonEvidenceInvalid, rankInvalid},
	} {
		if key, rank := classify(reason); key != want.key || rank != want.rank {
			t.Errorf("%s: %s %d", reason, key, rank)
		}
	}
	if !(rankMismatch < rankInvalid && rankInvalid < rankUnbound && rankUnbound < rankStale && rankStale < rankFailing) {
		t.Error("severity order")
	}
	if got := reasons([]attest.Reason{attest.ReasonStale, attest.ReasonUnbound}); len(got) != 2 || got[0] != "evidence.stale" || got[1] != "evidence.unbound" {
		t.Errorf("reasons = %v", got)
	}
	if len(reasons(nil)) != 0 || reasons(nil) == nil {
		t.Error("reasons(nil) must be an empty list")
	}
}

func TestErrIs(t *testing.T) {
	base := errors.New("base")
	other := errors.New("other")
	if !errIs(base, other, base) || errIs(base, other) || errIs(nil, base) || errIs(base) {
		t.Error("errIs")
	}
}

type fakeGit struct {
	calls [][]string
	size  string
	sizeX int
	show  string
	showX int
	err   error
}

func (f *fakeGit) run(_ context.Context, argv ...string) (string, int, error) {
	f.calls = append(f.calls, argv)
	if f.err != nil {
		return "", 0, f.err
	}
	for i, a := range argv {
		if a == "cat-file" {
			return f.size, f.sizeX, nil
		}
		if a == "show" && i > 0 {
			return f.show, f.showX, nil
		}
	}
	return "", 1, nil
}

func TestPinnedReadsTheTrustFileAtTheCommit(t *testing.T) {
	pin := strings.Repeat("a", 40)
	ctx := context.Background()
	g := &fakeGit{size: "12\n", show: "the file"}
	o := Options{Root: "/repo", Pin: pin, Capture: g.run}
	b, ok, err := o.pinned(ctx, keysPath)
	if err != nil || !ok || string(b) != "the file" {
		t.Fatalf("%q %v %v", b, ok, err)
	}
	want := []string{"git", "-c", "core.fsmonitor=false", "-C", "/repo", "cat-file", "-s", pin + ":" + keysPath}
	if strings.Join(g.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("argv %v", g.calls[0])
	}
	if got := g.calls[1]; got[len(got)-2] != "show" || got[len(got)-1] != pin+":"+keysPath {
		t.Fatalf("show argv %v", got)
	}
	if _, ok, err = (Options{}).pinned(ctx, keysPath); ok || err != nil {
		t.Errorf("no pin must read nothing: %v %v", ok, err)
	}
	for name, o := range map[string]Options{
		"short pin":   {Root: "/r", Pin: "abc", Capture: g.run},
		"upper pin":   {Root: "/r", Pin: strings.ToUpper(pin), Capture: g.run},
		"option pin":  {Root: "/r", Pin: "--upload-pack=x", Capture: g.run},
		"no root":     {Pin: pin, Capture: g.run},
		"64-hex lies": {Root: "/r", Pin: strings.Repeat("a", 39), Capture: g.run},
	} {
		if _, ok, err := o.pinned(ctx, keysPath); ok || err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, ok, err := (Options{Root: "/r", Pin: strings.Repeat("a", 64), Capture: g.run}).pinned(ctx, keysPath); err != nil || !ok {
		t.Errorf("a 64-digit commit id is allowed: %v %v", ok, err)
	}
	for name, f := range map[string]*fakeGit{
		"file absent": {size: "", sizeX: 128},
		"git broke":   {err: errors.New("no git")},
	} {
		if _, ok, err := (Options{Root: "/r", Pin: pin, Capture: f.run}).pinned(ctx, keysPath); ok || err != nil {
			t.Errorf("%s: want no file and no error, got %v %v", name, ok, err)
		}
	}
	for name, f := range map[string]*fakeGit{
		"too big":      {size: "1048577", show: "x"},
		"size garbage": {size: "many", show: "x"},
		"show exit":    {size: "3", show: "", showX: 1},
		"show not ran": {size: "3", show: "x", err: nil, showX: 128},
	} {
		if _, ok, err := (Options{Root: "/r", Pin: pin, Capture: f.run}).pinned(ctx, keysPath); ok || err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, ok, err := (Options{Root: "/r", Pin: pin, Capture: (&fakeGit{size: "1048576", show: "x"}).run}).pinned(ctx, keysPath); !ok || err != nil {
		t.Errorf("a file of exactly the cap is allowed: %v %v", ok, err)
	}
}

func TestOperatorFile(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big")
	exact := filepath.Join(dir, "exact")
	if err := os.WriteFile(big, make([]byte, maxTrust+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exact, make([]byte, maxTrust), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := operatorFile(""); b != nil || err != nil {
		t.Errorf("empty path: %v %v", b, err)
	}
	if b, err := operatorFile(filepath.Join(dir, "absent")); b != nil || err != nil {
		t.Errorf("absent: %v %v", b, err)
	}
	if _, err := operatorFile(big); err == nil {
		t.Error("an oversize trust file was read")
	}
	if b, err := operatorFile(exact); err != nil || len(b) != maxTrust {
		t.Errorf("exact cap: %d %v", len(b), err)
	}
	if _, err := operatorFile(dir); err == nil {
		t.Error("a directory was read as a trust file")
	}
}

func TestFailureExit(t *testing.T) {
	if (&Failure{Reason: ReasonQuarantined}).ExitCode() != 1 || (&Failure{Reason: ReasonEvidenceFailing}).ExitCode() != 1 || (&Failure{Reason: ReasonSBOMLink}).ExitCode() != 11 {
		t.Error("exit mapping")
	}
	if ExitVerified != 0 || exitFailing != 1 || exitInvalid != 11 {
		t.Error("exit constants")
	}
	if Code != "E719" {
		t.Error("code")
	}
}
