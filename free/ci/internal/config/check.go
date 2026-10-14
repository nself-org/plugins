package config

// Purpose: checks the JSON schema cannot express: references between jobs,
// services and pipelines, dependency cycles, and the Epic D28 warning for a
// required job that `when.env_present` can exclude.
// Inputs:  a decoded model.PipelineConfig and the line table of its file.
// Outputs: located Problems (fatal) or Warnings (E620 as a warning).
// Constraints: pure; no file or process access.

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func jobPtr(id, field string) string { return "/jobs/" + escape(id) + field }

func escape(s string) string { return strings.NewReplacer("~", "~0", "/", "~1").Replace(s) }

// semantic reports dangling references and dependency cycles.
func semantic(file string, cfg model.PipelineConfig, lines map[string]int) []Problem {
	var out []Problem
	add := func(ptr, msg string) {
		out = append(out, Problem{File: file, Line: lineFor(lines, ptr), Pointer: ptr, Message: msg})
	}
	for _, id := range sortedKeys(cfg.Jobs) {
		job := cfg.Jobs[id]
		if !safeName.MatchString(id) {
			add("/jobs/"+escape(id), "job id must match "+safeName.String())
		}
		if escapesRepo(job.Workdir) {
			add(jobPtr(id, "/workdir"), fmt.Sprintf("job %q workdir must stay inside the repository", id))
		}
		if job.When != nil {
			for i, p := range job.When.Paths {
				if escapesRepo(p) {
					add(fmt.Sprintf("%s/%d", jobPtr(id, "/when/paths"), i), "when.paths entries must stay inside the repository")
				}
			}
		}
		if len(job.Run) == 0 {
			add(jobPtr(id, "/run"), fmt.Sprintf("job %q needs a run argv with at least one element", id))
		}
		if job.Timeout != "" && !positiveDuration(job.Timeout) {
			add(jobPtr(id, "/timeout"), fmt.Sprintf("job %q timeout is not a positive duration that fits in 290 years", id))
		}
		for i, dep := range job.Deps {
			if _, ok := cfg.Jobs[dep]; !ok {
				add(fmt.Sprintf("%s/%d", jobPtr(id, "/deps"), i), fmt.Sprintf("job %q depends on unknown job %q", id, dep))
			}
		}
		for i, name := range job.NeedsServices {
			if _, ok := cfg.Services[name]; !ok {
				add(fmt.Sprintf("%s/%d", jobPtr(id, "/needs_services"), i), fmt.Sprintf("job %q needs unknown service %q", id, name))
			}
		}
	}
	for _, name := range sortedKeys(cfg.Services) {
		if !safeName.MatchString(name) {
			add("/services/"+escape(name), "service name must match "+safeName.String())
		}
	}
	for _, name := range sortedKeys(cfg.Pipelines) {
		if !safeName.MatchString(name) {
			add("/pipelines/"+escape(name), "pipeline name must match "+safeName.String())
		}
		if escapesRepo(cfg.Pipelines[name].DrillsDir) {
			add("/pipelines/"+escape(name)+"/drills_dir", "drills_dir must stay inside the repository")
		}
		if age := cfg.Pipelines[name].MaxAge; age != "" && !positiveDuration(age) {
			add("/pipelines/"+escape(name)+"/max_age", fmt.Sprintf("pipeline %q max_age is not a positive duration", name))
		}
		for i, id := range cfg.Pipelines[name].Jobs {
			if _, ok := cfg.Jobs[id]; !ok {
				add(fmt.Sprintf("/pipelines/%s/jobs/%d", escape(name), i), fmt.Sprintf("pipeline %q lists unknown job %q", name, id))
			}
		}
	}
	if cycle := findCycle(cfg.Jobs); cycle != "" {
		add("/jobs", "dependency cycle: "+cycle)
	}
	return out
}

// safeName is the shape of an id that later becomes a path component or a
// log field (the same rule internal/exec applies to attempt ids).
var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// escapesRepo reports a path that is absolute or climbs out with "..".
func escapesRepo(p string) bool {
	if p == "" {
		return false
	}
	if path.IsAbs(p) || filepath.IsAbs(p) || strings.HasPrefix(p, "~") {
		return true
	}
	clean := path.Clean(strings.ReplaceAll(p, "\\", "/"))
	return clean == ".." || strings.HasPrefix(clean, "../")
}

// positiveDuration reports whether s parses as a Go duration above zero; the
// schema pattern alone admits signs and values that overflow time.Duration.
func positiveDuration(s string) bool {
	d, err := time.ParseDuration(s)
	return err == nil && d > 0
}

// findCycle returns "a -> b -> a" for the first cycle in deps, or "".
func findCycle(jobs map[string]model.Job) string {
	const (
		unseen = iota
		open
		closed
	)
	state := map[string]int{}
	var stack []string
	var visit func(id string) string
	visit = func(id string) string {
		switch state[id] {
		case closed:
			return ""
		case open:
			for i, s := range stack {
				if s == id {
					return strings.Join(append(append([]string{}, stack[i:]...), id), " -> ")
				}
			}
		}
		state[id] = open
		stack = append(stack, id)
		for _, dep := range jobs[id].Deps {
			if _, ok := jobs[dep]; !ok {
				continue
			}
			if c := visit(dep); c != "" {
				return c
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = closed
		return ""
	}
	for _, id := range sortedKeys(jobs) {
		if c := visit(id); c != "" {
			return c
		}
	}
	return ""
}

// envPresentWarnings implements Epic D28: a required job (the default) whose
// selection depends on `when.env_present` fails the run when the variables
// are absent, so the loader says so.
func envPresentWarnings(cfg model.PipelineConfig, lines map[string]int) []Warning {
	var out []Warning
	for _, id := range sortedKeys(cfg.Jobs) {
		job := cfg.Jobs[id]
		if job.When == nil || len(job.When.EnvPresent) == 0 || (job.Required != nil && !*job.Required) {
			continue
		}
		ptr := jobPtr(id, "/when/env_present")
		out = append(out, Warning{
			Code:    CodeInvalid,
			Line:    lineFor(lines, ptr),
			Pointer: ptr,
			Message: fmt.Sprintf("job %q is required but runs only when %s is set: a missing variable fails the run; set required: false for an optional job", id, strings.Join(job.When.EnvPresent, ", ")),
		})
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
