package api

// Purpose: read a GitLab version string and decide whether the instance takes pipeline inputs.
// Inputs:  the "version" field of GET /version ("17.10.0-ee", "18.4.1", "16.11.2-ee").
// Outputs: major and minor numbers; SupportsInputs.
// Constraints: pipeline inputs on POST /projects/:id/pipeline arrived in GitLab 17.10 (behind a flag that
//          was on by default) and are general in 18.1 (docs.gitlab.com/api/pipelines, read 2026-10-19);
//          a version that does not parse supports nothing, so the dispatch falls back to variables.

import (
	"strconv"
	"strings"
)

// InputsSince is the first version that accepts pipeline inputs on the create call.
const InputsSince = "17.10"

// ParseVersion returns the major and minor of a GitLab version string.
func ParseVersion(v string) (major, minor int, ok bool) {
	v = strings.TrimSpace(v)
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	var err1, err2 error
	major, err1 = strconv.Atoi(parts[0])
	minor, err2 = strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || major < 0 || minor < 0 {
		return 0, 0, false
	}
	return major, minor, true
}

// SupportsInputs reports whether the version takes pipeline inputs.
func SupportsInputs(version string) bool {
	major, minor, ok := ParseVersion(version)
	return ok && (major > 17 || major == 17 && minor >= 10)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
