package publish_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func mustRun(t *testing.T, f *fixture) publish.Release {
	t.Helper()
	rel, err := publish.Run(context.Background(), f.deps(), runID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return rel
}

func TestPublishWritesTheEpicLayout(t *testing.T) {
	f := newFixture(t)
	rel := mustRun(t, f)
	if rel.State != publish.Published || rel.Dir != publish.Dir(f.home, runID) || len(rel.ManifestSHA256) != 64 {
		t.Fatalf("release %+v", rel)
	}
	got := readDir(t, rel.Dir)
	want := []string{"SHA256SUMS", "artifacts/app", "artifacts/image.tar", "evidence.json", "provenance.intoto.dsse.json", "release-manifest.dsse.json",
		"sbom/app.cdx.json", "sbom/app.spdx.json", "sbom/image.tar.cdx.json", "sbom/image.tar.spdx.json"}
	var names []string
	for n := range got {
		names = append(names, n)
	}
	if len(names) != len(want) {
		t.Fatalf("files %v, want %v", names, want)
	}
	for _, n := range want {
		if _, ok := got[n]; !ok {
			t.Fatalf("missing %s in %v", n, names)
		}
	}
	if string(got["artifacts/app"]) != "app-bytes" || string(got["evidence.json"]) == "" {
		t.Fatal("artifact or evidence bytes are wrong")
	}
	row, ok, _ := f.ledger.ReleaseFor(context.Background(), runID)
	if !ok || row.State != store.ReleasePublished || row.ManifestSHA256 != rel.ManifestSHA256 || row.KeyID != rel.KeyID || !strings.HasPrefix(row.KeyID, "ci-release-") {
		t.Fatalf("ledger %+v", row)
	}
	if info, _ := os.Stat(filepath.Join(rel.Dir, "evidence.json")); info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence mode %v: it is private operator material", info.Mode())
	}
	if ents, _ := os.ReadDir(filepath.Join(f.home, "releases")); len(ents) != 1 {
		t.Fatalf("staging leftovers: %v", ents)
	}
}

func TestPublishSecondCallSignsNothingAndIsByteIdentical(t *testing.T) {
	f := newFixture(t)
	first := mustRun(t, f)
	before := readDir(t, first.Dir)
	signs := f.signer.signs
	second := mustRun(t, f)
	if f.signer.signs != signs || second.ManifestSHA256 != first.ManifestSHA256 {
		t.Fatalf("an intact published release was signed again (%d -> %d)", signs, f.signer.signs)
	}
	// the directory is lost: the recovery rebuilds byte-identical files
	if err := os.RemoveAll(first.Dir); err != nil {
		t.Fatal(err)
	}
	third := mustRun(t, f)
	if f.signer.signs == signs || third.ManifestSHA256 != first.ManifestSHA256 || !reflect.DeepEqual(readDir(t, third.Dir), before) {
		t.Fatal("the rebuilt release differs from the first one")
	}
	// a damaged file is rebuilt too
	if err := os.WriteFile(filepath.Join(third.Dir, "SHA256SUMS"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fourth := mustRun(t, f)
	if !reflect.DeepEqual(readDir(t, fourth.Dir), before) {
		t.Fatal("a damaged release was kept")
	}
}

func TestPublishRecoveryAfterAMissingKey(t *testing.T) {
	f := newFixture(t)
	key := f.res.vals[signing.KeyRef]
	delete(f.res.vals, signing.KeyRef)
	_, err := publish.Run(context.Background(), f.deps(), runID)
	var pe *publish.Error
	if !errors.As(err, &pe) || pe.Reason != signing.ReasonKeyMissing || pe.Code() != "E717" || pe.ExitCode() != 1 {
		t.Fatalf("error %v", err)
	}
	if !strings.Contains(err.Error(), "nself ci artifacts sign "+runID) {
		t.Fatalf("the recovery command is not named: %v", err)
	}
	row, ok, _ := f.ledger.ReleaseFor(context.Background(), runID)
	if !ok || row.State != store.ReleaseFailed || !strings.Contains(row.Reason, "key_missing") {
		t.Fatalf("ledger %+v", row)
	}
	if _, serr := os.Stat(publish.Dir(f.home, runID)); serr == nil {
		t.Fatal("a failed publication left a release directory")
	}
	f.res.vals[signing.KeyRef] = key
	rel := mustRun(t, f)
	if row, _, _ = f.ledger.ReleaseFor(context.Background(), runID); row.State != store.ReleasePublished || row.ManifestSHA256 != rel.ManifestSHA256 {
		t.Fatalf("recovery left %+v", row)
	}
}

func TestPublishNothingWithoutAPassingReleaseJob(t *testing.T) {
	cases := map[string]func(f *fixture){
		"no release job": func(f *fixture) {
			f.src.jobs = f.src.jobs[:1]
			f.setChecks(func(c []model.Check) []model.Check { return c[:1] })
		},
		"release check failed": func(f *fixture) {
			f.setChecks(func(c []model.Check) []model.Check { c[1].Result = "fail"; return c })
		},
		"run failed": func(f *fixture) { f.setResult("fail") },
	}
	for name, mutate := range cases {
		f := newFixture(t)
		mutate(f)
		_, err := publish.Run(context.Background(), f.deps(), runID)
		if !errors.Is(err, publish.ErrNotRelease) {
			t.Fatalf("%s: %v", name, err)
		}
		if _, ok, _ := f.ledger.ReleaseFor(context.Background(), runID); ok || f.signer.signs+f.signer.pubs+f.res.calls != 0 {
			t.Fatalf("%s: a record or a signer call was made", name)
		}
		if _, err := os.Stat(filepath.Join(f.home, "releases")); err == nil {
			t.Fatalf("%s: a releases directory exists", name)
		}
	}
}

func TestPublishTeamCoordinatorAwaitsAndResolvesNothing(t *testing.T) {
	f := newFixture(t)
	d := f.deps()
	d.Team = true
	rel, err := publish.Run(context.Background(), d, runID)
	if err != nil || rel.State != publish.Awaiting || !strings.Contains(rel.Message, "--coordinator") {
		t.Fatalf("release %+v err %v", rel, err)
	}
	if f.res.calls != 0 || f.signer.signs != 0 || f.signer.pubs != 0 {
		t.Fatalf("a TEAM coordinator touched the signer: resolves %d signs %d pubs %d", f.res.calls, f.signer.signs, f.signer.pubs)
	}
	if row, ok, _ := f.ledger.ReleaseFor(context.Background(), runID); !ok || row.State != store.ReleaseAwaiting {
		t.Fatalf("ledger %+v", row)
	}
	// the operator's machine finishes it
	if rel = mustRun(t, f); rel.State != publish.Published {
		t.Fatalf("after the operator signed: %+v", rel)
	}
}

func TestPublishRefusesBadInputs(t *testing.T) {
	for name, mutate := range map[string]func(f *fixture){
		"artifact bytes changed": func(f *fixture) { f.blobs.data[sum(f.files["app"])] = []byte("tampered") },
		"sbom missing": func(f *fixture) {
			for i, r := range f.src.recs {
				if r.Name == "app.spdx.json" {
					f.src.recs = append(f.src.recs[:i], f.src.recs[i+1:]...)
					break
				}
			}
		},
		"no artifact": func(f *fixture) { f.src.recs = nil },
		"attempt still running": func(f *fixture) {
			f.src.attempts = append(f.src.attempts, store.AttemptView{ID: "x.a1", JobID: "x", N: 1, State: "running"})
		},
	} {
		f := newFixture(t)
		mutate(f)
		_, err := publish.Run(context.Background(), f.deps(), runID)
		var pe *publish.Error
		if !errors.As(err, &pe) {
			t.Fatalf("%s: %v", name, err)
		}
		if row, ok, _ := f.ledger.ReleaseFor(context.Background(), runID); !ok || row.State != store.ReleaseFailed {
			t.Fatalf("%s: ledger %+v", name, row)
		}
		if _, serr := os.Stat(publish.Dir(f.home, runID)); serr == nil {
			t.Fatalf("%s: a release directory exists", name)
		}
	}
}

func TestExportWritesAVerifiedCopy(t *testing.T) {
	f := newFixture(t)
	rel := mustRun(t, f)
	out := filepath.Join(t.TempDir(), "out")
	exp, err := publish.Export(context.Background(), f.deps(), runID, out, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if exp.Dir != out || !strings.Contains(exp.Message, "private operator material") || !reflect.DeepEqual(readDir(t, out), readDir(t, rel.Dir)) {
		t.Fatalf("export %+v", exp)
	}
	if _, err = publish.Export(context.Background(), f.deps(), runID, out, t.TempDir()); err == nil {
		t.Fatal("an export into a non-empty directory was accepted")
	}
	// a release that is not published names its recovery
	g := newFixture(t)
	if _, err = publish.Export(context.Background(), g.deps(), runID, filepath.Join(t.TempDir(), "o"), t.TempDir()); err == nil || !strings.Contains(err.Error(), "artifacts sign") {
		t.Fatalf("unpublished export: %v", err)
	}
	// a tampered published directory is refused
	if err = os.WriteFile(filepath.Join(rel.Dir, "artifacts", "app"), []byte("evil"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = publish.Export(context.Background(), f.deps(), runID, filepath.Join(t.TempDir(), "o2"), t.TempDir()); err == nil {
		t.Fatal("a tampered release was exported")
	}
}

func TestSignSourcesRegistry(t *testing.T) {
	publish.RegisterSignSource("zz-test", nil, func(context.Context, string, *flagSet) (publish.Release, error) { return publish.Release{}, nil })
	var found bool
	for _, s := range publish.SignSources() {
		found = found || s.Name == "zz-test"
	}
	if !found {
		t.Fatal("the source was not registered")
	}
	for _, bad := range []string{"zz-test", "Bad Name", ""} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("%q was accepted", bad)
				}
			}()
			publish.RegisterSignSource(bad, nil, func(context.Context, string, *flagSet) (publish.Release, error) { return publish.Release{}, nil })
		}()
	}
}
