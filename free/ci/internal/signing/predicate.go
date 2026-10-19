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
// run; everything else in the predicate is carried through unchecked.

import (
	"encoding/json"
	"strings"
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
		} `json:"externalParameters"`
		ResolvedDependencies []struct {
			URI    string            `json:"uri"`
			Digest map[string]string `json:"digest"`
		} `json:"resolvedDependencies"`
	} `json:"buildDefinition"`
	RunDetails struct {
		Metadata struct {
			InvocationID string `json:"invocationId"`
		} `json:"metadata"`
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
	return attempt, nil
}
