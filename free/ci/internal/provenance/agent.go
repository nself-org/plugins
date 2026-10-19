package provenance

// Purpose: BuildAgent, the producer of the provenance statement of an
// nself-ci-agent release (Epic CACHE D17, P7-CACHE-10's tools/provgen).
// Inputs:  AgentFacts: the GitHub repository, the workflow ref and run, the
// built binaries and their SBOM digests.
// Outputs: canonical statement bytes, buildType agent/v1, builder id
// "https://github.com/<repo>/.github/workflows/release-agent.yml@<ref>".
// Constraints: pure; agent releases come from a public GitHub workflow, so
// there is nothing private to scrub; the level is the same Build L1 as for a
// fabric release (GitHub's own attestation, if any, is labelled as GitHub's).

import (
	"regexp"
	"sort"
	"time"
)

// AgentWorkflow is the workflow file that builds an agent release.
const AgentWorkflow = ".github/workflows/release-agent.yml"

// AgentFacts are the facts of one agent release.
type AgentFacts struct {
	Repo     string // owner/name on GitHub
	Ref      string // the workflow ref, for example refs/tags/v1.5.0
	Revision string // the commit the tag points at
	RunID    string // the workflow run id (digits)

	StartedOn, FinishedOn time.Time // optional; both or neither
	Subjects              []FileDigest
	SBOMs                 []FileDigest
}

var (
	repoRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	refRe   = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,200}$`)
	runIDRe = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// BuildAgent returns the canonical statement bytes of an agent release.
func BuildAgent(f AgentFacts) ([]byte, error) {
	if !repoRe.MatchString(f.Repo) || !refRe.MatchString(f.Ref) || !runIDRe.MatchString(f.RunID) || !revisionRe.MatchString(f.Revision) {
		return nil, invalid("agent repo, ref, run id or revision is malformed")
	}
	subjects, err := files("subject", f.Subjects, 1)
	if err != nil {
		return nil, err
	}
	sboms, err := files("sbom", f.SBOMs, 0)
	if err != nil {
		return nil, err
	}
	meta := Metadata{InvocationID: "https://github.com/" + f.Repo + "/actions/runs/" + f.RunID}
	if !f.StartedOn.IsZero() || !f.FinishedOn.IsZero() {
		if f.FinishedOn.Before(f.StartedOn) {
			return nil, invalid("finishedOn is before startedOn")
		}
		if meta.StartedOn, err = rfc3339("startedOn", f.StartedOn); err != nil {
			return nil, err
		}
		if meta.FinishedOn, err = rfc3339("finishedOn", f.FinishedOn); err != nil {
			return nil, err
		}
	}
	sort.Slice(sboms, func(i, j int) bool { return sboms[i].Name < sboms[j].Name })
	subs := make([]Subject, 0, len(subjects))
	for _, s := range subjects {
		subs = append(subs, Subject{Name: s.Name, Digest: map[string]string{"sha256": s.SHA256}})
	}
	repoURL := "https://github.com/" + f.Repo
	return encode(Statement{Type: StatementType, Subject: subs, PredicateType: PredicateType, Predicate: Predicate{
		BuildDefinition: BuildDefinition{BuildType: BuildTypeAgent,
			ExternalParameters:   map[string]string{"repository": repoURL, "ref": f.Ref, "workflow": AgentWorkflow},
			InternalParameters:   map[string]string{},
			ResolvedDependencies: []ResourceDescriptor{{URI: "git+" + repoURL + "@" + f.Ref, Digest: map[string]string{"gitCommit": f.Revision}}}},
		RunDetails: RunDetails{Builder: Builder{ID: repoURL + "/" + AgentWorkflow + "@" + f.Ref}, Metadata: meta, Byproducts: descriptors(sboms)},
	}})
}
