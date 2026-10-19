package specs

// Purpose: the per-job `cache:` config (Epic CACHE D5): `paths` narrow which
// detected projects of the workspace the cache applies to, `keys` add key
// fields, `disable` turns the job's cache off. The revision can only tighten
// (X1): nothing here widens a spec, enables a disabled cache or adds a dir.
// Inputs:  the job's cache config values and the detector facts.
// Outputs: CheckPaths, Narrow, Scope.
// Constraints: pure. A path outside the workspace (absolute, ".." component,
// backslash, NUL or control byte) is refused with E711 path_outside before the
// job starts.

import (
	"path"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// CheckPaths validates cache.paths and returns the cleaned, sorted, unique set.
func CheckPaths(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		c := path.Clean(strings.TrimSuffix(p, "/"))
		if p == "" || path.IsAbs(p) || strings.ContainsAny(p, "\\\x00") || hasControl(p) || c == ".." || strings.HasPrefix(c, "../") {
			return nil, &entries.Refused{Reason: entries.ReasonPathOutside}
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out, nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// Narrow keeps the facts whose directory lies in one of scope's directories;
// an empty scope keeps them all.
func Narrow(facts []detect.Fact, scope []string) []detect.Fact {
	if len(scope) == 0 {
		return facts
	}
	var out []detect.Fact
	for _, f := range facts {
		dir := path.Dir(f.Path)
		for _, s := range scope {
			if s == "." || dir == s || strings.HasPrefix(dir, s+"/") {
				out = append(out, f)
				break
			}
		}
	}
	return out
}
