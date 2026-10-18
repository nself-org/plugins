package doctor

// Purpose: the environment checks: language toolchains, Docker, and whether a
// commit status can be posted (gh auth plus a GitHub remote).
// Inputs:  the loaded config (may be nil), Deps.Capture.
// Outputs: Check values CI-D001 (per stack), CI-D002, CI-D003.
// Constraints: a missing tool is a warning unless a required configured job
// uses it, then a failure naming that job (package comment).

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// toolchain maps a detected stack to the binary and version argv it needs.
type toolchain struct {
	bin  string
	argv []string
}

var toolchains = map[string]toolchain{
	"go":      {"go", []string{"go", "version"}},
	"node":    {"node", []string{"node", "--version"}},
	"rust":    {"cargo", []string{"cargo", "--version"}},
	"flutter": {"flutter", []string{"flutter", "--version"}},
	"dart":    {"dart", []string{"dart", "--version"}},
}

type probeState int

const (
	probeOK probeState = iota
	probeMissing
	probeFailed
)

// probe runs argv; 127 or an unstartable process means the tool is missing.
func probe(ctx context.Context, d Deps, argv ...string) (string, probeState) {
	out, exit, err := d.Capture(ctx, argv...)
	switch {
	case err != nil || exit == 127:
		return "", probeMissing
	case exit != 0:
		return firstLine(out), probeFailed
	}
	return firstLine(out), probeOK
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// degraded is the status of a tool that is not usable: a failure naming the
// required jobs that use it, otherwise a warning.
func degraded(jobs []string) (Status, string) {
	if len(jobs) > 0 {
		return Fail, " (required by job " + strings.Join(jobs, ", ") + ")"
	}
	return Warn, ""
}

func checkToolchains(ctx context.Context, root string, cfg *model.PipelineConfig, d Deps) []Check {
	facts, err := detect.DefaultRegistry().Detect(os.DirFS(root), func(string) bool { return true })
	stacks := map[string]bool{}
	for _, f := range facts {
		if _, ok := toolchains[f.Name]; ok && f.Kind == "stack" && f.Confidence == detect.Certain {
			stacks[f.Name] = true
		}
	}
	base := Check{ID: "CI-D001", Name: "toolchain"}
	if err != nil || len(stacks) == 0 {
		base.Status, base.Message = Pass, "no language stack detected, no toolchain needed"
		if err != nil {
			base.Status, base.Message = Warn, "could not detect language stacks: "+err.Error()
			base.Remediation = "Run doctor from a readable repository root (--repo)"
		}
		return []Check{base}
	}
	names := make([]string, 0, len(stacks))
	for s := range stacks {
		names = append(names, s)
	}
	sort.Strings(names)
	var out []Check
	for _, s := range names {
		tc := toolchains[s]
		c := base
		c.Subject = s
		version, st := probe(ctx, d, tc.argv...)
		jobs := requiredJobs(cfg, func(j model.Job) bool { return len(j.Run) > 0 && filepath.Base(j.Run[0]) == tc.bin })
		switch st {
		case probeOK:
			c.Status, c.Message = Pass, s+": "+version
		case probeMissing:
			var tail string
			c.Status, tail = degraded(jobs)
			c.Message = s + ": " + tc.bin + " not found" + tail
			c.Remediation = "Install " + tc.bin + " and put it on PATH, or run jobs with --container"
		default:
			var tail string
			c.Status, tail = degraded(jobs)
			c.Message = s + ": `" + strings.Join(tc.argv, " ") + "` failed" + tail
			c.Detail = version
			c.Remediation = "Repair the " + tc.bin + " installation"
		}
		out = append(out, c)
	}
	return out
}

func needsDocker(j model.Job) bool {
	return j.Container != nil || j.Isolation == "container" || j.Isolation == "sandboxed-container"
}

func checkDocker(ctx context.Context, cfg *model.PipelineConfig, d Deps) Check {
	c := Check{ID: "CI-D002", Name: "docker"}
	version, st := probe(ctx, d, "docker", "info", "--format", "{{.ServerVersion}}")
	if st == probeOK {
		c.Status, c.Message = Pass, "docker reachable (server "+version+")"
		return c
	}
	var tail string
	c.Status, tail = degraded(requiredJobs(cfg, needsDocker))
	if st == probeMissing {
		c.Message = "docker not found" + tail
		c.Remediation = "Install Docker, or run only jobs with isolation: process"
	} else {
		c.Message = "docker is installed but the daemon is not reachable" + tail
		c.Detail = version
		c.Remediation = "Start Docker (Docker Desktop, Colima or the docker service) and re-run doctor"
	}
	return c
}

func checkStatusPosting(ctx context.Context, root string, d Deps) Check {
	c := Check{ID: "CI-D003", Name: "status posting"}
	if v, err := strconv.ParseBool(d.Getenv("NSELF_CI_SKIP_STATUS")); err == nil && v {
		c.Status, c.Message = Pass, "status posting off: "+report.ReasonNoStatus+" (NSELF_CI_SKIP_STATUS)"
		return c
	}
	_, owner, repo, reason := report.GH{Capture: d.Capture}.Resolve(ctx, root)
	if reason == "" {
		c.Status, c.Message = Pass, "status posting on ("+owner+"/"+repo+")"
		return c
	}
	c.Status, c.Message = Warn, "status posting off: "+reason
	switch reason {
	case report.ReasonNoAuth:
		c.Remediation = "Run `gh auth login`; results are still recorded locally without it"
	case report.ReasonNoRemote:
		c.Remediation = "Add a github.com `origin` remote, or ignore this if the repository is not on GitHub"
	}
	return c
}
