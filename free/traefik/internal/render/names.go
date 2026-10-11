package render

import (
	"fmt"
	"reflect"
	"regexp"
)

// rname is the injective prefix of every router a route owns: the readable form
// of the id plus a hash of the exact id. safe() alone maps distinct ids to one
// name ("a/b.c" and "a-b/c"), and the later route would silently overwrite the
// earlier router (a deny location could vanish).
func rname(id string) string { return safe(id) + "-" + h8(id) }

// put stores one named item of a Traefik section. A name already taken by a
// different spec is a refusal (exit 1, nothing written), never an overwrite.
// Content-hashed items may repeat with an equal spec; routers may not (unique).
func (b *builder) put(section map[string]any, kind, name string, spec any, unique bool) {
	if old, taken := section[name]; taken && (unique || !reflect.DeepEqual(old, spec)) {
		b.refuse(b.cur, kind, fmt.Sprintf("name %q is already used by another route or location; the later one would overwrite it", name))
		return
	}
	section[name] = spec
}

var headerNameRE = regexp.MustCompile("^[!#$%&'*+.^_|~0-9A-Za-z-]+$")

// headerOK reports whether a header name is an RFC 9110 token and the value
// holds no control character (tab excepted), so the renderer never relies on
// Traefik or Go to drop an injection attempt.
func headerOK(name, val string) bool {
	if !headerNameRE.MatchString(name) {
		return false
	}
	for i := 0; i < len(val); i++ {
		if c := val[i]; (c < 0x20 && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}
