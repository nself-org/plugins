package truth

// Purpose: repo-relative path globs for rule scopes, skips and allowlists.
// Inputs:  a slash-separated relative path and a pattern.
// Outputs: whether the pattern matches.
// Constraints: `**` as a whole segment matches zero or more segments; any other
// segment uses path.Match; a pattern with no slash matches the base name at any
// depth (so `CHANGELOG*` covers every changelog); the match is on the whole path
// and ignores letter case.

import (
	"path"
	"strings"
)

// matchGlob reports whether rel (slash-separated, no leading slash) matches pattern.
func matchGlob(pattern, rel string) bool {
	pattern, rel = strings.ToLower(pattern), strings.ToLower(rel) // A.MD is a Markdown file too
	if !strings.Contains(pattern, "/") {
		ok, _ := path.Match(pattern, path.Base(rel))
		return ok
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for len(pat) > 1 && pat[1] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegments(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

// matchAny reports whether any pattern matches rel.
func matchAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		if matchGlob(p, rel) {
			return true
		}
	}
	return false
}

// validGlob reports whether path.Match accepts every segment of the pattern.
//
// A pattern with an empty segment (`docs/`, `a//b`) or a leading `./` or `/`
// can never match a clean relative path, so it is refused rather than inert.
func validGlob(pattern string) bool {
	if pattern == "" || strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "./") {
		return false
	}
	for _, seg := range strings.Split(pattern, "/") {
		if seg == "" {
			return false
		}
		if _, err := path.Match(seg, "x"); err != nil {
			return false
		}
	}
	return true
}
