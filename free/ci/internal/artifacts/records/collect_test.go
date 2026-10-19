package records_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func collect(e *env, path string) ([]records.Staged, error) {
	ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl(path)}}, false, nil)
	if err != nil {
		return nil, err
	}
	return e.collector().Collect(context.Background(), "proj", e.ws, ds)
}

func wantPath(t *testing.T, err error, reason authz.PathReason) {
	t.Helper()
	var pe *authz.PathError
	if !errors.As(err, &pe) || pe.Reason != reason || !strings.HasPrefix(err.Error(), "E683") {
		t.Fatalf("want E683 %s, got %v", reason, err)
	}
}

func TestCollectRefusesAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("host file"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.write("ok.txt", "fine")
	if err := os.Symlink("../../outside", filepath.Join(e.ws, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("ok.txt", filepath.Join(e.ws, "inlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(e.ws, "hard")); err != nil {
		t.Fatal(err)
	}
	if err := mkfifo(filepath.Join(e.ws, "pipe")); err != nil {
		t.Fatal(err)
	}
	e.write("dir/a.txt", "a")
	if err := os.Symlink("/etc/passwd", filepath.Join(e.ws, "dir", "abs")); err != nil {
		t.Fatal(err)
	}
	// declarations the sanitiser refuses before the job runs
	for _, p := range []string{"../../etc/passwd", "/etc/passwd", "a/../../b", "a\\b", "x/./../../y"} {
		if _, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl(p)}}, false, nil); err == nil || !strings.HasPrefix(err.Error(), "E683") {
			t.Fatalf("declaration %q: %v", p, err)
		}
	}
	cases := []struct {
		path   string
		reason authz.PathReason
	}{
		{"escape", authz.LinkEscape},
		{"inlink", authz.EntryIrregular},
		{"hard", authz.EntryHardlink},
		{"dir", authz.LinkAbsolute},
	}
	if fifoSupported {
		cases = append(cases, struct {
			path   string
			reason authz.PathReason
		}{"pipe", authz.EntryFIFO})
	}
	for _, c := range cases {
		_, err := collect(e, c.path)
		wantPath(t, err, c.reason)
	}
	if files := e.blobFiles(); len(files) != 0 {
		t.Fatalf("refused collections stored %v", files)
	}
	if b, _ := os.ReadFile(outside); string(b) != "host file" {
		t.Fatal("a file outside the workspace changed")
	}
}

func TestCollectMissingCapsAndDisk(t *testing.T) {
	e := newEnv(t)
	e.write("big.bin", strings.Repeat("x", 2048))
	if _, err := collect(e, "nothing/*.zip"); !records.IsReason(err, records.ReasonMissing) {
		t.Fatalf("no match: %v", err)
	}
	e.pol.MaxFile = 1024
	if _, err := collect(e, "big.bin"); !records.IsReason(err, records.ReasonFileCap) {
		t.Fatalf("over the file cap: %v", err)
	}
	e.pol.MaxFile = 1 << 20
	low := e.pol
	low.Statfs = func(string) (uint64, uint64, error) { return 1 << 30, 100 << 30, nil } // 1 GiB free of 100 GiB: below the 5 GiB / 10 % floor
	e.pol = low
	if _, err := collect(e, "big.bin"); !records.IsReason(err, records.ReasonDiskFloor) {
		t.Fatalf("below the disk floor: %v", err)
	}
	if files := e.blobFiles(); len(files) != 0 {
		t.Fatalf("a refused write landed bytes: %v", files)
	}
}

func TestCollectSecretRawAndBase64(t *testing.T) {
	e := newEnv(t)
	e.pol.MaxFile = 4 << 20
	secret := "ghs-planted-registered-value-0123456789"
	if err := e.red.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	e.write("clean.txt", "nothing to see")
	e.write("raw.txt", "prefix "+secret+" suffix")
	e.write("b64.txt", "prefix "+base64.StdEncoding.EncodeToString([]byte(secret))+" suffix")
	// a value that straddles the 1 MiB window edge is still found
	e.write("edge.txt", strings.Repeat("a", (1<<20)-10)+secret+strings.Repeat("b", 100))
	for _, f := range []string{"raw.txt", "b64.txt", "edge.txt"} {
		if _, err := collect(e, f); !records.IsReason(err, records.ReasonSecretMatch) {
			t.Fatalf("%s: %v", f, err)
		} else if strings.Contains(err.Error(), secret) {
			t.Fatalf("the error leaks the value: %v", err)
		}
	}
	if len(e.blobFiles()) != 0 {
		t.Fatal("a file holding a secret was stored")
	}
	if _, err := collect(e, "clean.txt"); err != nil {
		t.Fatal(err)
	}
	// no scanner: nothing is stored
	c := e.collector()
	c.Scanner = nil
	ds, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl("clean.txt")}}, false, nil)
	if _, err := c.Collect(context.Background(), "p2", e.ws, ds); !records.IsReason(err, records.ReasonSecretMatch) {
		t.Fatalf("nil scanner: %v", err)
	}
}

func TestCollectGlobNamesAndDuplicates(t *testing.T) {
	e := newEnv(t)
	e.write("dist/app", "bin")
	e.write("dist/lib/a.so", "so")
	e.write("dist/docs/x.md", "md")
	e.write("report.xml", "<r/>")
	e.write("deep/one/two/cov.out", "cov")
	ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{
		{Path: "report.xml", Name: "test-report", Kind: "report"},
		{Path: "dist", Name: "release"},
		{Path: "deep/**/cov.out"},
		{Path: "dist/lib/*.so", Name: "libs"},
	}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.collector().Collect(context.Background(), "proj", e.ws, ds)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
		if s.SHA256 == "" || s.Size == 0 {
			t.Fatalf("unstored: %+v", s)
		}
	}
	want := "test-report release/app release/docs/x.md release/lib/a.so deep/one/two/cov.out libs/a.so"
	if strings.Join(names, " ") != want {
		t.Fatalf("names\n got %v\nwant %s", names, want)
	}
	// two declarations that produce one name are refused
	dup, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "report.xml", Name: "same"}, {Path: "dist/app", Name: "same"}}}, false, nil)
	if _, err = e.collector().Collect(context.Background(), "proj", e.ws, dup); !records.IsReason(err, records.ReasonDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	// an image declaration must select exactly one file
	img, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "dist", Kind: "image"}}}, false, nil)
	if _, err = e.collector().Collect(context.Background(), "proj", e.ws, img); !records.IsReason(err, records.ReasonImage) {
		t.Fatalf("image with many files: %v", err)
	}
}

func TestNormalizeDefaultsAndRefusals(t *testing.T) {
	ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "a/"}, {Path: "b", Retention: "keep", Visibility: "restricted"}}}, false, []model.ArtifactDecl{{Path: "sbom.json", Kind: "sbom"}})
	if err != nil || len(ds) != 3 {
		t.Fatalf("%+v %v", ds, err)
	}
	if ds[0].Path != "a" || ds[0].Retention != "30d" || ds[0].Visibility != authz.VisibilityProject || ds[0].Kind != records.KindOther {
		t.Fatalf("defaults: %+v", ds[0])
	}
	if ds[1].Retention != "keep" || ds[1].Visibility != authz.VisibilityRestricted || ds[2].Kind != records.KindSBOM || ds[2].Ord != 2 {
		t.Fatalf("explicit: %+v %+v", ds[1], ds[2])
	}
	if rel, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "a"}}}, true, nil); rel[0].Retention != "keep" {
		t.Fatalf("a release job keeps by default: %+v", rel)
	}
	if rel, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "a", Retention: "7d"}}}, true, nil); rel[0].Retention != "7d" {
		t.Fatalf("a release job honours its own retention: %+v", rel)
	}
	for _, bad := range []model.ArtifactDecl{{Path: "a", Kind: "weird"}, {Path: "a", Visibility: "public"}, {Path: "a", Retention: "forever"}, {Path: "a", Retention: "0d"},
		{Path: "a", Image: &model.ArtifactImage{Ref: "r/x"}}, {Path: "."}, {Path: ""}, {Path: "a", Name: "../x"}} {
		if _, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{bad}}, false, nil); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	for s, want := range map[string]string{"30d": "720h0m0s", "2w": "336h0m0s", "12h": "12h0m0s", "90m": "1h30m0s", "45s": "45s"} {
		d, keep, err := records.RetentionDuration(s)
		if err != nil || keep || d.String() != want {
			t.Fatalf("%s: %v %v %v", s, d, keep, err)
		}
	}
	if _, keep, err := records.RetentionDuration("keep"); err != nil || !keep {
		t.Fatal("keep")
	}
	for _, s := range []string{"", "d", "30", "-1d", "x5d", "99999d", "5y"} {
		if _, _, err := records.RetentionDuration(s); err == nil {
			t.Fatalf("accepted retention %q", s)
		}
	}
}

func TestImplicitDeclarations(t *testing.T) {
	records.RegisterImplicit(func(j model.Job) []model.ArtifactDecl {
		if j.Kind != "build" {
			return nil
		}
		return []model.ArtifactDecl{{Path: "sbom.cdx.json", Kind: "sbom"}}
	})
	if got := records.ImplicitDecls(model.Job{Kind: "build"}); len(got) != 1 || got[0].Path != "sbom.cdx.json" {
		t.Fatalf("%+v", got)
	}
	if got := records.ImplicitDecls(model.Job{Kind: "test"}); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestCollectTotalCapBoundary(t *testing.T) {
	e := newEnv(t)
	e.write("a", strings.Repeat("a", 60))
	e.write("b", strings.Repeat("b", 40))
	e.pol.MaxFile, e.pol.MaxTotal = 100, 100
	ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl("a"), decl("b")}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.collector().Collect(context.Background(), "proj", e.ws, ds); err != nil {
		t.Fatalf("exactly the cap: %v", err)
	}
	e.pol.MaxTotal = 99
	if _, err = e.collector().Collect(context.Background(), "proj", e.ws, ds); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("one byte over: %v", err)
	}
}

func TestCollectRefusesSecretInNameAndSymlinkedWorkdir(t *testing.T) {
	e := newEnv(t)
	const secret = "registered-name-secret-123456"
	if err := e.red.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	e.write("out/"+secret, "benign bytes")
	e.write("fine/x", "x")
	if _, err := collect(e, "out/*"); !records.IsReason(err, records.ReasonSecretMatch) {
		t.Fatalf("secret in a file name: %v", err)
	}
	ds, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "fine/x", Name: "n-" + secret}}}, false, nil)
	if _, err := e.collector().Collect(context.Background(), "proj", e.ws, ds); !records.IsReason(err, records.ReasonSecretMatch) {
		t.Fatalf("secret in a declared name: %v", err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "data"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "ws")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	ds, _ = records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl("data")}}, false, nil)
	if _, err := e.collector().Collect(context.Background(), "proj", link, ds); !records.IsReason(err, records.ReasonUnreadable) {
		t.Fatalf("symlinked workdir: %v", err)
	}
	if _, err := e.collector().Collect(context.Background(), "proj", filepath.Join(e.ws, "fine", "x"), ds); !records.IsReason(err, records.ReasonUnreadable) {
		t.Fatalf("a file as workdir: %v", err)
	}
	if len(e.blobFiles()) != 0 {
		t.Fatal("something was stored")
	}
}

func TestCollectFindsALongSecretAcrossTheWindowEdge(t *testing.T) {
	e := newEnv(t)
	e.pol.MaxFile = 64 << 20
	long := strings.Repeat("z", redact.MaxValueLen-1) + "Q" // the longest value Register accepts
	if err := e.red.Register([]byte(long)); err != nil {
		t.Fatal(err)
	}
	edge := 16 << 20 // the window size
	e.write("edge.bin", strings.Repeat("!", edge-redact.MaxValueLen/2)+long+strings.Repeat("!", 100))
	if _, err := collect(e, "edge.bin"); !records.IsReason(err, records.ReasonSecretMatch) {
		t.Fatalf("%v", err)
	}
	if len(e.blobFiles()) != 0 {
		t.Fatal("a blob holding the secret was published")
	}
}

func TestCollectHonoursTheStoreBudget(t *testing.T) {
	e := newEnv(t)
	e.pol.MaxTotal, e.pol.MaxFile = 100, 100
	e.write("a", strings.Repeat("a", 60))
	c := e.collector()
	c.Usage = records.Usage{Total: 50}
	ds, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl("a")}}, false, nil)
	if _, err := c.Collect(context.Background(), "proj", e.ws, ds); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("declared bytes over the remaining room: %v", err)
	}
	if len(e.blobFiles()) != 0 {
		t.Fatal("bytes were written before the quota check")
	}
	// a file that grows after planning is stopped by the streamed budget
	c.Usage = records.Usage{Total: 30}
	if _, err := c.Collect(context.Background(), "proj", e.ws, ds); err != nil {
		t.Fatalf("60 B fit in 70 B of room: %v", err)
	}
	e.write("b", strings.Repeat("b", 40))
	two, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl("a"), decl("b")}}, false, nil)
	c.Usage = records.Usage{Total: 0}
	if _, err := c.Collect(context.Background(), "proj", e.ws, two); err != nil {
		t.Fatalf("exactly the cap: %v", err)
	}
	c.Usage = records.Usage{Total: 1}
	if _, err := c.Collect(context.Background(), "proj", e.ws, two); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("one over: %v", err)
	}
}

func TestCollectNeverEchoesASecretNamedFile(t *testing.T) {
	e := newEnv(t)
	const secret = "registered-opus-secret-987654"
	if err := e.red.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	e.pol.MaxFile = 10
	e.write("out/"+secret, strings.Repeat("x", 100)) // over the file cap
	if err := os.Symlink("real", filepath.Join(e.ws, "out", "ln-"+secret)); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"out/*", "out/" + secret, "out/ln-" + secret} {
		_, err := collect(e, p)
		if !records.IsReason(err, records.ReasonSecretMatch) || strings.Contains(err.Error(), secret) {
			t.Fatalf("%s: %v", p, err)
		}
	}
}

func TestCollectBoundsRecordsAndReservesBeforeWriting(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 5; i++ {
		e.write(fmt.Sprintf("d/f%d", i), "x")
	}
	c := e.collector()
	c.MaxRecords = 4
	ds, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl("d")}}, false, nil)
	if _, err := c.Collect(context.Background(), "proj", e.ws, ds); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("5 files over a 4-record budget: %v", err)
	}
	c.MaxRecords = 5
	var reserved int64 = -1
	c.Reserve = func(total int64) error { reserved = total; return nil }
	if got, err := c.Collect(context.Background(), "proj", e.ws, ds); err != nil || len(got) != 5 || reserved != 5 {
		t.Fatalf("exactly the budget: %d %v reserved %d", len(got), err, reserved)
	}
	// the budget spans declarations
	two, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "d", Name: "a"}, {Path: "d", Name: "b"}}}, false, nil)
	if _, err := c.Collect(context.Background(), "proj", e.ws, two); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("two declarations of 5 files: %v", err)
	}
	// a refused reservation writes nothing
	c2 := e.collector()
	home2 := t.TempDir()
	blobs2, _ := cas.New(home2)
	c2.Blobs = blobs2
	c2.Reserve = func(int64) error { return records.Failf(records.ReasonSizeCap, "claimed by another attempt") }
	if _, err := c2.Collect(context.Background(), "proj", e.ws, ds); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("%v", err)
	}
	if f, _ := filepath.Glob(filepath.Join(home2, "blobs", "artifacts-*", "sha256", "*", "*")); len(f) != 0 {
		t.Fatalf("bytes were written after a refused reservation: %v", f)
	}
	// a cancelled context stops the expansion
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.collector().Collect(ctx, "proj", e.ws, ds); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	many := make([]model.ArtifactDecl, records.MaxDecls+1)
	for i := range many {
		many[i] = decl("d")
	}
	if _, err := records.Normalize(model.Job{Artifacts: many[:records.MaxDecls]}, false, nil); err != nil {
		t.Fatalf("exactly MaxDecls: %v", err)
	}
	if _, err := records.Normalize(model.Job{Artifacts: many}, false, nil); !records.IsReason(err, records.ReasonDeclaration) {
		t.Fatalf("one over MaxDecls: %v", err)
	}
}

func TestCollectBudgetAppliesToGlobsToo(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 6; i++ {
		e.write(fmt.Sprintf("d/f%d", i), "x")
	}
	c := e.collector()
	c.MaxRecords = 5
	one, _ := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{decl("d/*")}}, false, nil)
	if _, err := c.Collect(context.Background(), "proj", e.ws, one); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("a glob of 6 files over a budget of 5: %v", err)
	}
	c.MaxRecords = 6
	if got, err := c.Collect(context.Background(), "proj", e.ws, one); err != nil || len(got) != 6 {
		t.Fatalf("a glob of exactly the budget: %d %v", len(got), err)
	}
	e.write("d/other", "different bytes") // a new blob a refused plan must not store
	before := len(e.blobFiles())
	// forty declarations of the same glob exceed the budget long before they are all expanded
	many := make([]model.ArtifactDecl, 40)
	for i := range many {
		many[i] = model.ArtifactDecl{Path: "d/*", Name: fmt.Sprintf("n%d", i)}
	}
	ds, _ := records.Normalize(model.Job{Artifacts: many}, false, nil)
	c.MaxRecords = 20
	if _, err := c.Collect(context.Background(), "proj", e.ws, ds); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("40 globs of 6 files over a budget of 20: %v", err)
	}
	if n := len(e.blobFiles()); n != before {
		t.Fatalf("a refused plan stored bytes: %d blobs, was %d", n, before)
	}
}
