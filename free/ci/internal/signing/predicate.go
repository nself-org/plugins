package signing

// Purpose: re-check the provenance predicate of an in-toto statement against
// the stored run (Epic CACHE §Contracts, Provenance statement): the signed
// provenance is the claim "this revision of this repository was built by this
// run", so the signer reads those claims from the store, never from the caller.
// Inputs:  the statement's predicate bytes and the run's facts.
// Outputs: the release attempt id named by runDetails.metadata.invocationId.
// Constraints: buildType is the release build type; externalParameters name
// the repository of the stored evidence and the revision of the run; the
// resolved dependencies hold the git commit of that revision; the invocation
// id is "<run id>/<attempt id>" of a passed attempt of a release job of the
// run; the job is the attempt's job, the builder id is a release builder id and
// the evidence.json byproduct carries the stored evidence digest; everything
// else in the predicate is carried through unchecked.

import (
	"encoding/json"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

func cutPrefix(s, prefix string) (string, bool) { return strings.CutPrefix(s, prefix) }

// ReleaseBuildType is the buildType of a release provenance predicate.
const ReleaseBuildType = "https://nself.org/ci/buildtypes/release/v1"

type predicate struct {
	BuildDefinition struct {
		BuildType          string `json:"buildType"`
		ExternalParameters struct {
			Repository string `json:"repository"`
			Revision   string `json:"revision"`
			Job        string `json:"job"`
		} `json:"externalParameters"`
		ResolvedDependencies []struct {
			URI    string            `json:"uri"`
			Digest map[string]string `json:"digest"`
		} `json:"resolvedDependencies"`
	} `json:"buildDefinition"`
	RunDetails struct {
		Builder struct {
			ID string `json:"id"`
		} `json:"builder"`
		Metadata struct {
			InvocationID string `json:"invocationId"`
		} `json:"metadata"`
		Byproducts []struct {
			Name   string            `json:"name"`
			Digest map[string]string `json:"digest"`
		} `json:"byproducts"`
	} `json:"runDetails"`
}

// checkPredicate validates the predicate and returns the release attempt it names.
func (f *runFacts) checkPredicate(raw json.RawMessage) (string, error) {
	var p predicate
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", Failf(ReasonPayloadRejected, "statement: the predicate is not a SLSA provenance v1 object")
	}
	bd := p.BuildDefinition
	if bd.BuildType != ReleaseBuildType || bd.ExternalParameters.Repository != f.evidence.RepoID || bd.ExternalParameters.Revision != f.pipe.Revision {
		return "", Failf(ReasonPayloadRejected, "statement: buildType, repository or revision differ from the stored run")
	}
	commit := "git+" + f.evidence.RepoID + "@" + f.pipe.Revision
	pinned := false
	for _, d := range bd.ResolvedDependencies {
		if d.URI == commit && d.Digest["gitCommit"] == f.pipe.Revision {
			pinned = true
		}
	}
	if !pinned {
		return "", Failf(ReasonPayloadRejected, "statement: resolvedDependencies do not pin the revision of the run")
	}
	attempt, ok := cutPrefix(p.RunDetails.Metadata.InvocationID, f.runID+"/")
	if !ok || !f.passedRelease(attempt) {
		return "", Failf(ReasonPayloadRejected, "statement: invocationId is not <run>/<passed release attempt> of this run")
	}
	if bd.ExternalParameters.Job != f.jobName(attempt) || !strings.HasPrefix(p.RunDetails.Builder.ID, builderPrefix) || !f.byproduct(p, manifest.EvidencePath, f.evDigest) {
		return "", Failf(ReasonPayloadRejected, "statement: job, builder id or the evidence byproduct differ from the stored run")
	}
	return attempt, nil
}

// builderPrefix starts every release builder id.
const builderPrefix = "https://nself.org/ci/builder/v1#"

// jobName is the name of the job the attempt belongs to.
func (f *runFacts) jobName(attempt string) string {
	for _, a := range f.attempts {
		if a.ID != attempt {
			continue
		}
		for _, j := range f.jobs {
			if j.ID == a.JobID {
				return j.Name
			}
		}
	}
	return ""
}

// byproduct reports whether the predicate lists a byproduct with this name and sha256.
func (f *runFacts) byproduct(p predicate, name, digest string) bool {
	for _, b := range p.RunDetails.Byproducts {
		if b.Name == name && b.Digest["sha256"] == digest {
			return true
		}
	}
	return false
}
