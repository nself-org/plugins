package adhoc

// Purpose: the positional secret forms of an ad-hoc argv, found by shape
// because no secret-looking flag name precedes them (D-1020).
// Inputs:  the argv. Outputs: spans (argument index plus byte range) of the
// secret part, used by Register and Recorded in argv.go.
// Constraints: well-known forms only, nothing is guessed. Tool-specific forms
// (mysql -p<pw>, curl -u user:pass) need the tool named in the argv, so
// `docker run -u 1000:1000` and `gcc -pthread` stay visible. The tool match is
// on any argument's basename so `sudo`, `env` and `timeout` wrappers still hit.

import (
	"path/filepath"
	"regexp"
	"strings"
)

// span is the secret part of argv[arg]: bytes [from, to).
type span struct {
	arg, from, to int
	parts         bool // a header value: its words and cookie values are secret on their own
}

// authSchemes are the words that lead an Authorization value; they are not secret.
var authSchemes = map[string]bool{"basic": true, "bearer": true, "digest": true, "token": true}

// pieces are the values to register for a span: the whole text, and for a
// header its words and `name=value` values, so the bare secret is masked in
// job output too.
func pieces(text string, parts bool) []string {
	out := []string{text}
	if !parts {
		return out
	}
	for _, w := range strings.Fields(text) {
		w = strings.TrimRight(w, ";,")
		if _, v, ok := strings.Cut(w, "="); ok {
			w = v
		}
		if w != "" && w != text && !authSchemes[strings.ToLower(w)] {
			out = append(out, w)
		}
	}
	return out
}

const secretAlt = `token|secret|passw(?:or)?d|passwd|credential|api[-_]?key|private[-_]?key|access[-_]?key|auth`

var (
	// "password": "v" in a JSON body; group 1 is the value.
	jsonSecret = regexp.MustCompile(`(?i)"[\w.-]*(?:` + secretAlt + `)[\w.-]*"\s*:\s*"([^"]+)"`)
	// a=b&password=v in a form body or query; group 1 is the value.
	formSecret = regexp.MustCompile(`(?i)[&?;][\w.-]*(?:` + secretAlt + `)[\w.-]*=([^&\s]+)`)
	// scheme://user:pass@host; group 1 is the password.
	urlUserinfo = regexp.MustCompile(`://[^/\s:@]+:([^@/\s]+)@`)
	// Name: value header whose name carries a secret; group 1 is the value.
	secretHeader = regexp.MustCompile(`(?i)^\s*[\w-]*(?:` + secretAlt + `|cookie)[\w-]*\s*:\s*(\S.*)$`)
)

var (
	mysqlTools = map[string]bool{"mysql": true, "mysqldump": true, "mysqladmin": true, "mariadb": true, "mariadb-dump": true}
	curlTools  = map[string]bool{"curl": true}
)

// hasTool reports whether any argument is one of the named tools.
func hasTool(argv []string, tools map[string]bool) bool {
	for _, a := range argv {
		if tools[filepath.Base(a)] {
			return true
		}
	}
	return false
}

// formSpans returns the positional secret spans of argv.
func formSpans(argv []string) []span {
	var out []span
	add := func(i int, a string, m []int) {
		if len(m) >= 4 && m[2] >= 0 && m[3] > m[2] {
			out = append(out, span{arg: i, from: m[2], to: m[3]})
		}
	}
	mysql, curl := hasTool(argv, mysqlTools), hasTool(argv, curlTools)
	for i, a := range argv {
		add(i, a, jsonSecret.FindStringSubmatchIndex(a))
		for _, m := range formSecret.FindAllStringSubmatchIndex(a, -1) {
			add(i, a, m)
		}
		add(i, a, urlUserinfo.FindStringSubmatchIndex(a))
		if mysql && strings.HasPrefix(a, "-p") && !strings.HasPrefix(a, "--") && len(a) > 2 {
			out = append(out, span{arg: i, from: 2, to: len(a)})
		}
		if curl {
			out = append(out, curlUserSpans(argv, i)...)
		}
		out = append(out, headerSpans(argv, i)...)
	}
	return out
}

// curlUserSpans finds the password of `-u user:pass`, `-uuser:pass` and
// `--user[=]user:pass` (also --proxy-user) when argv[i] is the option.
func curlUserSpans(argv []string, i int) []span {
	a := argv[i]
	passAt := func(arg, from int) []span {
		if k := strings.IndexByte(argv[arg][from:], ':'); k >= 0 && from+k+1 < len(argv[arg]) {
			return []span{{arg: arg, from: from + k + 1, to: len(argv[arg])}}
		}
		return nil
	}
	switch {
	case a == "-u" || a == "-U" || a == "--user" || a == "--proxy-user":
		if i+1 < len(argv) {
			return passAt(i+1, 0)
		}
	case strings.HasPrefix(a, "--user=") || strings.HasPrefix(a, "--proxy-user="):
		return passAt(i, strings.IndexByte(a, '=')+1)
	case strings.HasPrefix(a, "-u") && !strings.HasPrefix(a, "--"), strings.HasPrefix(a, "-U") && !strings.HasPrefix(a, "--"):
		return passAt(i, 2)
	}
	return nil
}

// headerSpans finds the value of a secret header given as `-H V`,
// `--header V` or `--header=V` when argv[i] is the option.
func headerSpans(argv []string, i int) []span {
	a := argv[i]
	switch {
	case (a == "-H" || a == "--header") && i+1 < len(argv):
		if m := secretHeader.FindStringSubmatchIndex(argv[i+1]); m != nil {
			return []span{{arg: i + 1, from: m[2], to: m[3], parts: true}}
		}
	case strings.HasPrefix(a, "--header="):
		if m := secretHeader.FindStringSubmatchIndex(a[len("--header="):]); m != nil {
			off := len("--header=")
			return []span{{arg: i, from: off + m[2], to: off + m[3], parts: true}}
		}
	}
	return nil
}
