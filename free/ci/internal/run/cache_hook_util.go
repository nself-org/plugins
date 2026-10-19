package run

// Purpose: the small pure helpers of the cache hook (cache_hook.go): the level a
// job really runs at, the directories private-dependency files are looked for
// in, environment merging, the top-level disable switch and the stale
// cache-work sweep.
// Inputs:  AttemptInfo, detector facts, environment lists.
// Outputs: values for cacheHook.Before and begin.
// Constraints: no state; nothing here writes except sweepStale (removes
// abandoned cache-work directories older than staleAfter).

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// cacheLevel is where the job really runs, as the executor decides it: "container"
// for the run's --container option or isolation container (a Container block
// alone changes nothing), "process" for the default, and none for any other
// isolation (vm, sandbox, hosted), which cannot see the host's cache dirs.
func cacheLevel(a AttemptInfo) (string, bool) {
	switch {
	case a.Container || a.Job.Isolation == "container":
		return "container", true
	case a.Job.Isolation == "" || a.Job.Isolation == "process":
		return "process", true
	}
	return "", false
}

// factDirs lists the directories private-dependency files are looked for in:
// the project root, the job's workdir and the directory of every detected
// project (a monorepo package may carry its own .npmrc).
func factDirs(root, workdir string, facts []detect.Fact) []string {
	dirs := []string{root, workdir}
	for _, f := range facts {
		if d := path.Dir(f.Path); d != "." {
			dirs = append(dirs, filepath.Join(workdir, filepath.FromSlash(d)))
		}
	}
	return dirs
}

func lookup(environ []string, name string) string {
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// workdirOf resolves the job's workdir below the project root (relative, inside).
func workdirOf(root, workdir string) (string, error) {
	if workdir == "" || workdir == "." {
		return root, nil
	}
	if !fs.ValidPath(workdir) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(root, filepath.FromSlash(workdir)), nil
}

// mergeEnv overlays the job's env on the process environment.
func mergeEnv(environ []string, job map[string]string) map[string]string {
	out := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	for k, v := range job {
		out[k] = v
	}
	return out
}

func flatEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// sweepStale removes cache-work directories abandoned by a crashed run. Removal
// goes through a handle on cache-work, so a planted symlink is removed as a
// link and its target is never touched.
func sweepStale(dir string, now time.Time, alive func(id string) bool) {
	parent, err := os.OpenRoot(dir)
	if err != nil {
		return
	}
	defer func() { _ = parent.Close() }()
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if alive(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > staleAfter {
			_ = specs.RemoveIn(parent, e.Name())
		}
	}
}

// topLevelDisabled reads the top-level `cache: {disable: true}` of the default
// config (<root>/.nself/ci.yaml). The revision can only tighten, so an
// unreadable or absent file means "not disabled here"; a config passed with
// --config from elsewhere is not seen (debt D-cache-config-path).
func topLevelDisabled(root string) bool {
	l, err := config.Load(filepath.Join(root, ".nself", "ci.yaml"))
	return err == nil && l.Config.Cache != nil && l.Config.Cache.Disable
}

func off(v string) bool { return v == "1" || strings.EqualFold(v, "true") }
