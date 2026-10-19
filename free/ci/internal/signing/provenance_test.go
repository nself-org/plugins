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
	for _, n := range []string{"img", "img.cdx", "img.spdx"} {
		f.files[n] = []byte("bytes-of-" + n)
		f.blobs.data[sum(f.files[n])] = f.files[n]
	}
	add := func(name string, kind records.Kind, img *records.ImageInfo, platform string) {
		f.src.recs = append(f.src.recs, records.Record{ID: "r-" + name, Project: project, Run: runID, Attempt: "att-rel-2", Revision: revision, Name: name, Kind: kind,
			SHA256: sum(f.files[name]), Size: int64(len(f.files[name])), State: records.StateCommitted, Image: img, Platform: platform})
	}
	add("img", records.KindImage, &records.ImageInfo{Ref: imageRef, IndexDigest: indexDigest, Manifests: []records.ImageManifest{{Platform: "linux/amd64", Digest: amdDigest}, {Platform: "linux/arm64", Digest: armDigest}}}, "linux/amd64")
	add("img.cdx", records.KindSBOM, nil, "")
	add("img.spdx", records.KindSBOM, nil, "")
}

func imageArtifact(f *fixture, tweak func(*manifest.Artifact)) func(*manifest.Manifest) {
	return func(m *manifest.Manifest) {
		a := manifest.Artifact{Name: "img", Path: "artifacts/img", Kind: manifest.KindImage, SHA256: sum(f.files["img"]), Size: int64(len(f.files["img"])),
			Image: &manifest.Image{Ref: imageRef, IndexDigest: indexDigest, Manifests: []manifest.ImageManifest{{Platform: "linux/amd64", Digest: amdDigest}, {Platform: "linux/arm64", Digest: armDigest}}},
			SBOM:  manifest.SBOM{CycloneDX: manifest.FileRef{Path: "sbom/img.cdx.json", SHA256: sum(f.files["img.cdx"])}, SPDX: manifest.FileRef{Path: "sbom/img.spdx.json", SHA256: sum(f.files["img.spdx"])}}}
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
				p["runDetails"] = map[string]any{"metadata": map[string]any{"invocationId": runID + "/" + attempt}}
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
	h.Project = t.TempDir()
	if _, err := h.Handle(context.Background(), req, decodeAll(t, req)); err == nil {
		t.Fatal("another directory was accepted")
	}
}
