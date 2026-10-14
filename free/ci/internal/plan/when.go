package plan

// Purpose: evaluate a job's `when` conditions against the revision context.
// Inputs:  model.WhenConfig, Revision, the trigger.
// Outputs: whether the job passes and the reason code when it does not;
// the env_present names that are missing (a separate, non-dropping check).
// Constraints: a condition that cannot be evaluated (no branch known, no
// base) passes; conditions combine with AND, entries within one list with OR.

import (
	"path"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// whenPasses evaluates triggers, branches and paths.
func whenPasses(w *model.WhenConfig, rev Revision, trig model.Trigger) (bool, model.Reason, string) {
	if w == nil {
		return true, "", ""
	}
	if len(w.Triggers) > 0 && !containsTrigger(w.Triggers, trig) {
		return false, "when.trigger", "trigger " + string(trig) + " is not listed"
	}
	if len(w.Branches) > 0 && rev.Branch != "" && !anyGlob(w.Branches, []string{rev.Branch}) {
		return false, "when.branch", "branch " + rev.Branch + " does not match"
	}
	if len(w.Paths) > 0 && rev.BaseKnown && !anyGlob(w.Paths, rev.Changed) {
		return false, "when.paths", "no changed path matches"
	}
	return true, "", ""
}

// envMissing lists the env_present names that are not set, in declared order.
func envMissing(w *model.WhenConfig, rev Revision) []string {
	if w == nil {
		return nil
	}
	set := map[string]bool{}
	for _, n := range rev.Env {
		set[n] = true
	}
	var miss []string
	for _, n := range w.EnvPresent {
		if !set[n] {
			miss = append(miss, n)
		}
	}
	return miss
}

func containsTrigger(list []model.Trigger, t model.Trigger) bool {
	for _, x := range list {
		if x == t {
			return true
		}
	}
	return false
}

func anyGlob(patterns, values []string) bool {
	for _, p := range patterns {
		for _, v := range values {
			if globMatch(p, v) {
				return true
			}
		}
	}
	return false
}

// globMatch matches a slash path against a pattern: `*` and `?` stay inside
// one segment, `**` spans segments, and a pattern without wildcards also
// matches everything below it.
func globMatch(pattern, value string) bool {
	pattern, value = strings.TrimPrefix(pattern, "./"), strings.TrimPrefix(value, "./")
	if !strings.ContainsAny(pattern, "*?[") {
		p := strings.TrimSuffix(pattern, "/")
		return value == p || strings.HasPrefix(value, p+"/")
	}
	return matchSegs(strings.Split(pattern, "/"), strings.Split(value, "/"))
}

func matchSegs(pat, val []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for len(pat) > 1 && pat[1] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(val); i++ {
				if matchSegs(pat[1:], val[i:]) {
					return true
				}
			}
			return false
		}
		if len(val) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], val[0]); err != nil || !ok {
			return false
		}
		pat, val = pat[1:], val[1:]
	}
	return len(val) == 0
}
