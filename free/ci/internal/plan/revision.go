package plan

// Purpose: classify the checked-out revision from git facts the caller
// probed, failing closed: whenever the default branch cannot be known, the
// revision counts as the default branch, so required deep jobs are kept.
// Inputs:  BranchFacts (current branch, detached flag, origin/HEAD target,
// which of origin/main, origin/master, origin/trunk exist, exact-tag flag).
// Outputs: the default branch name ("" when unknown) and the Revision
// flags DefaultBranch and Release.
// Constraints: pure; extra retention is safe, a dropped required job is not.

// BranchFacts are the probed git facts.
type BranchFacts struct {
	Branch     string   // current branch, "" when HEAD is detached
	Detached   bool     // HEAD is not on a branch (tag or CI checkout)
	OriginHead string   // short name origin/HEAD points at, "" when unset
	Remotes    []string // names among main, master, trunk that exist as origin/<name>
	ExactTag   bool     // HEAD is exactly at a tag
}

// DefaultBranchName is the best-known default branch, "" when unknown.
func DefaultBranchName(f BranchFacts) string {
	if f.OriginHead != "" {
		return f.OriginHead
	}
	for _, n := range []string{"main", "master", "trunk"} {
		for _, r := range f.Remotes {
			if r == n {
				return n
			}
		}
	}
	return ""
}

// ClassifyRevision reports whether the revision is on the default branch
// (true when unknown or detached) and whether it is a release.
func ClassifyRevision(f BranchFacts) (defaultBranch, release bool) {
	release = f.ExactTag
	switch {
	case f.Detached || f.Branch == "":
		return true, release
	case f.OriginHead != "":
		return f.Branch == f.OriginHead, release
	case len(f.Remotes) > 0:
		for _, r := range f.Remotes {
			if r == f.Branch {
				return true, release
			}
		}
		return false, release
	}
	return true, release
}
