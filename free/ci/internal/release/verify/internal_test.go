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
	if _, err := open(dir, "real"); err == nil {
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
	if _, err := parseSums([]byte(a + "  a\n" + b + " b\n")); err == nil || !strings.Contains(err.Error(), "line 2 ") {
		t.Errorf("a bad second line must name line 2: %v", err)
	}
	if _, err := parseSums([]byte(a + "  b\n" + a + "  a\n")); err == nil || !strings.Contains(err.Error(), "line 2 ") {
		t.Errorf("a misordered second line must name line 2: %v", err)
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

func TestRankOfEveryEvidenceReason(t *testing.T) {
	for reason, want := range map[attest.Reason]int{
		attest.ReasonRevisionMismatch: rankMismatch, attest.ReasonRepoMismatch: rankMismatch, authz.ReasonArtifactUnbound: rankMismatch,
		attest.ReasonUnbound: rankUnbound, attest.ReasonStale: rankStale,
		attest.ReasonAdhoc: rankFailing, attest.ReasonSelectionPartial: rankFailing, attest.ReasonNotPassing: rankFailing,
		attest.ReasonInvalid: rankInvalid, attest.ReasonPolicyMismatch: rankInvalid, attest.ReasonJobsMismatch: rankInvalid,
		attest.ReasonTrustBelowMin: rankInvalid, attest.ReasonSelfReportedNotAdmitted: rankInvalid, attest.ReasonSignatureUnverifiable: rankInvalid,
		"made.up": rankInvalid,
	} {
		if got := rankOf(reason); got != want {
			t.Errorf("%s: rank %d, want %d", reason, got, want)
		}
	}
	keys := map[int]string{rankMismatch: ReasonEvidenceMismatch, rankInvalid: ReasonEvidenceInvalid, rankUnbound: ReasonEvidenceUnbound, rankStale: ReasonEvidenceStale, rankFailing: ReasonEvidenceFailing}
	for rank, key := range keys {
		if rankKeys[rank] != key {
			t.Errorf("rank %d names %q, want %q", rank, rankKeys[rank], key)
		}
	}
	if len(rankKeys) != rankNone || rankNone != 5 {
		t.Error("rankKeys must cover every rank")
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

// fakeGit answers the four git calls pinned() makes.
type fakeGit struct {
	commitX int    // exit of `cat-file -e <pin>^{commit}`
	listed  string // output of `ls-tree`
	listX   int
	size    string // output of `cat-file -s`
	sizeX   int
	show    string
	showX   int
	err     error
	calls   [][]string
}

func (f *fakeGit) run(_ context.Context, argv ...string) (string, int, error) {
	f.calls = append(f.calls, argv)
	if f.err != nil {
		return "", 0, f.err
	}
	has := func(w string) bool {
		for _, a := range argv {
			if a == w {
				return true
			}
		}
		return false
	}
	switch {
	case has("-e"):
		return "", f.commitX, nil
	case has("ls-tree"):
		return f.listed, f.listX, nil
	case has("-s"):
		return f.size, f.sizeX, nil
	case has("show"):
		return f.show, f.showX, nil
	}
	return "", 1, nil
}

func okGit() *fakeGit {
	return &fakeGit{listed: "100644 blob abc\t" + keysPath + "\n", size: "12\n", show: "the file"}
}

func TestPinnedReadsTheTrustFileAtTheCommit(t *testing.T) {
	pin := strings.Repeat("a", 40)
	ctx := context.Background()
	g := okGit()
	o := Options{Root: "/repo", Pin: pin, Capture: g.run}
	b, ok, err := o.pinned(ctx, keysPath)
	if err != nil || !ok || string(b) != "the file" {
		t.Fatalf("%q %v %v", b, ok, err)
	}
	base := []string{"git", "--no-replace-objects", "-c", "core.fsmonitor=false", "-C", "/repo"}
	spec := pin + ":./" + keysPath
	for i, want := range [][]string{
		append(append([]string{}, base...), "cat-file", "-e", pin+"^{commit}"),
		append(append([]string{}, base...), "ls-tree", pin, "--", keysPath),
		append(append([]string{}, base...), "cat-file", "-s", spec),
		append(append([]string{}, base...), "show", "--no-textconv", spec),
	} {
		if len(g.calls) != 4 || strings.Join(g.calls[i], " ") != strings.Join(want, " ") {
			t.Fatalf("call %d: %v, want %v", i, g.calls, want)
		}
	}
	if _, ok, err = (Options{}).pinned(ctx, keysPath); ok || err != nil {
		t.Errorf("no pin must read nothing: %v %v", ok, err)
	}
	for name, o := range map[string]Options{
		"short pin":   {Root: "/r", Pin: "abc", Capture: okGit().run},
		"upper pin":   {Root: "/r", Pin: strings.ToUpper(pin), Capture: okGit().run},
		"option pin":  {Root: "/r", Pin: "--upload-pack=x", Capture: okGit().run},
		"no root":     {Pin: pin, Capture: okGit().run},
		"39 digits":   {Root: "/r", Pin: strings.Repeat("a", 39), Capture: okGit().run},
		"branch name": {Root: "/r", Pin: "main", Capture: okGit().run},
	} {
		if _, ok, err := o.pinned(ctx, keysPath); ok || err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, ok, err := (Options{Root: "/r", Pin: strings.Repeat("a", 64), Capture: okGit().run}).pinned(ctx, keysPath); err != nil || !ok {
		t.Errorf("a 64-digit commit id is allowed: %v %v", ok, err)
	}
	// Absence is proved by an empty listing at an existing commit, and only then.
	if _, ok, err := (Options{Root: "/r", Pin: pin, Capture: (&fakeGit{listed: "\n"}).run}).pinned(ctx, keysPath); ok || err != nil {
		t.Errorf("an empty listing means absent: %v %v", ok, err)
	}
	for name, f := range map[string]*fakeGit{
		"git broke":      {err: errors.New("no git")},
		"pin not found":  {commitX: 128},
		"listing failed": {listX: 128, listed: ""},
		"symlink entry":  {listed: "120000 blob abc\t" + keysPath},
		"tree entry":     {listed: "040000 tree abc\t" + keysPath},
		"too big":        {listed: "100644 blob abc\tx", size: "1048577", show: "x"},
		"size garbage":   {listed: "100644 blob abc\tx", size: "many", show: "x"},
		"size exit":      {listed: "100644 blob abc\tx", size: "3", sizeX: 128, show: "x"},
		"show exit":      {listed: "100644 blob abc\tx", size: "3", show: "", showX: 1},
	} {
		if _, ok, err := (Options{Root: "/r", Pin: pin, Capture: f.run}).pinned(ctx, keysPath); ok || err == nil {
			t.Errorf("%s accepted: %v %v", name, ok, err)
		}
	}
	for _, listed := range []string{"100644 blob abc\tx", "100755 blob abc\tx"} {
		if _, ok, err := (Options{Root: "/r", Pin: pin, Capture: (&fakeGit{listed: listed, size: "1048576", show: "x"}).run}).pinned(ctx, keysPath); !ok || err != nil {
			t.Errorf("%q at the size cap is allowed: %v %v", listed, ok, err)
		}
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
	if err := os.Symlink(exact, filepath.Join(dir, "link")); err == nil {
		if b, err := operatorFile(filepath.Join(dir, "link")); err != nil || len(b) != maxTrust {
			t.Errorf("a symlink to a regular file is allowed: %d %v", len(b), err)
		}
	}
	if err := os.Symlink("/dev/zero", filepath.Join(dir, "zero")); err == nil {
		if _, err := operatorFile(filepath.Join(dir, "zero")); err == nil {
			t.Error("a symlink to a device was read as a trust file")
		}
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

func TestCheckSize(t *testing.T) {
	dir := t.TempDir()
	r := &run{dir: dir}
	r.man.Evidence.Path, r.man.Provenance.Path = "evidence.json", "prov.json"
	put := func(name string, n int64) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(n); err != nil { // sparse: no disk used
			t.Fatal(err)
		}
		_ = f.Close()
	}
	put("art", 5)
	put("evidence.json", maxEvidence)
	put("evidence-big.json", maxEvidence+1)
	put("prov.json", maxEnvelope)
	put("prov-big.json", maxEnvelope+1)
	put("sbom-ok", maxEvidence+1)
	put("sbom-big", maxSBOM+1)
	put("sbom-edge", maxSBOM)
	sizes := map[string]int64{"art": 5, "art-wrong": 4}
	put("art-wrong", 5)
	for rel, ok := range map[string]bool{"art": true, "art-wrong": false, "evidence.json": true, "prov.json": true, "sbom-ok": true, "sbom-edge": true, "sbom-big": false, "missing": true} {
		if f := r.checkSize(rel, sizes); (f == nil) != ok {
			t.Errorf("%s: %v", rel, f)
		}
	}
	r.man.Evidence.Path, r.man.Provenance.Path = "evidence-big.json", "prov-big.json"
	for _, rel := range []string{"evidence-big.json", "prov-big.json"} {
		if r.checkSize(rel, sizes) == nil {
			t.Errorf("%s over its cap accepted", rel)
		}
	}
}
