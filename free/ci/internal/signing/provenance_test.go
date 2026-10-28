package signing_test

// Purpose: tests for the adversarial review round of P7-CACHE-07: image
// digests, records of failed or foreign attempts, the provenance predicate,
// evidence that did not pass, and the TEAM coordinator.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	indexDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	amdDigest   = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	armDigest   = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	imageRef    = "ghcr.io/example/img:1.0.0"
)

// withImage adds a committed image record ("img", an OCI layout) and its SBOMs to the release attempt.
func withImage(f *fixture) {
	for _, n := range []string{"img", "img.cdx.json", "img.spdx.json"} {
		f.files[n] = []byte("bytes-of-" + n)
		f.blobs.data[sum(f.files[n])] = f.files[n]
	}
	add := func(name string, kind records.Kind, img *records.ImageInfo, platform string) {
		f.src.recs = append(f.src.recs, records.Record{ID: "r-" + name, Project: project, Run: runID, Attempt: "att-rel-2", Revision: revision, Name: name, Kind: kind,
			SHA256: sum(f.files[name]), Size: int64(len(f.files[name])), State: records.StateCommitted, Image: img, Platform: platform})
	}
	add("img", records.KindImage, &records.ImageInfo{Ref: imageRef, IndexDigest: indexDigest, Manifests: []records.ImageManifest{{Platform: "linux/amd64", Digest: amdDigest}, {Platform: "linux/arm64", Digest: armDigest}}}, "linux/amd64")
	add("img.cdx.json", records.KindSBOM, nil, "")
	add("img.spdx.json", records.KindSBOM, nil, "")
}

func imageArtifact(f *fixture, tweak func(*manifest.Artifact)) func(*manifest.Manifest) {
	return func(m *manifest.Manifest) {
		a := manifest.Artifact{Name: "img", Path: "artifacts/img", Kind: manifest.KindImage, SHA256: sum(f.files["img"]), Size: int64(len(f.files["img"])),
			Image: &manifest.Image{Ref: imageRef, IndexDigest: indexDigest, Manifests: []manifest.ImageManifest{{Platform: "linux/amd64", Digest: amdDigest}, {Platform: "linux/arm64", Digest: armDigest}}},
			SBOM:  manifest.SBOM{CycloneDX: manifest.FileRef{Path: "sbom/img.cdx.json", SHA256: sum(f.files["img.cdx.json"])}, SPDX: manifest.FileRef{Path: "sbom/img.spdx.json", SHA256: sum(f.files["img.spdx.json"])}}}
		if tweak != nil {
			tweak(&a)
		}
		m.Artifacts = append(m.Artifacts, a)
	}
}

func handle(t *testing.T, f *fixture, items ...signing.Item) error {
	t.Helper()
	req := request(items...)
	_, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
	return err
}

func TestManifestImageDetailsMustBeTheRecords(t *testing.T) {
	f := newFixture(t)
	withImage(f)
	if err := handle(t, f, item(manifest.PayloadType, f.manifestPayload(imageArtifact(f, nil)))); err != nil {
		t.Fatalf("a manifest that matches its image record: %v", err)
	}
	evil := "sha256:" + strings.Repeat("e", 64)
	amd := "linux/amd64"
	arm := "linux/arm64"
	bad := map[string]func(*manifest.Artifact){
		"index digest":         func(a *manifest.Artifact) { a.Image.IndexDigest = evil },
		"platform digest":      func(a *manifest.Artifact) { a.Image.Manifests[0].Digest = evil },
		"missing platform":     func(a *manifest.Artifact) { a.Image.Manifests = a.Image.Manifests[:1] },
		"another platform":     func(a *manifest.Artifact) { a.Image.Manifests[1].Platform = "linux/s390x" },
		"reference":            func(a *manifest.Artifact) { a.Image.Ref = "ghcr.io/example/other:1" },
		"platform of artifact": func(a *manifest.Artifact) { a.Platform = &arm },
	}
	for name, tweak := range bad {
		err := handle(t, f, item(manifest.PayloadType, f.manifestPayload(imageArtifact(f, tweak))))
		if !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the platform the record has is accepted
	if err := handle(t, f, item(manifest.PayloadType, f.manifestPayload(imageArtifact(f, func(a *manifest.Artifact) { a.Platform = &amd })))); err != nil {
		t.Fatalf("matching platform: %v", err)
	}
	// a record that is not an image cannot be passed off as one, and the reverse
	g := newFixture(t)
	withImage(g)
	g.mutateRecord("img", func(r *records.Record) { r.Image = nil; r.Kind = records.KindBinary })
	if err := handle(t, g, item(manifest.PayloadType, g.manifestPayload(imageArtifact(g, nil)))); !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
		t.Fatalf("a binary record presented as an image: %v", err)
	}
	h := newFixture(t)
	withImage(h)
	err := handle(t, h, item(manifest.PayloadType, h.manifestPayload(imageArtifact(h, func(a *manifest.Artifact) { a.Kind, a.Image = manifest.KindArchive, nil }))))
	if !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
		t.Fatalf("an image record presented as an archive: %v", err)
	}
	// kind mismatch on an ordinary artifact
	k := newFixture(t)
	if err := handle(t, k, item(manifest.PayloadType, k.manifestPayload(func(m *manifest.Manifest) { m.Artifacts[0].Kind = manifest.KindPackage }))); !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
		t.Fatalf("a binary presented as a package: %v", err)
	}
}

func TestRecordsOfOtherAttemptsAreNotSigned(t *testing.T) {
	f := newFixture(t)
	evilBytes := []byte("evil-app-bytes")
	f.blobs.data[sum(evilBytes)] = evilBytes
	// a failed release attempt committed an "app" before it ended
	f.src.recs = append(f.src.recs, records.Record{ID: "r-evil", Project: project, Run: runID, Attempt: "att-rel-1", Revision: revision, Name: "app", Kind: records.KindBinary,
		SHA256: sum(evilBytes), Size: int64(len(evilBytes)), State: records.StateCommitted})
	// a passed attempt of a non-release job committed a coverage report
	cov := []byte("coverage-html")
	f.blobs.data[sum(cov)] = cov
	f.src.recs = append(f.src.recs, records.Record{ID: "r-cov", Project: project, Run: runID, Attempt: "att-test", Revision: revision, Name: "cov", Kind: records.KindReport,
		SHA256: sum(cov), Size: int64(len(cov)), State: records.StateCommitted})
	// a second passed release attempt committed its own "app"
	other := []byte("app-of-attempt-3")
	f.blobs.data[sum(other)] = other
	f.src.attempts = append(f.src.attempts, store.AttemptView{ID: "att-rel-3", JobID: "job-rel", N: 3, State: "passed"})
	f.src.recs = append(f.src.recs, records.Record{ID: "r-3", Project: project, Run: runID, Attempt: "att-rel-3", Revision: revision, Name: "app", Kind: records.KindBinary,
		SHA256: sum(other), Size: int64(len(other)), State: records.StateCommitted})

	manifestWith := func(name string, kind string, b []byte) []byte {
		return f.manifestPayload(func(m *manifest.Manifest) {
			m.Artifacts[0].Name, m.Artifacts[0].Path, m.Artifacts[0].Kind = name, "artifacts/"+name, kind
			m.Artifacts[0].SHA256, m.Artifacts[0].Size = sum(b), int64(len(b))
			m.Artifacts[0].SBOM.CycloneDX.Path, m.Artifacts[0].SBOM.SPDX.Path = "sbom/"+name+".cdx.json", "sbom/"+name+".spdx.json"
		})
	}
	for name, p := range map[string][]byte{
		"a failed attempt's record":        manifestWith("app", manifest.KindBinary, evilBytes),
		"a test job's record":              manifestWith("cov", manifest.KindReport, cov),
		"another release attempt's record": manifestWith("app", manifest.KindBinary, other),
	} {
		if err := handle(t, f, item(manifest.PayloadType, p)); !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
			t.Errorf("manifest with %s: %v", name, err)
		}
	}
	subject := func(name string, b []byte, attempt string) []byte {
		return f.statementPayload(func(m map[string]any) {
			m["subject"] = []map[string]any{{"name": name, "digest": map[string]string{"sha256": sum(b)}}}
			m["predicate"] = f.predicate(func(p map[string]any) {
				p["runDetails"] = f.runDetails(runID + "/" + attempt)
			})
		})
	}
	for name, p := range map[string][]byte{
		"a failed attempt's record":      subject("app", evilBytes, "att-rel-2"),
		"a test job's record":            subject("cov", cov, "att-rel-2"),
		"a record of another attempt":    subject("app", other, "att-rel-2"),
		"a record not of the invocation": subject("app", f.files["app"], "att-rel-3"),
	} {
		if err := handle(t, f, item(signing.PayloadStatement, p)); !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
			t.Errorf("statement with %s: %v", name, err)
		}
	}
	// each release attempt's own records sign, but not together under one request
	if err := handle(t, f, item(signing.PayloadStatement, subject("app", other, "att-rel-3"))); err != nil {
		t.Fatalf("a statement of attempt 3: %v", err)
	}
	mixed := []signing.Item{item(manifest.PayloadType, f.manifestPayload(nil)), item(signing.PayloadStatement, subject("app", other, "att-rel-3"))}
	if err := handle(t, f, mixed...); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("a manifest and a statement of different attempts: %v", err)
	}
}

func TestStatementPredicateMustMatchTheRun(t *testing.T) {
	f := newFixture(t)
	stmt := func(tweak func(map[string]any)) []byte {
		return f.statementPayload(func(m map[string]any) { m["predicate"] = f.predicate(tweak) })
	}
	if err := handle(t, f, item(signing.PayloadStatement, stmt(nil))); err != nil {
		t.Fatal(err)
	}
	f.res.calls = 0
	bd := func(p map[string]any) map[string]any { return p["buildDefinition"].(map[string]any) }
	cases := map[string]func(map[string]any){
		"another build type": func(p map[string]any) { bd(p)["buildType"] = "https://nself.org/ci/buildtypes/agent/v1" },
		"another repository": func(p map[string]any) { bd(p)["externalParameters"].(map[string]any)["repository"] = "evil/repo" },
		"another revision": func(p map[string]any) {
			bd(p)["externalParameters"].(map[string]any)["revision"] = strings.Repeat("f", 40)
		},
		"no pinned commit": func(p map[string]any) { bd(p)["resolvedDependencies"] = []map[string]any{} },
		"a pin for another commit": func(p map[string]any) {
			bd(p)["resolvedDependencies"] = []map[string]any{{"uri": "git+repo-1@" + revision, "digest": map[string]string{"gitCommit": strings.Repeat("f", 40)}}}
		},
		"a pin for another repo": func(p map[string]any) {
			bd(p)["resolvedDependencies"] = []map[string]any{{"uri": "git+evil@" + revision, "digest": map[string]string{"gitCommit": revision}}}
		},
		"another run": func(p map[string]any) {
			p["runDetails"] = map[string]any{"metadata": map[string]any{"invocationId": "run-9/att-rel-2"}}
		},
		"a failed attempt": func(p map[string]any) {
			p["runDetails"] = map[string]any{"metadata": map[string]any{"invocationId": runID + "/att-rel-1"}}
		},
		"a check job's attempt": func(p map[string]any) {
			p["runDetails"] = map[string]any{"metadata": map[string]any{"invocationId": runID + "/att-test"}}
		},
		"an unknown attempt": func(p map[string]any) {
			p["runDetails"] = map[string]any{"metadata": map[string]any{"invocationId": runID + "/att-zz"}}
		},
		"no run details":      func(p map[string]any) { delete(p, "runDetails") },
		"no build definition": func(p map[string]any) { delete(p, "buildDefinition") },
	}
	for name, tweak := range cases {
		if err := handle(t, f, item(signing.PayloadStatement, stmt(tweak))); !signing.IsReason(err, signing.ReasonPayloadRejected) || f.res.calls != 0 {
			t.Errorf("%s: %v (%d resolves)", name, err, f.res.calls)
		}
	}
	if err := handle(t, f, item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) { m["predicate"] = "text" }))); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("a predicate that is not an object: %v", err)
	}
}

func TestEvidenceThatDidNotPassBlocksEveryPayload(t *testing.T) {
	for _, body := range []string{
		`{"revision":"` + revision + `","repo_id":"repo-1","run_id":"run-1","result":"fail"}`,
		`{"revision":"` + revision + `","repo_id":"repo-1","run_id":"run-1","result":"cancelled"}`,
		`{"revision":"` + revision + `","repo_id":"repo-1","run_id":"run-1"}`,
	} {
		f := newFixture(t)
		f.setEvidence(body)
		if err := handle(t, f, item(signing.PayloadStatement, f.statementPayload(nil))); !signing.IsReason(err, signing.ReasonPayloadRejected) || f.res.calls != 0 {
			t.Errorf("a statement over evidence %q: %v", body[len(body)-20:], err)
		}
	}
}

func TestTeamCoordinatorNeverSigns(t *testing.T) {
	f := newFixture(t)
	s := &fakeSigner{ans: answer{out: `{"key_id":"ci-release-0123456789abcdef","public_key":"AAAA"}`}}
	c := clientFor(f, s)
	c.Team = true
	items := []signing.Item{item(signing.PayloadStatement, []byte("x"))}
	if _, err := c.Sign(context.Background(), runID, items); !signing.IsReason(err, signing.ReasonSignerFailed) || !strings.Contains(err.Error(), "TEAM") {
		t.Fatalf("sign: %v", err)
	}
	for name, call := range map[string]func() error{
		"public": func() error { _, err := c.PublicKey(context.Background()); return err },
		"init":   func() error { _, err := c.Init(context.Background()); return err },
		"rotate": func() error { _, err := c.Rotate(context.Background()); return err },
	} {
		if err := call(); !signing.IsReason(err, signing.ReasonSignerFailed) || !strings.Contains(err.Error(), "TEAM") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(s.calls) != 0 {
		t.Fatalf("a signer was started on a TEAM coordinator: %v", s.calls)
	}
}

func TestClientMapsUnknownReasonsToSignerFailed(t *testing.T) {
	f := newFixture(t)
	for _, reason := range []string{"made_up", "", "KEY_MISSING", "digest_not_committed "} {
		out := `{"error":{"code":"E717","reason":"` + reason + `","message":"m"}}`
		_, err := clientFor(f, &fakeSigner{ans: answer{out: out, code: 2}}).PublicKey(context.Background())
		if !signing.IsReason(err, signing.ReasonSignerFailed) {
			t.Errorf("reason %q: %v", reason, err)
		}
	}
	for _, reason := range []string{"key_missing", "key_invalid", "key_id_mismatch", "payload_rejected", "digest_not_committed", "signer_failed", "cli_too_old", "secret_exists"} {
		out := `{"error":{"code":"E717","reason":"` + reason + `","message":"m"}}`
		_, err := clientFor(f, &fakeSigner{ans: answer{out: out, code: 2}}).PublicKey(context.Background())
		if !signing.IsReason(err, reason) {
			t.Errorf("reason %q: %v", reason, err)
		}
	}
}

func TestProjectRootsCompareThroughSymlinks(t *testing.T) {
	f := newFixture(t)
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	f.src.pipe.Project = real
	for i := range f.src.recs {
		f.src.recs[i].Project = real
	}
	req := request(item(signing.PayloadStatement, f.statementPayload(nil)))
	h := f.handler()
	h.Project = link
	if _, err := h.Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatalf("a symlinked spelling of the project root: %v", err)
	}
	h.Project = filepath.Join(real, "sub", "..") // a relative part
	if _, err := h.Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatalf("a root with a .. part: %v", err)
	}
	t.Chdir(real)
	h.Project = "." // a relative spelling resolves against the working directory
	if _, err := h.Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatalf("a relative project root: %v", err)
	}
	h.Project = t.TempDir()
	if _, err := h.Handle(context.Background(), req, decodeAll(t, req)); err == nil {
		t.Fatal("another directory was accepted")
	}
}

func TestSBOMDigestsMustBeThatArtifactsSBOMs(t *testing.T) {
	f := newFixture(t)
	swap := func(m *manifest.Manifest) {
		m.Artifacts[0].SBOM, m.Artifacts[1].SBOM = m.Artifacts[1].SBOM, m.Artifacts[0].SBOM
		for i := range m.Artifacts { // the paths follow the artifact names again; only the digests are swapped
			n := m.Artifacts[i].Name
			m.Artifacts[i].SBOM.CycloneDX.Path, m.Artifacts[i].SBOM.SPDX.Path = "sbom/"+n+".cdx.json", "sbom/"+n+".spdx.json"
		}
	}
	if err := handle(t, f, item(manifest.PayloadType, f.manifestPayload(swap))); !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
		t.Fatalf("artifact A carrying artifact B's SBOM: %v", err)
	}
	for _, name := range []string{"myapp.cdx.json", "app.cdx.json.bak", "xapp.cdx.json", "sbom/myapp.cdx.json", ".nself-ci/sbom/nested/app.cdx.json", "sbom/nested/app.cdx.json", "nested/app.cdx.json", "other/sbom/app.cdx.json", "/app.cdx.json"} {
		g := newFixture(t)
		g.mutateRecord("app.cdx.json", func(r *records.Record) { r.Name = name })
		if err := handle(t, g, item(manifest.PayloadType, g.manifestPayload(nil))); !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
			t.Errorf("sbom record named %q: %v", name, err)
		}
	}
	for _, name := range []string{"app.cdx.json", "sbom/app.cdx.json", ".nself-ci/sbom/app.cdx.json"} {
		g := newFixture(t)
		g.mutateRecord("app.cdx.json", func(r *records.Record) { r.Name = name })
		if err := handle(t, g, item(manifest.PayloadType, g.manifestPayload(nil))); err != nil {
			t.Errorf("sbom record named %q: %v", name, err)
		}
	}
}

func TestStatementSubjectsAreReleaseArtifacts(t *testing.T) {
	for kind, ok := range map[records.Kind]bool{records.KindLog: false, records.KindCoverage: false, records.KindBench: false, records.KindSBOM: false,
		records.KindReport: true, records.KindOther: true, records.KindPackage: true, records.KindBinary: true, records.KindArchive: true, records.KindImage: true} {
		f := newFixture(t)
		f.mutateRecord("app", func(r *records.Record) { r.Kind = kind })
		err := handle(t, f, item(signing.PayloadStatement, f.statementPayload(nil)))
		if ok && err != nil || !ok && !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
			t.Errorf("subject record of kind %s: %v", kind, err)
		}
	}
}

func TestStatementPredicateBuilderJobAndByproduct(t *testing.T) {
	f := newFixture(t)
	stmt := func(tweak func(map[string]any)) []byte {
		return f.statementPayload(func(m map[string]any) { m["predicate"] = f.predicate(tweak) })
	}
	rd := func(p map[string]any) map[string]any { return p["runDetails"].(map[string]any) }
	bd := func(p map[string]any) map[string]any { return p["buildDefinition"].(map[string]any) }
	for name, tweak := range map[string]func(map[string]any){
		"another builder": func(p map[string]any) { rd(p)["builder"] = map[string]any{"id": "https://evil.example/builder#x"} },
		"wrong coordinator": func(p map[string]any) {
			rd(p)["builder"] = map[string]any{"id": "https://nself.org/ci/builder/v1#other/runner-1"}
		},
		"no builder":    func(p map[string]any) { delete(rd(p), "builder") },
		"another job":   func(p map[string]any) { bd(p)["externalParameters"].(map[string]any)["job"] = "test" },
		"no job":        func(p map[string]any) { delete(bd(p)["externalParameters"].(map[string]any), "job") },
		"no byproducts": func(p map[string]any) { delete(rd(p), "byproducts") },
		"another evidence digest": func(p map[string]any) {
			rd(p)["byproducts"] = []map[string]any{{"name": "evidence.json", "digest": map[string]string{"sha256": strings.Repeat("a", 64)}}}
		},
		"two evidence byproducts, the first right": func(p map[string]any) {
			rd(p)["byproducts"] = []map[string]any{{"name": "evidence.json", "digest": map[string]string{"sha256": sum(f.evRaw)}}, {"name": "evidence.json", "digest": map[string]string{"sha256": strings.Repeat("0", 64)}}}
		},
		"two evidence byproducts, the last right": func(p map[string]any) {
			rd(p)["byproducts"] = []map[string]any{{"name": "evidence.json", "digest": map[string]string{"sha256": strings.Repeat("0", 64)}}, {"name": "evidence.json", "digest": map[string]string{"sha256": sum(f.evRaw)}}}
		},
		"the same evidence byproduct twice": func(p map[string]any) {
			rd(p)["byproducts"] = []map[string]any{{"name": "evidence.json", "digest": map[string]string{"sha256": sum(f.evRaw)}}, {"name": "evidence.json", "digest": map[string]string{"sha256": sum(f.evRaw)}}}
		},
		"a second pin of the repository at another commit": func(p map[string]any) {
			bd(p)["resolvedDependencies"] = append(bd(p)["resolvedDependencies"].([]map[string]any), map[string]any{"uri": "git+repo-1@" + strings.Repeat("f", 40), "digest": map[string]string{"gitCommit": strings.Repeat("f", 40)}})
		},
		"the right pin twice": func(p map[string]any) {
			d := bd(p)["resolvedDependencies"].([]map[string]any)
			bd(p)["resolvedDependencies"] = append(d, d[0])
		},
		"digest under another name": func(p map[string]any) {
			rd(p)["byproducts"] = []map[string]any{{"name": "other.json", "digest": map[string]string{"sha256": sum(f.evRaw)}}}
		},
	} {
		if err := handle(t, f, item(signing.PayloadStatement, stmt(tweak))); !signing.IsReason(err, signing.ReasonPayloadRejected) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestStatementOtherDependenciesAreCarriedThrough(t *testing.T) {
	f := newFixture(t)
	other := map[string]any{"name": "go.sum", "digest": map[string]string{"sha256": strings.Repeat("a", 64)}}
	img := map[string]any{"uri": "oci://ghcr.io/example/toolchain:1", "digest": map[string]string{"sha256": strings.Repeat("b", 64)}}
	req := item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) {
		m["predicate"] = f.predicate(func(p map[string]any) {
			d := p["buildDefinition"].(map[string]any)
			d["resolvedDependencies"] = append(d["resolvedDependencies"].([]map[string]any), other, img)
		})
	}))
	if err := handle(t, f, req); err != nil {
		t.Fatalf("lockfile and toolchain dependencies: %v", err)
	}
}

func TestManifestPipelineAndFinishTimeAreTheEvidences(t *testing.T) {
	f := newFixture(t)
	for name, tweak := range map[string]func(*manifest.Manifest){
		"another pipeline":    func(m *manifest.Manifest) { m.Release.Pipeline = "nightly" },
		"another finish time": func(m *manifest.Manifest) { m.Release.FinishedAt = "2026-10-19T12:00:01Z" },
	} {
		if err := handle(t, f, item(manifest.PayloadType, f.manifestPayload(tweak))); !signing.IsReason(err, signing.ReasonPayloadRejected) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the same instant in another zone is the same finish time
	if err := handle(t, f, item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Release.FinishedAt = "2026-10-19T08:00:00-04:00" }))); err != nil {
		t.Fatalf("the same instant in another zone: %v", err)
	}
	// evidence without a finish time cannot back a manifest
	g := newFixture(t)
	g.setEvidence(`{"revision":"` + revision + `","repo_id":"repo-1","run_id":"run-1","result":"pass","pipeline":{"name":"release"}}`)
	if err := handle(t, g, item(manifest.PayloadType, g.manifestPayload(nil))); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("evidence with no finish time: %v", err)
	}
	h := newFixture(t)
	h.setEvidence(`{"revision":"` + revision + `","repo_id":"repo-1","run_id":"run-1","result":"pass","pipeline":{"name":"release"},"timings":{"finished_at":"yesterday"}}`)
	if err := handle(t, h, item(manifest.PayloadType, h.manifestPayload(nil))); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("evidence with an unparseable finish time: %v", err)
	}
}

func TestClientReadsTheCoordinatorModeFromItsEnvironment(t *testing.T) {
	f := newFixture(t)
	s := &fakeSigner{}
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "team")
	if _, err := clientFor(f, s).Sign(context.Background(), runID, []signing.Item{item(signing.PayloadStatement, []byte("x"))}); !signing.IsReason(err, signing.ReasonSignerFailed) || !strings.Contains(err.Error(), "TEAM") {
		t.Fatalf("%v", err)
	}
	if _, err := clientFor(f, s).PublicKey(context.Background()); !signing.IsReason(err, signing.ReasonSignerFailed) || len(s.calls) != 0 {
		t.Fatalf("%v (%d calls)", err, len(s.calls))
	}
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "personal")
	s.ans = answer{out: `{"key_id":"k","public_key":"p"}`}
	if _, err := clientFor(f, s).PublicKey(context.Background()); err != nil {
		t.Fatalf("a personal coordinator: %v", err)
	}
}

func TestStatementPredicateFieldsHaveTheirTypes(t *testing.T) {
	f := newFixture(t)
	rd := func(p map[string]any) map[string]any { return p["runDetails"].(map[string]any) }
	bd := func(p map[string]any) map[string]any { return p["buildDefinition"].(map[string]any) }
	for name, tweak := range map[string]func(map[string]any){
		"builder is a string":        func(p map[string]any) { rd(p)["builder"] = "not-an-object" },
		"builder id is a number":     func(p map[string]any) { rd(p)["builder"] = map[string]any{"id": 7} },
		"byproducts is an object":    func(p map[string]any) { rd(p)["byproducts"] = map[string]any{} },
		"metadata is a list":         func(p map[string]any) { rd(p)["metadata"] = []any{} },
		"runDetails is a string":     func(p map[string]any) { p["runDetails"] = "x" },
		"buildDefinition is a list":  func(p map[string]any) { p["buildDefinition"] = []any{} },
		"dependencies is an object":  func(p map[string]any) { bd(p)["resolvedDependencies"] = map[string]any{} },
		"externalParameters is text": func(p map[string]any) { bd(p)["externalParameters"] = "x" },
		"revision is a number":       func(p map[string]any) { bd(p)["externalParameters"].(map[string]any)["revision"] = 7 },
	} {
		if err := handle(t, f, item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) { m["predicate"] = f.predicate(tweak) }))); !signing.IsReason(err, signing.ReasonPayloadRejected) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestEvidenceNameIsReservedAndSubjectNamesAreUnique(t *testing.T) {
	f := newFixture(t)
	// a job committed an artifact that is called evidence.json
	bytes := []byte("not-the-evidence")
	f.blobs.data[sum(bytes)] = bytes
	f.src.recs = append(f.src.recs, records.Record{ID: "r-ev", Project: project, Run: runID, Attempt: "att-rel-2", Revision: revision, Name: "evidence.json", Kind: records.KindOther,
		SHA256: sum(bytes), Size: int64(len(bytes)), State: records.StateCommitted})
	subjects := func(list ...map[string]any) []byte {
		return f.statementPayload(func(m map[string]any) { m["subject"] = list })
	}
	subj := func(name string, digest string) map[string]any {
		return map[string]any{"name": name, "digest": map[string]string{"sha256": digest}}
	}
	ev := sum(f.evRaw)
	if err := handle(t, f, item(signing.PayloadStatement, subjects(subj("evidence.json", ev), subj("app", sum(f.files["app"]))))); err != nil {
		t.Fatalf("the stored evidence under its own name: %v", err)
	}
	for name, p := range map[string][]byte{
		"a committed record called evidence.json":               subjects(subj("evidence.json", sum(bytes))),
		"the evidence subject and the record under one name":    subjects(subj("evidence.json", ev), subj("evidence.json", sum(bytes))),
		"the same subject twice":                                subjects(subj("app", sum(f.files["app"])), subj("app", sum(f.files["app"]))),
		"one name with two digests":                             subjects(subj("app", sum(f.files["app"])), subj("app", sum(f.files["image"]))),
		"the record called evidence.json under artifacts/ only": subjects(subj("artifacts/evidence.json", sum(bytes))),
	} {
		if err := handle(t, f, item(signing.PayloadStatement, p)); err == nil {
			t.Errorf("%s was signed", name)
		}
	}
}
