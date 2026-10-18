package attest

// Purpose: decide whether evidence satisfies a gate (Epic D14).
// Inputs:  one evidence document and the Gate the caller built from its own
// protected policy (never from the document).
// Outputs: every reason the evidence fails, in a fixed order; an empty list
// means the evidence is accepted.
// Constraints: pure and fail-closed: an empty or unknown gate field never
// matches, unknown freshness is stale, a zero Gate accepts nothing. Adhoc and
// drill pipelines never gate. Self-reported evidence passes only a default
// class gate whose operator policy admitted it, never a release gate.

import (
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// GateClass is the strictness of a gate.
type GateClass string

const (
	// ClassDefault is an ordinary gate. Any other value is treated as release.
	ClassDefault GateClass = "default"
	// ClassRelease guards a release: full selection, no self-reported evidence.
	ClassRelease GateClass = "release"
)

// Gate is what a caller requires of evidence. The caller builds it from the
// protected policy and the current checkout; nothing in it comes from the
// evidence being judged.
type Gate struct {
	RepoID       string           `json:"repo_id"`
	Revision     string           `json:"revision"` // the exact revision the gate is about
	PolicyDigest string           `json:"policy_digest"`
	RequiredJobs []string         `json:"required_jobs"` // from the protected policy, never from the revision
	MinTrust     model.TrustClass `json:"min_trust"`
	Class        GateClass        `json:"class"`
	// AllowSelfReported is the operator-scope policy decision to admit
	// self-reported evidence; it is ignored for a release gate.
	AllowSelfReported bool `json:"allow_self_reported"`
	// FullSelection demands selection.mode full (a release gate always does).
	FullSelection bool `json:"full_selection"`
	// Current holds the digests of the inputs now; nil means unknown, which
	// never satisfies a gate.
	Current *model.InputDigests `json:"current,omitempty"`
}

func (g Gate) release() bool { return g.Class != ClassDefault }

// GateAccept returns every reason ev does not satisfy g.
func GateAccept(ev model.Evidence, g Gate) []Reason {
	set := reasonSet{}
	set.add(consistency(ev)...)
	if ev.RepoID == "" || ev.RepoID != g.RepoID {
		set.add(ReasonRepoMismatch)
	}
	if ev.Revision == "" || ev.Revision != g.Revision {
		set.add(ReasonRevisionMismatch)
	}
	if ev.PolicyDigest == "" || ev.PolicyDigest != g.PolicyDigest {
		set.add(ReasonPolicyMismatch)
	}
	set.add(jobReasons(ev, g.RequiredJobs)...)
	// an unknown class ranks -1: below every known minimum, and an unknown minimum is never met
	if got, min := trust.RankTrust(ev.SourceTrust), trust.RankTrust(g.MinTrust); min < 0 || got < min {
		set.add(ReasonTrustBelowMin)
	}
	if ev.Binding.State != "bound" || ev.WorktreeState != "clean" {
		set.add(ReasonUnbound)
	}
	if ev.Pipeline.Kind != "default" && ev.Pipeline.Kind != "named" {
		set.add(ReasonAdhoc)
	}
	if g.Current == nil || Fresh(ev, *g.Current).State != "current" {
		set.add(ReasonStale)
	}
	if (g.release() || g.FullSelection) && ev.Selection.Mode != "full" {
		set.add(ReasonSelectionPartial)
	}
	if ev.Result != "pass" {
		set.add(ReasonNotPassing)
	}
	set.add(producerReasons(ev, g)...)
	return set.list()
}

// jobReasons requires every job the gate names to appear in the evidence with
// only passing checks; a job the revision removed or skipped is a mismatch.
func jobReasons(ev model.Evidence, required []string) []Reason {
	if len(required) == 0 {
		return []Reason{ReasonJobsMismatch} // a gate that requires nothing cannot be satisfied
	}
	seen := map[string]bool{}
	bad := map[string]bool{}
	for _, c := range ev.Checks {
		seen[c.JobID] = true
		if c.Result != "pass" {
			bad[c.JobID] = true
		}
	}
	for _, job := range required {
		if job == "" || !seen[job] {
			return []Reason{ReasonJobsMismatch}
		}
		if bad[job] {
			return []Reason{ReasonNotPassing}
		}
	}
	return nil
}

// producerReasons judges who recorded the document.
func producerReasons(ev model.Evidence, g Gate) []Reason {
	switch ev.Producer.Kind {
	case "coordinator-recorded":
		return nil
	case "signed-remote":
		doc, err := json.Marshal(ev)
		if err != nil {
			return []Reason{ReasonSignatureUnverifiable}
		}
		return verifySignature(doc, ev)
	case "self-reported":
		if g.release() || !g.AllowSelfReported {
			return []Reason{ReasonSelfReportedNotAdmitted}
		}
		return nil
	}
	return []Reason{ReasonInvalid}
}
