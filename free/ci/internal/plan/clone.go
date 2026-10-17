package plan

// Purpose: deep copies of config values, so a plan never aliases the
// caller's config and no two matrix entries share mutable state.
// Inputs:  a model.Job (or Requirements) as authored or normalised.
// Outputs: an independent copy: every slice, map and pointer is new.
// Constraints: pure; clone_test.go walks the type with reflection and fails
// if a field added to model.Job is shared instead of copied.

import "github.com/nself-org/plugins/free/ci/internal/model"

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string{}, in...)
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// cloneAny copies the YAML/JSON shapes a matrix platform can take.
func cloneAny(v any) any {
	switch x := v.(type) {
	case []string:
		return cloneStrings(x)
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = cloneAny(x[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = cloneAny(e)
		}
		return out
	}
	return v
}

func cloneRequirements(r *model.Requirements) *model.Requirements {
	if r == nil {
		return nil
	}
	c := *r
	c.Labels, c.Tools, c.Accelerators = cloneStrings(r.Labels), cloneStrings(r.Tools), cloneStrings(r.Accelerators)
	return &c
}

// cloneJob returns a deep copy of j.
func cloneJob(j model.Job) model.Job {
	c := j
	c.Run, c.Deps = cloneStrings(j.Run), cloneStrings(j.Deps)
	c.NeedsServices, c.Secrets = cloneStrings(j.NeedsServices), cloneStrings(j.Secrets)
	if j.Matrix != nil {
		c.Matrix = &model.JobMatrix{Platform: cloneAny(j.Matrix.Platform)}
	}
	c.Required, c.Idempotent, c.Retry = clonePtr(j.Required), clonePtr(j.Idempotent), clonePtr(j.Retry)
	if j.When != nil {
		w := *j.When
		w.Branches, w.Paths, w.EnvPresent = cloneStrings(j.When.Branches), cloneStrings(j.When.Paths), cloneStrings(j.When.EnvPresent)
		if j.When.Triggers != nil {
			w.Triggers = append([]model.Trigger{}, j.When.Triggers...)
		}
		c.When = &w
	}
	if j.Env != nil {
		c.Env = make(map[string]string, len(j.Env))
		for k, v := range j.Env {
			c.Env[k] = v
		}
	}
	c.Container = clonePtr(j.Container)
	if j.Artifacts != nil {
		c.Artifacts = make([]model.ArtifactDecl, len(j.Artifacts))
		for i, a := range j.Artifacts {
			a.Image = clonePtr(a.Image)
			c.Artifacts[i] = a
		}
	}
	if j.Cache != nil {
		k := *j.Cache
		k.Keys, k.Paths = cloneStrings(j.Cache.Keys), cloneStrings(j.Cache.Paths)
		c.Cache = &k
	}
	c.Requirements = cloneRequirements(j.Requirements)
	return c
}
