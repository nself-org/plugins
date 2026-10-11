package render

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// A tiny evaluator for the router-rule subset the renderer emits (Host, HostRegexp, Path,
// PathPrefix, PathRegexp, Method, &&, ||, parentheses). It lets a test ask which router
// Traefik would pick for a request (highest priority whose rule matches), so host and
// location ordering is judged on behaviour, not on priority numbers.
type reqIn struct{ host, path, method string }

var ruleTok = regexp.MustCompile("^\\s*(\\(|\\)|&&|\\|\\||[A-Za-z]+\\(`[^`]*`\\))")

func evalRule(rule string, q reqIn) (bool, error) {
	toks := []string{}
	for rest := rule; strings.TrimSpace(rest) != ""; {
		m := ruleTok.FindStringSubmatch(rest)
		if m == nil {
			return false, fmt.Errorf("cannot tokenise %q", rest)
		}
		toks = append(toks, m[1])
		rest = rest[len(m[0]):]
	}
	pos := 0
	var expr func() bool
	factor := func() bool {
		t := toks[pos]
		pos++
		if t == "(" {
			v := expr()
			pos++ // ")"
			return v
		}
		i := strings.Index(t, "(`")
		fn, arg := t[:i], t[i+2:len(t)-2]
		switch fn {
		case "Host":
			return strings.EqualFold(q.host, arg)
		case "HostRegexp":
			return regexp.MustCompile(arg).MatchString(strings.ToLower(q.host))
		case "Path":
			return q.path == arg
		case "PathPrefix":
			return strings.HasPrefix(q.path, arg)
		case "PathRegexp":
			return regexp.MustCompile(arg).MatchString(q.path)
		case "Method":
			return q.method == arg
		}
		panic("unknown matcher " + fn)
	}
	term := func() bool {
		v := factor()
		for pos < len(toks) && toks[pos] == "&&" {
			pos++
			v = factor() && v
		}
		return v
	}
	expr = func() bool {
		v := term()
		for pos < len(toks) && toks[pos] == "||" {
			pos++
			v = term() || v
		}
		return v
	}
	return expr(), nil
}

// winner returns the name and spec of the router Traefik would pick for the request on the
// given entrypoint, or "" when none matches.
func winner(t *testing.T, out []byte, entry string, q reqIn) (string, map[string]any) {
	t.Helper()
	if q.method == "" {
		q.method = "GET"
	}
	best, bestPri, spec0 := "", -1, map[string]any(nil)
	for name, r := range parse(t, out)["http"]["routers"] {
		spec := r.(map[string]any)
		if eps := spec["entryPoints"].([]any); len(eps) != 1 || eps[0] != entry {
			continue
		}
		ok, err := evalRule(spec["rule"].(string), q)
		if err != nil {
			t.Fatal(err)
		}
		pri := spec["priority"].(int)
		if ok && (pri > bestPri || (pri == bestPri && name < best)) {
			best, bestPri, spec0 = name, pri, spec
		}
	}
	return best, spec0
}

// answers reports the fixed status a router answers (its replacePath middleware), or 0.
func answers(t *testing.T, out []byte, spec map[string]any) int {
	t.Helper()
	mws := parse(t, out)["http"]["middlewares"]
	for _, n := range spec["middlewares"].([]any) {
		if rp, ok := mws[n.(string)].(map[string]any)["replacePath"].(map[string]any); ok {
			var st int
			if _, err := fmt.Sscanf(rp["path"].(string), "/__nself/status/%d", &st); err == nil {
				return st
			}
		}
	}
	return 0
}
