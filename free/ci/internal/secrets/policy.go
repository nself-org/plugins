package secrets

// Purpose: decide whether a job's refs may be resolved on this runner, before any resolver call.
// Inputs:  the parsed refs, the job's trust facts, the runner's facts and the effective policy's
// `trust.secrets` class allowlist.
// Outputs: the refusal reasons (empty = eligible); never a partial result.
// Constraints: pure. The trust package owns the eligibility rules (SecretEligibility); this file
// adds only the policy allowlist, which fails closed when the policy names an allowlist.

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Facts is everything the eligibility decision needs besides the refs.
type Facts struct {
	Job    trust.JobFacts
	Runner trust.RunnerFacts
	// Allowed is the effective policy's `trust.secrets` list; HasPolicy is false when the policy
	// has no such key (then only SecretEligibility applies).
	Allowed   []string
	HasPolicy bool
	// Declared are the classes the runner declares (its Declaration.SecretClasses); HasDeclared is
	// false when the caller does not know them (a local run), then no declaration check applies.
	Declared    []model.SecretClass
	HasDeclared bool
}

// AllowlistOf reads `trust.secrets` from an effective policy.
func AllowlistOf(e trust.Effective) (allowed []string, ok bool) {
	v, _, found := e.Value("trust.secrets")
	if !found {
		return nil, false
	}
	return AllowlistOfValue(v)
}

// AllowlistOfValue reads the value of the `trust.secrets` key; ok is true when the key is present.
func AllowlistOfValue(v any) (allowed []string, ok bool) {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...), true
	case []any:
		out := []string{}
		for _, y := range x {
			if s, isStr := y.(string); isStr {
				out = append(out, s)
			}
		}
		return out, true
	}
	return []string{}, true // present but unreadable: allow nothing
}

// Check returns the reasons the refs may not be resolved here. It never resolves anything.
func Check(refs []Ref, f Facts) []trust.Reason {
	if len(refs) == 0 {
		return nil
	}
	job := f.Job
	job.SecretClasses = Classes(refs)
	reasons := trust.SecretEligibility(job, f.Runner)
	if f.HasDeclared {
		for _, class := range job.SecretClasses {
			if !hasClass(f.Declared, class) {
				reasons = append(reasons, trust.SecretsClassNotDeclared)
				break
			}
		}
	}
	if f.HasPolicy {
		for _, class := range job.SecretClasses {
			if !contains(f.Allowed, string(class)) {
				reasons = append(reasons, trust.SecretsNotInPolicy)
				break
			}
		}
	}
	return dedupe(reasons)
}

func hasClass(list []model.SecretClass, want model.SecretClass) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func dedupe(in []trust.Reason) []trust.Reason {
	var out []trust.Reason
	seen := map[trust.Reason]bool{}
	for _, r := range in {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

func reasonStrings(rs []trust.Reason) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}
