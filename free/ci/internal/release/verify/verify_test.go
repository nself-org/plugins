package verify_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
)

func TestGoodReleaseVerifies(t *testing.T) {
	for name, mode := range map[string]trustMode{"operator keys": operatorKeys, "pinned tip": pinnedKeys} {
		r := newRig(t, mode)
		res := r.verify()
		if !res.Verified || res.Exit != 0 || res.Reason != "" || res.Code != "" || res.SLSABuildLevel != "L1" {
			t.Fatalf("%s: %+v", name, res)
		}
		want := []string{"signature", "manifest", "checksum", "evidence_binding", "provenance", "sbom_link", "evidence_verdict"}
		if strings.Join(res.Passed, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: passed %v", name, res.Passed)
		}
		if res.Revision != r.rev || res.KeyID != r.keyID() || res.RunID != "run-1" || res.Artifacts != 2 || res.RepoID == "" {
			t.Fatalf("%s: summary %+v", name, res)
		}
	}
}

func TestKeyOnlyInTheReleasedRevisionIsNotTrusted(t *testing.T) {
	r := newRig(t, releasedOnly)
	res := r.verify()
	wantFail(t, res, verify.ReasonSignature, 11)
	if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "differs from the protected default branch") {
		t.Fatalf("no working-tree warning: %v", res.Warnings)
	}
}

func TestWorkingTreeCopyOfTheKeysIsIgnoredAndWarned(t *testing.T) {
	r := newRig(t, pinnedKeys)
	r.ownRepo()
	writeFile(t, r.repo, ".nself/trust/ci-release.keys", keysLine(signing.KeyID(signing.PurposeCIRelease, r.otherPub), r.otherPub))
	res := r.verify()
	if !res.Verified || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], ".nself/trust/ci-release.keys") {
		t.Fatalf("%+v", res)
	}
	r.manSigner = r.other
	r.publish()
	wantFail(t, r.verify(), verify.ReasonSignature, 11) // the working-tree key is not a trust root
}

func TestLaterLockfileChangeStillVerifiesAndAbsentRevisionFails(t *testing.T) {
	r := newRig(t, operatorKeys)
	r.ownRepo()
	commit(t, r.repo, map[string]string{"go.sum": "h1:two\n"}) // a later checkout changes the lockfile
	if res := r.verify(); !res.Verified {
		t.Fatalf("later checkout: %+v", res)
	}
	other := t.TempDir()
	git(t, other, "init", "-q", "-b", "main")
	commit(t, other, map[string]string{"go.mod": "module y\n"})
	r.opt.Root = other
	wantFail(t, r.verify(), verify.ReasonRevisionAbsent, 11)
	r.opt.Root = ""
	wantFail(t, r.verify(), verify.ReasonRevisionAbsent, 11)
}

func TestTamperMatrix(t *testing.T) {
	type tamper struct {
		name   string
		mode   trustMode
		bend   func(r *rig) // before publish
		after  func(r *rig) // after publish
		reason string
		exit   int
	}
	otherID := func(r *rig) string { return signing.KeyID(signing.PurposeCIRelease, r.otherPub) }
	cases := []tamper{
		{name: "swapped artifact", after: func(r *rig) { r.write("artifacts/nself-linux-amd64", []byte("evil")) }, reason: verify.ReasonChecksum},
		{name: "edited SHA256SUMS", after: func(r *rig) {
			r.write("SHA256SUMS", []byte(strings.Replace(string(r.read("SHA256SUMS")), "  artifacts/", "  artifacts/", 1)+"\n"))
		}, reason: verify.ReasonChecksum},
		{name: "SHA256SUMS lacks a line", after: func(r *rig) {
			lines := strings.SplitAfter(string(r.read("SHA256SUMS")), "\n")
			r.write("SHA256SUMS", []byte(strings.Join(lines[1:], "")))
		}, reason: verify.ReasonChecksum},
		{name: "unlisted file", after: func(r *rig) { r.write("artifacts/extra", []byte("x")) }, reason: verify.ReasonChecksum},
		{name: "symlinked artifact", after: func(r *rig) {
			p := filepath.Join(r.dir, "artifacts", "nself-linux-amd64")
			_ = os.Remove(p)
			if err := os.Symlink(filepath.Join(r.dir, "evidence.json"), p); err != nil {
				t.Skip("no symlinks here")
			}
		}, reason: verify.ReasonChecksum},
		{name: "artifact size", bend: func(r *rig) { r.tweakManifest = func(m *manifest.Manifest) { m.Artifacts[0].Size++ } }, reason: verify.ReasonChecksum},
		{name: "manifest digest of an artifact", bend: func(r *rig) {
			r.tweakManifest = func(m *manifest.Manifest) { m.Artifacts[0].SHA256 = strings.Repeat("f", 64) }
		}, reason: verify.ReasonChecksum},
		{name: "edited sbom", after: func(r *rig) { r.write("sbom/nself-linux-amd64.cdx.json", []byte("edited")) }, reason: verify.ReasonChecksum},
		{name: "SHA256SUMS digest in the manifest", bend: func(r *rig) {
			r.tweakManifest = func(m *manifest.Manifest) { m.Checksums.SHA256 = strings.Repeat("0", 64) }
		}, reason: verify.ReasonChecksum},
		{name: "plugins-purpose key", bend: func(r *rig) {
			r.manSigner = signing.NewEd25519Signer(signing.PurposePlugins, seedKey(1))
			r.tweakManifestPayload = func(b []byte) []byte { // manifest.Build refuses a key id that is not ci-release-
				return []byte(strings.Replace(string(b), r.keyID(), r.manSigner.KeyID(), 1))
			}
		}, reason: verify.ReasonKeyPurpose},
		{name: "revoked key", after: func(r *rig) { writeFile(t, filepath.Dir(r.revokedFile), "ci-release.revoked", r.keyID()+"\n") }, reason: verify.ReasonRevoked},
		{name: "unknown key", bend: func(r *rig) {
			r.manSigner = r.other
			r.tweakManifest = func(m *manifest.Manifest) { m.Signing.KeyID = otherID(r) }
		}, reason: verify.ReasonSignature},
		{name: "manifest names another key", bend: func(r *rig) { r.tweakManifest = func(m *manifest.Manifest) { m.Signing.KeyID = otherID(r) } }, reason: verify.ReasonSignature},
		{name: "two signatures", bend: func(r *rig) { r.manSigs = 2 }, reason: verify.ReasonSignature},
		{name: "wrong payloadType", bend: func(r *rig) { r.manType = "application/json" }, reason: verify.ReasonPayloadType},
		{name: "unknown major", bend: func(r *rig) {
			r.tweakManifestPayload = func(b []byte) []byte {
				return []byte(strings.Replace(string(b), `"schema":"ci.release-manifest/v1"`, `"schema":"ci.release-manifest/v2"`, 1))
			}
		}, reason: verify.ReasonManifestSchema},
		{name: "invalid manifest", bend: func(r *rig) {
			r.tweakManifestPayload = func(b []byte) []byte {
				return []byte(strings.Replace(string(b), `"revision":"`, `"revision":"zz`, 1))
			}
		}, reason: verify.ReasonManifestInvalid},
		{name: "manifest is not JSON", after: func(r *rig) { r.write(verify.ManifestFile, []byte("not json")) }, reason: verify.ReasonEnvelope},
		{name: "manifest missing", after: func(r *rig) { _ = os.Remove(filepath.Join(r.dir, verify.ManifestFile)) }, reason: verify.ReasonEnvelope},
		{name: "manifest envelope repeats a key", after: func(r *rig) {
			r.write(verify.ManifestFile, []byte(strings.Replace(string(r.read(verify.ManifestFile)), `{"payloadType"`, `{"payloadType":"x","payloadType"`, 1)))
		}, reason: verify.ReasonEnvelope},
		{name: "payload swapped under the signature", after: func(r *rig) {
			var env signing.Envelope
			if err := json.Unmarshal(r.read(verify.ManifestFile), &env); err != nil {
				t.Fatal(err)
			}
			env.Payload = "e30="
			b, _ := json.Marshal(env)
			r.write(verify.ManifestFile, b)
		}, reason: verify.ReasonSignature},
		{name: "missing sbom", bend: func(r *rig) { r.skipSBOM = "nself-linux-amd64" }, reason: verify.ReasonSBOMLink},
		{name: "sbom not bound by the provenance", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.SBOMs = f.SBOMs[1:] }
		}, reason: verify.ReasonSBOMLink},
		{name: "sbom digest in the provenance differs", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.SBOMs[0].SHA256 = strings.Repeat("1", 64) }
		}, reason: verify.ReasonSBOMLink},
		{name: "provenance for another digest", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.Subjects[0].SHA256 = strings.Repeat("1", 64) }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance names fewer artifacts", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.Subjects = f.Subjects[:1] }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance for another revision", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.Revision = strings.Repeat("c", 40) }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance for another repository", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.RepoID = "local:" + strings.Repeat("d", 64) }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance for another pipeline", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.Pipeline = "nightly" }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance for another job", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.Job = "other" }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance for another invocation", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.RunID = "run-2" }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance finished at another time", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.FinishedOn = f.FinishedOn.Add(2e9) }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance carries another evidence digest", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.Evidence = strings.Repeat("1", 64) }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance of the agent buildType", bend: func(r *rig) {
			r.tweakProvPayload = func(b []byte) []byte {
				return []byte(strings.Replace(string(b), provenance.BuildTypeRelease, provenance.BuildTypeAgent, 1))
			}
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance builder is not nself-ci", bend: func(r *rig) {
			r.tweakProvPayload = func(b []byte) []byte {
				return []byte(strings.Replace(string(b), "https://nself.org/ci/builder/v1#", "https://example.org/builder#", 1))
			}
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance payload is not a statement", bend: func(r *rig) { r.tweakProvPayload = func([]byte) []byte { return []byte(`{"x":1}`) } }, reason: verify.ReasonProvenanceSubject},
		{name: "provenance signed by another key", bend: func(r *rig) { r.provSigner = r.other }, reason: verify.ReasonProvenanceSig},
		{name: "provenance payloadType", bend: func(r *rig) { r.provType = "application/json" }, reason: verify.ReasonProvenanceSig},
		{name: "provenance is not an envelope", bend: func(r *rig) { r.tweakManifest = func(m *manifest.Manifest) {} },
			after: func(r *rig) {
				doc := []byte("garbage")
				r.write(manifest.ProvenancePath, doc)
				r.resignWith(func(m *manifest.Manifest) { m.Provenance.SHA256 = hexOf(doc) }, true)
			}, reason: verify.ReasonProvenanceSig},
		{name: "evidence of another revision", bend: func(r *rig) { r.tweakEvidence = func(e *model.Evidence) { e.Revision = r.base } }, reason: verify.ReasonRevision},
		{name: "evidence of another run", bend: func(r *rig) { r.tweakEvidence = func(e *model.Evidence) { e.RunID = "run-9" } }, reason: verify.ReasonEvidenceMismatch},
		{name: "evidence of another repository", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) { e.RepoID = "local:" + strings.Repeat("d", 64) }
		}, reason: verify.ReasonEvidenceMismatch},
		{name: "evidence finished at another time", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) { x := "2026-10-19T12:00:01Z"; e.Timings.FinishedAt = &x }
		}, reason: verify.ReasonEvidenceMismatch},
		{name: "evidence without a finish time", bend: func(r *rig) { r.tweakEvidence = func(e *model.Evidence) { e.Timings.FinishedAt = nil } }, reason: verify.ReasonEvidenceMismatch},
		{name: "evidence is not release class", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) { e.Checks[1].Kind = "test" }
		}, reason: verify.ReasonEvidenceMismatch},
		{name: "evidence digest differs from the manifest", bend: func(r *rig) {
			r.tweakManifest = func(m *manifest.Manifest) { m.Evidence.SHA256 = strings.Repeat("1", 64) }
		}, reason: verify.ReasonEvidenceMismatch},
		{name: "evidence is inconsistent", bend: func(r *rig) { r.tweakEvidence = func(e *model.Evidence) { e.Counts.Pass = 7 } }, reason: verify.ReasonEvidenceInvalid},
		{name: "evidence lacks an artifact", bend: func(r *rig) { r.tweakEvidence = func(e *model.Evidence) { e.Artifacts = e.Artifacts[:1] } }, reason: verify.ReasonEvidenceMismatch},
		{name: "evidence artifact not committed", bend: func(r *rig) { r.tweakEvidence = func(e *model.Evidence) { e.Artifacts[0].Committed = false } }, reason: verify.ReasonEvidenceMismatch},
		{name: "evidence stale at the revision", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) { e.InputDigests.Lockfiles[0].SHA256 = strings.Repeat("1", 64) }
		}, reason: verify.ReasonEvidenceStale},
		{name: "evidence unbound", bend: func(r *rig) {
			why := model.BindingReason("dirty")
			stale := "worktree is dirty"
			r.tweakEvidence = func(e *model.Evidence) {
				e.Binding = model.Binding{State: "unbound", Reason: &why}
				e.WorktreeState = "dirty"
				e.Freshness = model.Freshness{State: "unknown", Reason: &stale}
			}
		}, reason: verify.ReasonEvidenceUnbound},
		{name: "evidence selection partial", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) { e.Selection.Mode = "affected" }
		}, reason: verify.ReasonEvidenceFailing, exit: 1},
		{name: "evidence adhoc", bend: func(r *rig) { r.tweakEvidence = func(e *model.Evidence) { e.Pipeline.Kind = "adhoc" } }, reason: verify.ReasonEvidenceFailing, exit: 1},
		{name: "trust floor", bend: func(r *rig) { r.tweakEvidence = func(e *model.Evidence) { e.SourceTrust = "untrusted" } }, reason: verify.ReasonEvidenceInvalid},
		{name: "release directory missing", after: func(r *rig) { r.dir = filepath.Join(r.dir, "nope") }, reason: verify.ReasonReleaseMissing},
		{name: "release path is a file", after: func(r *rig) { r.dir = filepath.Join(r.dir, "evidence.json") }, reason: verify.ReasonReleaseMissing},
		{name: "operator keys file is malformed", after: func(r *rig) {
			writeFile(t, filepath.Dir(r.keysFile), "ci-release.keys", "ci-release-0000000000000000 !!!\n")
		}, reason: verify.ReasonSignature},
		{name: "revoked file is malformed", after: func(r *rig) { writeFile(t, filepath.Dir(r.revokedFile), "ci-release.revoked", "not a key id\n") }, reason: verify.ReasonSignature},
		{name: "no trust roots", after: func(r *rig) { r.opt.OperatorKeys = "" }, reason: verify.ReasonSignature},
		{name: "revocation hook", after: func(r *rig) { r.opt.Revoked = func(id string) bool { return id == r.keyID() } }, reason: verify.ReasonRevoked},
		{name: "sums line differs from the file", after: func(r *rig) {
			r.resignSums(func(s string) string { return strings.Repeat("f", 64) + s[64:] })
		}, reason: verify.ReasonChecksum},
		{name: "listed artifact is missing", after: func(r *rig) { _ = os.Remove(filepath.Join(r.dir, "artifacts", "nself-linux-amd64")) }, reason: verify.ReasonChecksum},
		{name: "provenance config digest differs from the evidence", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.ConfigDigest = strings.Repeat("9", 64) }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance lockfile digest differs from the evidence", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.Lockfiles[0].SHA256 = strings.Repeat("f", 64) }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance names no lockfiles", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) { f.Lockfiles = nil }
		}, reason: verify.ReasonProvenanceSubject},
		{name: "provenance names a lockfile the evidence lacks", bend: func(r *rig) {
			r.tweakFacts = func(f *provenance.ReleaseFacts) {
				f.Lockfiles = append(f.Lockfiles, provenance.FileDigest{Name: "extra.lock", SHA256: strings.Repeat("1", 64)})
			}
		}, reason: verify.ReasonProvenanceSubject},
		{name: "release is for another repository than the one checked", bend: func(r *rig) {
			r.repoID = "local:" + strings.Repeat("d", 64)
		}, reason: verify.ReasonEvidenceMismatch},
		{name: "failing release check is exit 1", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) {
				e.Checks[1].Result = "fail"
				e.Result = "fail"
				e.Counts = model.Counts{Pass: 1, Fail: 1, Total: 2}
			}
		}, reason: verify.ReasonEvidenceFailing, exit: 1},
		{name: "evidence lacks the release job", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) { e.Checks[1].JobID = "other"; e.Checks[1].Required = false }
		}, reason: verify.ReasonEvidenceMismatch},
		{name: "release check belongs to another job", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) { e.Checks[0].Kind = "release"; e.Checks[1].Kind = "test" }
		}, reason: verify.ReasonEvidenceMismatch},
		{name: "pin could not be resolved", after: func(r *rig) { r.opt.PinError = "origin unreachable" }, reason: verify.ReasonSignature},
		{name: "pinned commit is not in the repository", mode: pinnedKeys, after: func(r *rig) { r.opt.Pin = strings.Repeat("a", 40) }, reason: verify.ReasonSignature},
		{name: "protected policy digest differs", after: func(r *rig) { r.opt.PolicyDigest = "policy-2" }, reason: verify.ReasonEvidenceInvalid},
		{name: "protected policy requires a job the evidence lacks", after: func(r *rig) { r.opt.RequiredJobs = []string{"build", "security"} }, reason: verify.ReasonEvidenceInvalid},
		{name: "failing and quarantined", bend: func(r *rig) {
			r.tweakEvidence = func(e *model.Evidence) { e.Selection.Mode = "affected" }
		}, after: func(r *rig) {
			r.opt.Quarantined = func(context.Context, model.Evidence) (bool, error) { return true, nil }
		}, reason: verify.ReasonQuarantined, exit: 1},
		{name: "quarantined", after: func(r *rig) {
			r.opt.Quarantined = func(context.Context, model.Evidence) (bool, error) { return true, nil }
		}, reason: verify.ReasonQuarantined, exit: 1},
		{name: "quarantine unreadable", after: func(r *rig) {
			r.opt.Quarantined = func(context.Context, model.Evidence) (bool, error) { return false, errors.New("store down") }
		}, reason: verify.ReasonQuarantined, exit: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t, c.mode)
			if c.bend != nil {
				c.bend(r)
				r.publish()
			}
			if c.after != nil {
				c.after(r)
			}
			exit := c.exit
			if exit == 0 {
				exit = 11
			}
			wantFail(t, r.verify(), c.reason, exit)
		})
	}
}

// resignWith rewrites the manifest after a change to the files, keeping the
// release otherwise as it is on disk.
func (r *rig) resignWith(mod func(*manifest.Manifest), sums bool) {
	r.t.Helper()
	var env signing.Envelope
	if err := json.Unmarshal(r.read(verify.ManifestFile), &env); err != nil {
		r.t.Fatal(err)
	}
	raw, err := base64decode(env.Payload)
	if err != nil {
		r.t.Fatal(err)
	}
	m, err := manifest.Decode(raw)
	if err != nil {
		r.t.Fatal(err)
	}
	mod(&m)
	if sums { // keep SHA256SUMS in step with the rewritten provenance file
		lines := strings.Split(strings.TrimSuffix(string(r.read(manifest.ChecksumsPath)), "\n"), "\n")
		for i, l := range lines {
			if strings.HasSuffix(l, "  "+manifest.ProvenancePath) {
				lines[i] = m.Provenance.SHA256 + "  " + manifest.ProvenancePath
			}
		}
		body := strings.Join(lines, "\n") + "\n"
		r.write(manifest.ChecksumsPath, []byte(body))
		m.Checksums.SHA256 = hexOf([]byte(body))
	}
	payload, err := manifest.Build(m)
	if err != nil {
		r.t.Fatal(err)
	}
	signed, err := signing.SignEnvelope(r.signer, r.manType, payload)
	if err != nil {
		r.t.Fatal(err)
	}
	b, _ := json.Marshal(signed)
	r.write(verify.ManifestFile, b)
}

func TestResultAlwaysCarriesExactlyL1(t *testing.T) {
	r := newRig(t, operatorKeys)
	good := r.verify()
	r.write("artifacts/nself-linux-amd64", []byte("evil"))
	bad := r.verify()
	for name, res := range map[string]verify.Result{"good": good, "bad": bad} {
		b, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err = json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		if doc["slsa_build_level"] != "L1" || strings.Count(string(b), "slsa_build_level") != 1 {
			t.Fatalf("%s: %s", name, b)
		}
	}
	if (verify.Result{}).SLSABuildLevel != "" && provenance.Level != "L1" {
		t.Fatal("level")
	}
	m := manifest.Manifest{}
	if m.Provenance.SLSABuildLevel != "" {
		t.Fatal("zero manifest carries a level")
	}
}

func TestFailureExitCodes(t *testing.T) {
	for reason, want := range map[string]int{
		verify.ReasonEvidenceFailing: 1, verify.ReasonQuarantined: 1,
		verify.ReasonSignature: 11, verify.ReasonEvidenceStale: 11, verify.ReasonEvidenceUnbound: 11, verify.ReasonRevisionAbsent: 11,
		verify.ReasonChecksum: 11, verify.ReasonEnvelope: 11,
	} {
		f := &verify.Failure{Reason: reason, Message: "m"}
		if f.ExitCode() != want || !strings.HasPrefix(f.Error(), "E719 "+reason+": ") {
			t.Errorf("%s: exit %d, error %q", reason, f.ExitCode(), f.Error())
		}
	}
}

// resignSums rewrites SHA256SUMS with mod and signs a manifest that names it.
func (r *rig) resignSums(mod func(string) string) {
	r.t.Helper()
	body := mod(string(r.read(manifest.ChecksumsPath)))
	r.write(manifest.ChecksumsPath, []byte(body))
	r.resignWith(func(m *manifest.Manifest) { m.Checksums.SHA256 = hexOf([]byte(body)) }, false)
}

func TestNoQuarantineSourceIsSaidInTheWarnings(t *testing.T) {
	r := newRig(t, operatorKeys)
	r.opt.Quarantined = nil
	res := r.verify()
	if !res.Verified || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "quarantine state was not checked") {
		t.Fatalf("%+v", res)
	}
}

// A local refs/replace that points the pinned commit at the released one must
// not make the released revision's keys a trust root.
func TestReplacementObjectsDoNotSwapThePinnedTree(t *testing.T) {
	r := newRig(t, releasedOnly)
	r.ownRepo()
	git(t, r.repo, "replace", r.base, r.rev)
	wantFail(t, r.verify(), verify.ReasonSignature, 11)
}

func TestProtectedPolicyOrTheEvidenceOwnRequirements(t *testing.T) {
	r := newRig(t, operatorKeys)
	if res := r.verify(); !res.Verified || len(res.Warnings) != 0 {
		t.Fatalf("protected policy supplied: %+v", res)
	}
	for name, mutate := range map[string]func(*verify.Options){
		"no digest":       func(o *verify.Options) { o.PolicyDigest = "" },
		"no required job": func(o *verify.Options) { o.RequiredJobs = nil },
		"neither":         func(o *verify.Options) { o.PolicyDigest, o.RequiredJobs = "", nil },
	} {
		r = newRig(t, operatorKeys)
		mutate(&r.opt)
		res := r.verify()
		if !res.Verified || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "no protected policy was supplied") {
			t.Errorf("%s: %+v", name, res)
		}
	}
}
