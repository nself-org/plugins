package publish

// Purpose: build and sign the files of a release: the provenance statement,
// SHA256SUMS and the manifest, in that order, because each file names the
// digest of the one before it (the manifest names the signed provenance file
// and SHA256SUMS; SHA256SUMS lists the provenance file).
// Inputs:  *facts and the signer.
// Outputs: *bundle: every metadata file of the release directory and the list
// of artifact and SBOM copies.
// Constraints: all bytes are deterministic (finished_at and started_at come
// from the evidence, objects are key-sorted, Ed25519 is deterministic, the
// envelope file is the compact JSON of the signer's envelope plus a newline);
// the signer is called twice, statement first; nothing is written here.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

// file is one regular file of the release directory.
type file struct {
	path string // slash-separated, relative to the directory
	data []byte // set for metadata files
	rec  *artifact
	kind string // "artifact", "cdx" or "spdx" when data is nil
}

// bundle is a built release.
type bundle struct {
	manifestEnv, provenanceEnv, sums []byte
	manifestSHA, keyID               string
	files                            []file
}

func wrapSigner(err error) error {
	var se *signing.Error
	if errors.As(err, &se) {
		return &Error{ErrCode: signing.CodeSigning, Reason: se.Reason, Msg: se.Msg}
	}
	return refuse(signing.ReasonSignerFailed, "the signer failed")
}

// signOne sends one payload to the signer and returns the envelope file bytes.
func signOne(ctx context.Context, d Deps, runID, payloadType string, payload []byte) ([]byte, error) {
	envs, err := d.Signer.Sign(ctx, runID, []signing.Item{{PayloadType: payloadType, PayloadB64: base64.StdEncoding.EncodeToString(payload)}})
	if err != nil {
		return nil, wrapSigner(err)
	}
	if len(envs) != 1 {
		return nil, refuse(signing.ReasonSignerFailed, "the signer answered %d envelopes for one payload", len(envs))
	}
	return envelopeFile(envs[0])
}

func envelopeFile(e sdk.Envelope) ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, refuse(signing.ReasonSignerFailed, "an envelope could not be encoded")
	}
	return append(b, '\n'), nil
}

func parseTime(field string, p *string, fallback time.Time) (time.Time, error) {
	if p == nil {
		if fallback.IsZero() {
			return time.Time{}, refuse(signing.ReasonPayloadRejected, "the stored evidence has no %s", field)
		}
		return fallback, nil
	}
	t, err := time.Parse(time.RFC3339Nano, *p)
	if err != nil {
		return time.Time{}, refuse(signing.ReasonPayloadRejected, "the stored evidence %s is not an RFC 3339 time", field)
	}
	return t, nil
}

// provenanceFacts maps the run to the facts of the statement.
func (f *facts) provenanceFacts(d Deps, runID, coordinator string) (provenance.ReleaseFacts, error) {
	finished, err := parseTime("timings.finished_at", f.ev.Timings.FinishedAt, time.Time{})
	if err != nil {
		return provenance.ReleaseFacts{}, err
	}
	started, err := parseTime("timings.started_at", f.ev.Timings.StartedAt, finished)
	if err != nil {
		return provenance.ReleaseFacts{}, err
	}
	rf := provenance.ReleaseFacts{Visibility: d.Visibility, RepoID: f.ev.RepoID, Revision: f.pipe.Revision, Pipeline: f.ev.Pipeline.Name, Job: f.job.Name,
		ConfigDigest: f.ev.InputDigests.Config, Isolation: string(f.ev.Runner.Isolation), NselfCIVersion: f.ev.Producer.Version,
		Coordinator: coordinator, Runner: f.ev.Runner.ID, RunID: runID, AttemptID: f.attempt.ID, StartedOn: started, FinishedOn: finished, Evidence: f.evDigest}
	for _, l := range f.ev.InputDigests.Lockfiles {
		rf.Lockfiles = append(rf.Lockfiles, provenance.FileDigest{Name: l.Path, SHA256: l.SHA256})
	}
	for _, a := range f.artifacts {
		rf.Subjects = append(rf.Subjects, provenance.FileDigest{Name: a.rec.Name, SHA256: a.rec.SHA256})
		rf.SBOMs = append(rf.SBOMs, provenance.FileDigest{Name: "sbom/" + a.rec.Name + ".cdx.json", SHA256: a.cdx.SHA256},
			provenance.FileDigest{Name: "sbom/" + a.rec.Name + ".spdx.json", SHA256: a.spdx.SHA256})
	}
	return rf, nil
}

// manifestArtifacts maps the records to manifest artifacts.
func (f *facts) manifestArtifacts() []manifest.Artifact {
	var out []manifest.Artifact
	for _, a := range f.artifacts {
		r := a.rec
		m := manifest.Artifact{Name: r.Name, Path: "artifacts/" + r.Name, Kind: string(r.Kind), SHA256: r.SHA256, Size: r.Size,
			SBOM: manifest.SBOM{CycloneDX: manifest.FileRef{Path: "sbom/" + r.Name + ".cdx.json", SHA256: a.cdx.SHA256},
				SPDX: manifest.FileRef{Path: "sbom/" + r.Name + ".spdx.json", SHA256: a.spdx.SHA256}}}
		if r.Platform != "" {
			p := r.Platform
			m.Platform = &p
		}
		if r.Image != nil {
			im := &manifest.Image{Ref: r.Image.Ref, IndexDigest: r.Image.IndexDigest}
			for _, x := range r.Image.Manifests {
				im.Manifests = append(im.Manifests, manifest.ImageManifest{Platform: x.Platform, Digest: x.Digest})
			}
			m.Image = im
		}
		out = append(out, m)
	}
	return out
}

// checksums renders SHA256SUMS: `<sha256>  <path>`, ascending by path, covering
// the evidence, the provenance file, every artifact and every SBOM.
func checksums(f *facts, provenanceSHA string) []byte {
	lines := map[string]string{manifest.EvidencePath: f.evDigest, manifest.ProvenancePath: provenanceSHA}
	for _, a := range f.artifacts {
		lines["artifacts/"+a.rec.Name] = a.rec.SHA256
		lines["sbom/"+a.rec.Name+".cdx.json"] = a.cdx.SHA256
		lines["sbom/"+a.rec.Name+".spdx.json"] = a.spdx.SHA256
	}
	paths := make([]string, 0, len(lines))
	for p := range lines {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(lines[p] + "  " + p + "\n")
	}
	return []byte(b.String())
}

// build signs and assembles the release of runID.
func build(ctx context.Context, d Deps, runID string, f *facts) (*bundle, error) {
	// The runner and coordinator ids published below are an HMAC under the CI home's key (D-1198); this
	// path also runs outside a run (`nself ci artifacts sign`), so it registers the key itself and
	// refuses to publish the unkeyed, dictionary-recoverable form.
	if err := attest.UsePseudonymKeyAt(d.Home); err != nil {
		return nil, refuse(signing.ReasonSignerFailed, "the publication pseudonym key is not usable")
	}
	info, err := d.Signer.PublicKey(ctx)
	if err != nil {
		return nil, wrapSigner(err)
	}
	coord, err := d.Ledger.AttemptCoordinator(ctx, f.attempt.ID)
	if err != nil {
		return nil, refuse(signing.ReasonSignerFailed, "the run store could not be read")
	}
	if coord == "" {
		coord = "local"
	}
	rf, err := f.provenanceFacts(d, runID, coord)
	if err != nil {
		return nil, err
	}
	stmt, err := provenance.Build(rf)
	if err != nil {
		return nil, refuse(signing.ReasonPayloadRejected, "the provenance statement was refused: %v", err)
	}
	provEnv, err := signOne(ctx, d, runID, provenance.PayloadType, stmt)
	if err != nil {
		return nil, err
	}
	sums := checksums(f, sha(provEnv))
	prof := attest.ProfileFor(attest.RepoFacts{Visibility: d.Visibility, RepoID: f.ev.RepoID})
	payload, err := manifest.Build(manifest.Manifest{Schema: manifest.Schema, RepoID: f.ev.RepoID, Revision: f.pipe.Revision,
		Release: manifest.Release{RunID: runID, Pipeline: f.ev.Pipeline.Name, JobID: f.job.Name, AttemptID: f.attempt.ID,
			IdempotencyKey: releaseKey(f.attempt.ID), FinishedAt: *f.ev.Timings.FinishedAt},
		Evidence:  manifest.EvidenceRef{Path: manifest.EvidencePath, SHA256: f.evDigest, Result: manifest.EvidenceResult, Class: manifest.EvidenceClass},
		Artifacts: f.manifestArtifacts(),
		Provenance: manifest.Provenance{Path: manifest.ProvenancePath, SHA256: sha(provEnv), PredicateType: manifest.ProvenancePredicate,
			SLSABuildLevel: manifest.SLSALevel},
		Checksums: manifest.FileRef{Path: manifest.ChecksumsPath, SHA256: sha(sums)},
		Signing:   manifest.Signing{Purpose: manifest.SigningPurpose, KeyID: info.KeyID},
		Producer:  manifest.Producer{NselfCIVersion: f.ev.Producer.Version, Coordinator: prof.CoordinatorID(coord)}})
	if err != nil {
		return nil, refuse(signing.ReasonPayloadRejected, "the manifest was refused: %v", err)
	}
	manEnv, err := signOne(ctx, d, runID, manifest.PayloadType, payload)
	if err != nil {
		return nil, err
	}
	b := &bundle{manifestEnv: manEnv, provenanceEnv: provEnv, sums: sums, manifestSHA: sha(manEnv), keyID: info.KeyID}
	b.files = append(b.files, file{path: verifyManifestFile, data: manEnv}, file{path: manifest.ChecksumsPath, data: sums},
		file{path: manifest.ProvenancePath, data: provEnv}, file{path: manifest.EvidencePath, data: f.evBytes})
	for i := range f.artifacts {
		a := &f.artifacts[i]
		b.files = append(b.files, file{path: "artifacts/" + a.rec.Name, rec: a, kind: "artifact"},
			file{path: "sbom/" + a.rec.Name + ".cdx.json", rec: a, kind: "cdx"}, file{path: "sbom/" + a.rec.Name + ".spdx.json", rec: a, kind: "spdx"})
	}
	return b, nil
}
