package drill

// Purpose: turn a pipeline's drills_dir into `kind: drill` jobs (P7-CI-02).
// Every subdirectory holding an executable `run` is one job named
// "drill-<dir>", in name order, so adding a directory adds a job with no
// config edit. A job the config already defines under that name wins, which is
// how a drill gets its own timeout or environment.
// Inputs:  the repository root and the loaded config.
// Outputs: a copy of the config whose selected pipelines list their drill jobs.
// Constraints: only pipelines that set drills_dir and are selected by --pipeline
// or --trigger are expanded (a missing drills_dir elsewhere never fails a run);
// a missing or empty drills_dir is E640, never a silent pass; symlinked
// directories and `run` files are refused so a drill cannot reach outside the
// repository; the loaded config is never mutated.

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// JobPrefix starts the id of every discovered drill job.
const JobPrefix = "drill-"

// RunFile is the executable a drill directory must hold.
const RunFile = "run"

var dirName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,120}$`)

// Discover lists the drill directory names under root/dir, sorted.
func Discover(root, dir string) ([]string, error) {
	abs, err := contained(root, dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fail(CodeInvalid, "drills_dir %q cannot be read: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() { // a symlink reports as a symlink here, not a dir: refused
			continue
		}
		if !dirName.MatchString(e.Name()) {
			return nil, fail(CodeInvalid, "drills_dir %q holds a directory %q that cannot be a job name", dir, e.Name())
		}
		fi, err := os.Lstat(filepath.Join(abs, e.Name(), RunFile))
		if err != nil || !fi.Mode().IsRegular() || (runtime.GOOS != "windows" && fi.Mode().Perm()&0o111 == 0) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// contained resolves dir under root and refuses a path that leaves it.
func contained(root, dir string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fail(CodeInvalid, "repository root %q cannot be resolved: %v", root, err)
	}
	abs, err := filepath.EvalSymlinks(filepath.Join(realRoot, filepath.FromSlash(dir)))
	if err != nil {
		return "", fail(CodeInvalid, "drills_dir %q does not exist", dir)
	}
	if rel, err := filepath.Rel(realRoot, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fail(CodeInvalid, "drills_dir %q resolves outside the repository", dir)
	}
	return abs, nil
}

// selected reports whether Expand should look at pipeline name.
func selected(name string, p model.PipelineDef, pipeline, trigger string) bool {
	switch {
	case pipeline != "":
		return name == pipeline
	case trigger != "":
		for _, t := range p.Triggers {
			if string(t) == trigger {
				return true
			}
		}
	}
	return false
}

// Expand returns cfg with the drill jobs of the selected pipelines added.
func Expand(root string, cfg model.PipelineConfig, pipeline, trigger string) (model.PipelineConfig, error) {
	out := cfg
	out.Jobs = make(map[string]model.Job, len(cfg.Jobs))
	for id, j := range cfg.Jobs {
		out.Jobs[id] = j
	}
	out.Pipelines = make(map[string]model.PipelineDef, len(cfg.Pipelines))
	for n, p := range cfg.Pipelines {
		out.Pipelines[n] = p
	}
	for _, name := range sortedNames(cfg.Pipelines) {
		p := cfg.Pipelines[name]
		if p.DrillsDir == "" || !selected(name, p, pipeline, trigger) {
			continue
		}
		dir := path.Clean(filepath.ToSlash(p.DrillsDir))
		drills, err := Discover(root, dir)
		if err != nil {
			return cfg, err
		}
		if len(drills) == 0 && len(p.Jobs) == 0 {
			return cfg, fail(CodeInvalid, "pipeline %q: drills_dir %q holds no drill (a directory with an executable `run`)", name, dir)
		}
		jobs := append([]string(nil), p.Jobs...)
		for _, d := range drills {
			id := JobPrefix + d
			if _, ok := out.Jobs[id]; !ok {
				out.Jobs[id] = model.Job{Kind: "drill", Run: []string{"./" + path.Join(dir, d, RunFile)}}
			}
			if !contains(jobs, id) {
				jobs = append(jobs, id)
			}
		}
		p.Jobs = jobs
		out.Pipelines[name] = p
	}
	return out, nil
}

func sortedNames(m map[string]model.PipelineDef) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}
