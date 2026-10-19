package records_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestTwoAttemptsSamePathTwoRecords(t *testing.T) {
	e := newEnv(t)
	e.write("out/app", "version one")
	first := e.commit("a1", model.ArtifactDecl{Path: "out/app"})
	e.write("out/app", "version two")
	second := e.commit("a2", model.ArtifactDecl{Path: "out/app"})
	if len(first) != 1 || len(second) != 1 || first[0].ID == second[0].ID || first[0].SHA256 == second[0].SHA256 {
		t.Fatalf("records: %+v %+v", first, second)
	}
	if first[0].State != records.StateCommitted || first[0].Attempt != "a1" || first[0].Project != "proj" || first[0].Revision != "rev1" || first[0].Run != "pa1" || first[0].Job != "a1" {
		t.Fatalf("facts: %+v", first[0])
	}
	got, err := e.st.ArtifactList(context.Background(), records.Filter{Name: "out/app"})
	if err != nil || len(got) != 2 {
		t.Fatalf("both versions must be listed: %+v %v", got, err)
	}
	if first[0].ExpiresAt == nil || !first[0].ExpiresAt.Equal(e.now.Add(30*24*time.Hour)) {
		t.Fatalf("30 d default retention: %v", first[0].ExpiresAt)
	}
	if first[0].Platform != "linux/amd64" || first[0].Isolation != "process" || first[0].Producer != "nself-ci/a1" {
		t.Fatalf("environment facts: %+v", first[0])
	}
}

func TestCommitFailsOnMissingOrTamperedBlob(t *testing.T) {
	ctx := context.Background()
	prepare := func(id string) *env {
		e := newEnv(t)
		e.write("out/app", "payload bytes")
		facts, ds := e.attempt(id, "test", model.ArtifactDecl{Path: "out/app"})
		if _, err := e.stage(id, facts, ds); err != nil {
			t.Fatal(err)
		}
		e.finalizing(id)
		return e
	}
	t.Run("blob removed", func(t *testing.T) {
		e := prepare("m1")
		for _, f := range e.blobFiles() {
			_ = os.Remove(f)
		}
		if _, err := records.CommitAndVerify(ctx, e.st, e.blobs, "m1", 3, e.pol); !records.IsReason(err, records.ReasonMissingBlob) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("blob grown", func(t *testing.T) {
		e := prepare("m2")
		for _, f := range e.blobFiles() {
			if err := os.WriteFile(f, []byte("payload bytes plus more"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := records.CommitAndVerify(ctx, e.st, e.blobs, "m2", 3, e.pol); !records.IsReason(err, records.ReasonDigest) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("blob rewritten at the same size", func(t *testing.T) {
		e := prepare("m3")
		for _, f := range e.blobFiles() {
			if err := os.WriteFile(f, []byte("PAYLOAD BYTES"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := records.CommitAndVerify(ctx, e.st, e.blobs, "m3", 3, e.pol); !records.IsReason(err, records.ReasonDigest) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("declared but never collected", func(t *testing.T) {
		e := newEnv(t)
		e.attempt("m4", "test", model.ArtifactDecl{Path: "out/app"})
		e.finalizing("m4")
		if _, err := records.CommitAndVerify(ctx, e.st, e.blobs, "m4", 3, e.pol); !records.IsReason(err, records.ReasonMissing) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("stale epoch is refused with an audit row", func(t *testing.T) {
		e := prepare("m5")
		_, err := records.CommitAndVerify(ctx, e.st, e.blobs, "m5", 2, e.pol)
		if !records.IsReason(err, records.ReasonEpoch) || !strings.HasPrefix(err.Error(), "E711") {
			t.Fatalf("%v", err)
		}
		var n int
		if e.st != nil {
			recs, _ := e.st.ArtifactStaged(ctx, "m5")
			n = len(recs)
		}
		if n != 1 {
			t.Fatalf("the staged record must stay staged: %d", n)
		}
	})
	t.Run("nothing declared commits nothing", func(t *testing.T) {
		e := newEnv(t)
		e.attempt("m6", "test")
		e.finalizing("m6")
		recs, err := records.CommitAndVerify(ctx, e.st, e.blobs, "m6", 3, e.pol)
		if err != nil || recs != nil {
			t.Fatalf("%v %v", recs, err)
		}
	})
}

func TestCommitWarnsAtEightyPercentAndRefusesWhenFull(t *testing.T) {
	e := newEnv(t)
	e.pol.MaxTotal = 1000
	e.pol.MaxFile = 1000
	var warned []string
	e.pol.Warn = func(w string) { warned = append(warned, w) }
	e.write("one", strings.Repeat("a", 850))
	e.commit("w1", model.ArtifactDecl{Path: "one"})
	if len(warned) != 1 || !strings.Contains(warned[0], "85%") || !strings.Contains(warned[0], "prune") {
		t.Fatalf("warning: %v", warned)
	}
	e.write("two", strings.Repeat("b", 400))
	facts, ds := e.attempt("w2", "test", model.ArtifactDecl{Path: "two"})
	if _, err := e.stage("w2", facts, ds); err != nil {
		t.Fatal(err)
	}
	e.finalizing("w2")
	_, err := records.CommitAndVerify(context.Background(), e.st, e.blobs, "w2", 3, e.pol)
	if !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("full store: %v", err)
	}
}

func TestFullStoreOfReleaseRecordsNamesPruneRelease(t *testing.T) {
	e := newEnv(t)
	e.pol.MaxTotal, e.pol.MaxFile = 1000, 1000
	e.write("rel.bin", strings.Repeat("r", 900))
	facts, ds := e.attempt("r1", "release", model.ArtifactDecl{Path: "rel.bin"})
	if !facts.Release || ds[0].Retention != "keep" {
		t.Fatalf("release facts: %+v %+v", facts, ds)
	}
	if _, err := e.stage("r1", facts, ds); err != nil {
		t.Fatal(err)
	}
	e.finalizing("r1")
	recs, err := records.CommitAndVerify(context.Background(), e.st, e.blobs, "r1", 3, e.pol)
	if err != nil || recs[0].ExpiresAt != nil || !recs[0].Release {
		t.Fatalf("release record: %+v %v", recs, err)
	}
	e.write("more.bin", strings.Repeat("m", 300))
	f2, d2 := e.attempt("r2", "test", model.ArtifactDecl{Path: "more.bin"})
	if _, err = e.stage("r2", f2, d2); err != nil {
		t.Fatal(err)
	}
	e.finalizing("r2")
	_, err = records.CommitAndVerify(context.Background(), e.st, e.blobs, "r2", 3, e.pol)
	if !records.IsReason(err, records.ReasonSizeCap) || !strings.Contains(err.Error(), "prune --release") {
		t.Fatalf("must name prune --release: %v", err)
	}
}

func TestSecretNeverReachesTheStore(t *testing.T) {
	e := newEnv(t)
	secret := "planted-registered-secret-value-4242"
	if err := e.red.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	e.write("leak.txt", "token="+secret)
	facts, ds := e.attempt("s1", "test", model.ArtifactDecl{Path: "leak.txt"})
	_, err := e.stage("s1", facts, ds)
	if !records.IsReason(err, records.ReasonSecretMatch) {
		t.Fatalf("%v", err)
	}
	// no sink holds the value: the database file, the audit log, the error, the blob tree
	for _, p := range []string{e.home + "/state.db", e.home + "/state.db-wal"} {
		if b, _ := os.ReadFile(p); strings.Contains(string(b), secret) {
			t.Fatalf("%s holds the secret", p)
		}
	}
	if len(e.blobFiles()) != 0 || strings.Contains(err.Error(), secret) {
		t.Fatal("blob stored or value in the error")
	}
}

func TestReaderVisibilityAndGet(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.write("pub.txt", "public bytes")
	e.write("sec/key.txt", "restricted bytes")
	pub := e.commit("v1", model.ArtifactDecl{Path: "pub.txt"})[0]
	res := e.commit("v2", model.ArtifactDecl{Path: "sec/key.txt", Visibility: "restricted"})[0]
	rd := records.Reader{Ledger: e.st, Blobs: e.blobs}
	member := authz.Viewer{Project: "proj", Role: authz.RoleMember}
	release := authz.Viewer{Project: "proj", Role: authz.RoleRelease}
	other := authz.Viewer{Project: "other", Role: authz.RoleAdmin}

	// list: a member sees only the project-visible record
	if got, err := rd.List(ctx, member, records.Filter{}); err != nil || len(got) != 1 || got[0].ID != pub.ID {
		t.Fatalf("member list: %+v %v", got, err)
	}
	if got, _ := rd.List(ctx, release, records.Filter{}); len(got) != 2 {
		t.Fatalf("release list: %+v", got)
	}
	if got, _ := rd.List(ctx, other, records.Filter{}); len(got) != 0 {
		t.Fatalf("other project list: %+v", got)
	}
	// not found and not visible are the same value, byte for byte
	_, errMissing := rd.Show(ctx, member, "art_does_not_exist")
	_, errHidden := rd.Show(ctx, member, res.ID)
	_, errOther := rd.Show(ctx, other, pub.ID)
	if errMissing != records.ErrNotFound || errHidden != errMissing || errOther != errMissing || !strings.HasPrefix(errMissing.Error(), "E713") {
		t.Fatalf("%v | %v | %v", errMissing, errHidden, errOther)
	}
	if _, _, err := rd.Get(ctx, member, res.ID, t.TempDir()+"/d", t.TempDir()); err != records.ErrNotFound {
		t.Fatalf("get of a hidden record: %v", err)
	}
	// get: refuses cwd, a non-empty dir, and a path under .git
	cwd := t.TempDir()
	if err := os.WriteFile(cwd+"/x", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dest := range []string{cwd, ".", cwd + "/.git/objects", cwd + "/.git"} {
		if _, _, err := rd.Get(ctx, member, pub.ID, dest, cwd); err == nil || !strings.HasPrefix(err.Error(), "E683") {
			t.Fatalf("dest %q: %v", dest, err)
		}
	}
	dest := t.TempDir() + "/fresh"
	path, rec, err := rd.Get(ctx, release, res.ID, dest, cwd)
	if err != nil || rec.ID != res.ID {
		t.Fatalf("%v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "restricted bytes" || !strings.HasSuffix(path, "/fresh/sec/key.txt") {
		t.Fatalf("written %q to %s", b, path)
	}
	// an existing empty directory is fine; a second get into it is refused (no longer empty)
	empty := t.TempDir()
	if _, _, err = rd.Get(ctx, member, pub.ID, empty, cwd); err != nil {
		t.Fatal(err)
	}
	if _, _, err = rd.Get(ctx, member, pub.ID, empty, cwd); err == nil {
		t.Fatal("get into a non-empty directory")
	}
}

func TestReaderMissingBlobAfterRestore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.write("a.txt", "bytes")
	rec := e.commit("g1", model.ArtifactDecl{Path: "a.txt"})[0]
	// a store restore without blobs
	if err := os.RemoveAll(e.home + "/blobs"); err != nil {
		t.Fatal(err)
	}
	rd := records.Reader{Ledger: e.st, Blobs: e.blobs}
	shown, err := rd.Show(ctx, viewer, rec.ID)
	if err != nil || !shown.Missing {
		t.Fatalf("show must report missing: %+v %v", shown, err)
	}
	if stored, _, _ := e.st.ArtifactGet(ctx, rec.ID); !stored.Missing {
		t.Fatal("the missing flag was not remembered")
	}
	if _, _, err = rd.Get(ctx, viewer, rec.ID, t.TempDir()+"/d", t.TempDir()); !records.IsReason(err, records.ReasonMissingBlob) {
		t.Fatalf("get: %v", err)
	}
	if list, _ := rd.List(ctx, viewer, records.Filter{}); len(list) != 1 || !list[0].Missing {
		t.Fatalf("list: %+v", list)
	}
}

func TestGetReverifiesTheDigest(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "original bytes")
	rec := e.commit("h1", model.ArtifactDecl{Path: "a.txt"})[0]
	for _, f := range e.blobFiles() {
		if err := os.WriteFile(f, []byte("tampered bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dest := t.TempDir() + "/out"
	rd := records.Reader{Ledger: e.st, Blobs: e.blobs}
	if _, _, err := rd.Get(context.Background(), viewer, rec.ID, dest, t.TempDir()); !records.IsReason(err, records.ReasonDigest) {
		t.Fatalf("%v", err)
	}
	if ents, _ := os.ReadDir(dest); len(ents) != 0 {
		t.Fatalf("a tampered blob left files: %v", ents)
	}
}

func TestPruneRemovesRecordsAndBlobs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.write("short.txt", "short lived")
	e.write("keep.txt", "kept")
	e.write("rel.txt", "release bytes")
	e.commit("p1", model.ArtifactDecl{Path: "short.txt", Retention: "1d"})
	e.commit("p2", model.ArtifactDecl{Path: "keep.txt", Retention: "keep"})
	facts, ds := e.attempt("p3", "release", model.ArtifactDecl{Path: "rel.txt", Retention: "1d"})
	if _, err := e.stage("p3", facts, ds); err != nil {
		t.Fatal(err)
	}
	e.finalizing("p3")
	if _, err := records.CommitAndVerify(ctx, e.st, e.blobs, "p3", 3, e.pol); err != nil {
		t.Fatal(err)
	}
	if len(e.blobFiles()) != 3 {
		t.Fatalf("blobs: %v", e.blobFiles())
	}
	later := e.now.Add(48 * time.Hour)
	dry, err := records.Prune(ctx, e.st, e.blobs, records.PruneSpec{Now: later, DryRun: true})
	if err != nil || len(dry) != 1 || dry[0].Name != "short.txt" || len(e.blobFiles()) != 3 {
		t.Fatalf("dry run: %+v %v blobs %d", dry, err, len(e.blobFiles()))
	}
	got, err := records.Prune(ctx, e.st, e.blobs, records.PruneSpec{Now: later})
	if err != nil || len(got) != 1 {
		t.Fatalf("prune: %+v %v", got, err)
	}
	if n := len(e.blobFiles()); n != 2 {
		t.Fatalf("the expired record's blob must go and the others stay: %d blobs left", n)
	}
	withRelease, err := records.Prune(ctx, e.st, e.blobs, records.PruneSpec{Now: later, Release: true})
	if err != nil || len(withRelease) != 1 || withRelease[0].Name != "rel.txt" {
		t.Fatalf("release prune: %+v %v", withRelease, err)
	}
	if n := len(e.blobFiles()); n != 1 {
		t.Fatalf("one blob left, got %d", n)
	}
	if err = records.AutoPrune(ctx, e.st, e.blobs, later); err != nil {
		t.Fatal(err)
	}
	if left, _ := e.st.ArtifactList(ctx, records.Filter{}); len(left) != 1 || left[0].Name != "keep.txt" {
		t.Fatalf("left: %+v", left)
	}
}
