package plan

// Purpose: turn the config's jobs and the catalogue checks into candidate
// jobs with every default applied, so later stages read one shape.
// Inputs:  Input.Config.Jobs and Input.Checks.
// Outputs: candidates keyed by job id plus the sorted id list.
// Constraints: a config job wins over a catalogue job with the same id;
// the input is never mutated (slices are copied); defaults match
// internal/config precedence: required, deep, timeout 10m, one infra retry.

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Defaults shared with the config layer.
const (
	defaultTimeout = "10m"
	defaultRetry   = 1
	// MaxInfraRetries bounds the infra-only retry of any job.
	MaxInfraRetries = 5
)

type cand struct {
	id, name, check, workdir, tool string
	job                            model.Job
	required, allowFail, deep      bool
	derived                        bool
}

func configCand(id string, in model.Job) *cand {
	j := cloneJob(in)
	j.Deps = sortedUnique(in.Deps)
	if j.Workdir == "" {
		j.Workdir = "."
	}
	if j.Path == "" {
		j.Path = "deep"
	}
	if j.Priority == "" {
		j.Priority = "normal"
	}
	if j.Timeout == "" {
		j.Timeout = defaultTimeout
	}
	if j.Isolation == "" {
		j.Isolation = "process"
	}
	req := true
	if in.Required != nil {
		req = *in.Required
	}
	j.Required = &req
	idem := true
	if in.Idempotent != nil {
		idem = *in.Idempotent
	}
	j.Idempotent = &idem
	j.Retry = &model.RetryConfig{InfraMax: retryFor(in.Retry, idem)}
	return &cand{id: id, name: id, workdir: cleanDir(j.Workdir), job: j,
		required: req, allowFail: in.AllowFailure, deep: j.Path == "deep"}
}

// retryFor bounds infra retries: default one, never negative, none for a
// job that is not idempotent, at most MaxInfraRetries.
func retryFor(r *model.RetryConfig, idempotent bool) int {
	n := defaultRetry
	if r != nil {
		n = r.InfraMax
	}
	if n < 0 || !idempotent {
		n = 0
	}
	if n > MaxInfraRetries {
		n = MaxInfraRetries
	}
	return n
}

var kindOfCheck = map[string]model.JobKind{
	"format": "static", "lint": "static", "input": "static",
	"test": "test", "coverage": "test", "build": "build", "security": "quality",
}

var slugBad = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func derivedCand(c Check, id string) *cand {
	kind, ok := kindOfCheck[c.Kind]
	if !ok {
		kind = "quality"
	}
	p := c.Path
	if p == "" {
		p = "deep"
	}
	req := c.Required
	idem := true
	wd := cleanDir(c.Workdir)
	j := model.Job{
		Kind: kind, Run: append([]string(nil), c.Argv...), Workdir: wd, Timeout: defaultTimeout,
		Required: &req, Idempotent: &idem, Retry: &model.RetryConfig{InfraMax: defaultRetry},
		Priority: "normal", Path: p, Isolation: "process",
	}
	if c.Platform != "" {
		j.Matrix = &model.JobMatrix{Platform: []string{c.Platform}}
	}
	if len(c.Requires) > 0 {
		j.Requirements = &model.Requirements{Tools: sortedUnique(c.Requires)}
	}
	return &cand{id: id, name: id, check: c.ID, workdir: wd, tool: c.Tool, job: j,
		required: req, deep: p == "deep", derived: true}
}

func deriveID(c Check) string {
	id := c.ID
	if d := cleanDir(c.Workdir); d != "." {
		id += "." + strings.ReplaceAll(d, "/", ".")
	}
	if c.Target != "" {
		id += "." + c.Target
	}
	return slugBad.ReplaceAllString(id, "-")
}

// candidates builds every candidate job and the sorted id list.
func candidates(in Input) (map[string]*cand, []string) {
	out := map[string]*cand{}
	for id, j := range in.Config.Jobs {
		out[id] = configCand(id, j)
	}
	checks := append([]Check(nil), in.Checks...)
	sort.Slice(checks, func(i, j int) bool {
		a, b := checks[i], checks[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if cleanDir(a.Workdir) != cleanDir(b.Workdir) {
			return cleanDir(a.Workdir) < cleanDir(b.Workdir)
		}
		return path.Clean(a.Target) < path.Clean(b.Target)
	})
	for _, c := range checks {
		base := deriveID(c)
		if _, ok := in.Config.Jobs[base]; ok {
			continue // the configured job replaces the catalogue check
		}
		id := base
		for n := 2; out[id] != nil; n++ {
			id = base + "-" + strconv.Itoa(n)
		}
		out[id] = derivedCand(c, id)
		settings := in.Config.Checks[c.ID]
		extra := cloneJob(model.Job{Env: settings.Env, Secrets: settings.Secrets})
		out[id].job.Env, out[id].job.Secrets = extra.Env, extra.Secrets
	}
	ids := make([]string, 0, len(out))
	for id := range out {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return out, ids
}
