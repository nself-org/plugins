package verify

// Purpose: the evidence checks of a release (Epic CACHE D16): the evidence
// file is the one the signed manifest names, for the released revision, run
// and repository; then its verdict at that revision through the one evidence
// verifier (internal/evidence) and authz.Promotable.
// Inputs:  the decoded manifest, evidence.json, the repository of the release.
// Outputs: nil or a Failure: evidence_mismatch, revision, revision_absent,
// evidence_invalid, evidence_unbound, evidence_stale_at_revision (all exit 11),
// quarantined, evidence_failing (exit 1).
// Constraints: freshness is judged against the input digests computed from git
// objects at the manifest's revision, so a later checkout whose lockfile
// changed still verifies, and a revision the repository lacks fails closed.
// The signed manifest authenticates the evidence bytes (their digest is in the
// signed payload), so the document counts as recorded; nothing here
// re-implements the evidence verifier.

import (
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// evidenceBinding checks evidence.json is the document the manifest names.
func (r *run) evidenceBinding() *Failure {
	m := r.man
	doc, err := readFile(r.dir, m.Evidence.Path, maxEvidence)
	if err != nil {
		return fail(ReasonEvidenceMismatch, "cannot read %s: %v", m.Evidence.Path, err)
	}
	if sum(doc) != m.Evidence.SHA256 {
		return fail(ReasonEvidenceMismatch, "%s does not match the evidence digest in the signed manifest", m.Evidence.Path)
	}
	ev, err := attest.Verify(doc)
	if err != nil {
		return fail(ReasonEvidenceInvalid, "%s is not valid evidence: %v", m.Evidence.Path, err)
	}
	switch {
	case ev.Revision != m.Revision:
		return fail(ReasonRevision, "the evidence is for revision %.12s, the release for %.12s", ev.Revision, m.Revision)
	case ev.RepoID != m.RepoID || ev.RunID != m.Release.RunID:
		return fail(ReasonEvidenceMismatch, "the evidence is for another repository or run than the manifest")
	case ev.Timings.FinishedAt == nil || *ev.Timings.FinishedAt != m.Release.FinishedAt:
		return fail(ReasonEvidenceMismatch, "the evidence finished at another time than the release says")
	case evidence.ClassOf(ev) != evidence.ClassRelease:
		return fail(ReasonEvidenceMismatch, "the evidence is not release-class evidence")
	}
	r.ev, r.evDoc = ev, doc
	return nil
}

// Severity of an evidence reason: lower is more fundamental.
const (
	rankMismatch = iota
	rankInvalid
	rankUnbound
	rankStale
	rankFailing
)

func classify(reason attest.Reason) (string, int) {
	switch reason {
	case attest.ReasonRevisionMismatch, attest.ReasonRepoMismatch, authz.ReasonArtifactUnbound:
		return ReasonEvidenceMismatch, rankMismatch
	case attest.ReasonUnbound:
		return ReasonEvidenceUnbound, rankUnbound
	case attest.ReasonStale:
		return ReasonEvidenceStale, rankStale
	case attest.ReasonAdhoc, attest.ReasonSelectionPartial, attest.ReasonNotPassing:
		return ReasonEvidenceFailing, rankFailing
	}
	return ReasonEvidenceInvalid, rankInvalid // inconsistent, other policy, trust below the floor, unverifiable signature
}

// evidenceVerdict judges the evidence at the released revision.
func (r *run) evidenceVerdict() *Failure {
	m, ctx := r.man, r.ctx
	capture := r.opt.capture()
	if r.opt.Root == "" || !evidence.CommitExists(ctx, capture, r.opt.Root, m.Revision) {
		return fail(ReasonRevisionAbsent, "revision %.12s is not in the repository, so the evidence cannot be judged there", m.Revision)
	}
	tree, err := evidence.TreeFS(ctx, capture, r.opt.Root, m.Revision)
	if err != nil {
		return fail(ReasonRevisionAbsent, "cannot read revision %.12s: %v", m.Revision, err)
	}
	current, err := evidence.InputsOf(tree)
	if err != nil {
		return fail(ReasonEvidenceInvalid, "the inputs of revision %.12s cannot be read: %v", m.Revision, err)
	}
	recorded := func(model.Evidence) *model.Evidence { ev := r.ev; return &ev } // the signed manifest vouches for these bytes
	v, err := evidence.VerifyDocument(r.evDoc, m.Revision, &current, recorded)
	if err != nil {
		return fail(ReasonEvidenceInvalid, "%v", err)
	}
	v.MarkMissing(evidence.MissingInputs(v.Report.Evidence, tree))
	all := append([]attest.Reason{}, v.Reasons...)
	gate := attest.Gate{RepoID: r.ev.RepoID, Revision: m.Revision, PolicyDigest: r.ev.PolicyDigest, MinTrust: r.ev.SourceTrust, Current: &current}
	for _, c := range r.ev.Checks {
		if c.Required {
			gate.RequiredJobs = append(gate.RequiredJobs, c.JobID)
		}
	}
	for _, a := range m.Artifacts {
		_, rs := authz.Promotable(authz.Artifact{Project: m.RepoID, Revision: m.Revision, SHA256: a.SHA256}, r.ev, gate)
		all = append(all, rs...)
	}
	r.res.Evidence = reasons(all)
	best, bestRank := "", rankFailing+1
	for _, reason := range all {
		if key, rank := classify(reason); rank < bestRank {
			best, bestRank = key, rank
		}
	}
	if bestRank < rankFailing {
		return fail(best, "the evidence is not usable at revision %.12s (%s)", m.Revision, best)
	}
	if r.opt.Quarantined != nil {
		q, qerr := r.opt.Quarantined(ctx, r.ev)
		if qerr != nil {
			return fail(ReasonQuarantined, "the quarantine state of the evidence could not be read: %v", qerr)
		}
		if q {
			return fail(ReasonQuarantined, "the evidence of run %s is quarantined", r.ev.RunID)
		}
	}
	if bestRank == rankFailing {
		return fail(ReasonEvidenceFailing, "the evidence does not pass")
	}
	r.res.Warnings = append(r.res.Warnings, v.Warnings...)
	return nil
}
